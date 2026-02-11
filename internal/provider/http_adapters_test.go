package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collectEvents(ch <-chan Event) []Event {
	out := make([]Event, 0, 4)
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestOpenAIAdapterStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello from openai"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Prompt: "hi"}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventTextDelta || evs[1].Delta != "hello from openai" {
		t.Fatalf("unexpected text event: %+v", evs[1])
	}
}

func TestOpenAIAdapterToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"finish_reason": "tool_calls",
					"message": map[string]any{
						"tool_calls": []map[string]any{
							{
								"id":   "call-1",
								"type": "function",
								"function": map[string]any{
									"name":      "lookup",
									"arguments": `{"q":"go"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Prompt: "hi"}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventToolCall {
		t.Fatalf("expected tool call event, got %+v", evs[1])
	}
	if evs[1].ToolCall.Name != "lookup" {
		t.Fatalf("unexpected tool call name: %+v", evs[1].ToolCall)
	}
	if got, _ := evs[1].ToolCall.Arguments["q"].(string); got != "go" {
		t.Fatalf("unexpected tool call args: %+v", evs[1].ToolCall.Arguments)
	}
	if evs[2].Type != EventAwaitNext {
		t.Fatalf("expected await-next event, got %+v", evs[2])
	}
}

func TestOpenAIAdapterTextToolCallFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"content": "```json\n{\n  \"name\": \"read registered runtime tool\",\n  \"arguments\": {\"path\": \"/tmp/a.txt\"}\n}\n```",
					},
				},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Prompt:      "read file",
		ActiveTools: []string{"read", "ls"},
	}))
	if len(evs) < 4 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventToolCall {
		t.Fatalf("expected tool call event, got %+v", evs[1])
	}
	if evs[1].ToolCall.Name != "read" {
		t.Fatalf("unexpected tool call name: %+v", evs[1].ToolCall)
	}
	if got, _ := evs[1].ToolCall.Arguments["path"].(string); got != "/tmp/a.txt" {
		t.Fatalf("unexpected tool call args: %+v", evs[1].ToolCall.Arguments)
	}
	if evs[2].Type != EventAwaitNext {
		t.Fatalf("expected await-next event, got %+v", evs[2])
	}
}

func TestGeminiAdapterStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{
				{"content": map[string]any{"parts": []map[string]any{{"text": "hello "}, {"text": "gemini"}}}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewGeminiAdapter("test-key", "gemini-test", srv.URL)
	if err != nil {
		t.Fatalf("new gemini adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{Prompt: "hi"}))
	if len(evs) < 3 {
		t.Fatalf("unexpected event count: %d", len(evs))
	}
	if evs[1].Type != EventTextDelta || evs[1].Delta != "hello gemini" {
		t.Fatalf("unexpected text event: %+v", evs[1])
	}
}

func TestOpenAIAdapterSendsActiveTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawTools, ok := req["tools"].([]any)
		if !ok || len(rawTools) != 2 {
			t.Fatalf("expected two tools in request, got: %#v", req["tools"])
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Prompt:      "hi",
		ActiveTools: []string{"tool_a", "tool_b"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}

func TestOpenAIAdapterSendsToolResultsAsFollowupContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawMsgs, ok := req["messages"].([]any)
		if !ok || len(rawMsgs) < 2 {
			t.Fatalf("expected two messages, got: %#v", req["messages"])
		}
		last, _ := rawMsgs[len(rawMsgs)-1].(map[string]any)
		content, _ := last["content"].(string)
		if !strings.Contains(content, "Tool results:") || !strings.Contains(content, "first => ok") {
			t.Fatalf("tool results message missing, got: %q", content)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	a, err := NewOpenAIAdapter("test-key", "gpt-test", srv.URL)
	if err != nil {
		t.Fatalf("new openai adapter failed: %v", err)
	}
	evs := collectEvents(a.Stream(context.Background(), Request{
		Prompt:      "hi",
		ToolResults: []string{"first => ok"},
	}))
	if len(evs) < 3 || evs[1].Type != EventTextDelta {
		t.Fatalf("unexpected events: %+v", evs)
	}
}
