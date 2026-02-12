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

func TestAdapterContractMockText(t *testing.T) {
	a := NewMockAdapter()
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}))
	assertTextResponseContract(t, evs)
	assertKnownEventTypes(t, evs)
}

func TestAdapterContractOpenAIText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}))
	assertTextResponseContract(t, evs)
	assertKnownEventTypes(t, evs)
}

func TestAdapterContractGeminiText(t *testing.T) {
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
	evs := collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{{Role: "user", Content: "hello"}},
	}))
	assertTextResponseContract(t, evs)
	assertKnownEventTypes(t, evs)
}

func assertTextResponseContract(t *testing.T, evs []Event) {
	t.Helper()
	if len(evs) < 3 {
		t.Fatalf("unexpected short event stream: %+v", evs)
	}
	if evs[0].Type != EventStart {
		t.Fatalf("first event must be start, got %+v", evs[0])
	}
	if evs[len(evs)-1].Type != EventDone {
		t.Fatalf("last event must be done, got %+v", evs[len(evs)-1])
	}
	if evs[len(evs)-1].StopReason == "" {
		t.Fatalf("done event must include stop reason, got %+v", evs[len(evs)-1])
	}
	hasText := false
	for _, ev := range evs {
		if ev.Type == EventError {
			t.Fatalf("unexpected error event in text contract: %+v", ev)
		}
		if ev.Type == EventTextDelta {
			hasText = true
		}
	}
	if !hasText {
		t.Fatalf("expected at least one text_delta event, got %+v", evs)
	}
}

func assertKnownEventTypes(t *testing.T, evs []Event) {
	t.Helper()
	for i, ev := range evs {
		switch ev.Type {
		case EventStart, EventTextDelta, EventToolCall, EventAwaitNext, EventDone, EventError:
		default:
			t.Fatalf("unknown event type at %d: %+v", i, ev)
		}
	}
}

func TestAdapterContractOpenAIStructuredToolBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawMsgs, ok := req["messages"].([]any)
		if !ok || len(rawMsgs) != 3 {
			t.Fatalf("expected three messages, got: %#v", req["messages"])
		}
		assistant, _ := rawMsgs[1].(map[string]any)
		if role, _ := assistant["role"].(string); role != "assistant" {
			t.Fatalf("unexpected assistant role: %#v", assistant)
		}
		if _, ok := assistant["tool_calls"].([]any); !ok {
			t.Fatalf("expected assistant tool_calls payload, got: %#v", assistant)
		}
		toolMsg, _ := rawMsgs[2].(map[string]any)
		if role, _ := toolMsg["role"].(string); role != "tool" {
			t.Fatalf("expected tool role for structured tool result, got: %#v", toolMsg)
		}
		if id, _ := toolMsg["tool_call_id"].(string); id != "call-1" {
			t.Fatalf("unexpected tool_call_id: %#v", toolMsg)
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
		Messages: []Message{
			{Role: "user", Content: "hi"},
			{
				Role: "assistant",
				ToolCalls: []ToolCall{
					{ID: "call-1", Name: "read", Arguments: map[string]any{"path": "/tmp/a"}},
				},
				Blocks: []ContentBlock{
					{Type: "tool_call", ToolCallID: "call-1", ToolName: "read", Arguments: map[string]any{"path": "/tmp/a"}},
				},
			},
			{
				Role:       "tool_result",
				Content:    "read => 1",
				ToolCallID: "call-1",
				Blocks: []ContentBlock{
					{Type: "tool_result", Text: "read => 1", ToolCallID: "call-1", ToolName: "read"},
				},
			},
		},
	}))
	assertKnownEventTypes(t, evs)
	if len(evs) < 3 || evs[len(evs)-1].Type != EventDone {
		t.Fatalf("unexpected event stream: %+v", evs)
	}
}

func TestAdapterContractOpenAIFallbackUnknownBlocksToText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request failed: %v", err)
		}
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		rawMsgs, ok := req["messages"].([]any)
		if !ok || len(rawMsgs) == 0 {
			t.Fatalf("expected messages in request, got: %#v", req["messages"])
		}
		first, _ := rawMsgs[0].(map[string]any)
		content, _ := first["content"].(string)
		if !strings.Contains(content, "fallback block") {
			t.Fatalf("expected fallback block content, got: %#v", first)
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
	_ = collectEvents(a.Stream(context.Background(), Request{
		Messages: []Message{
			{Role: "user", Blocks: []ContentBlock{{Type: "widget", Text: "fallback block"}}},
		},
	}))
}
