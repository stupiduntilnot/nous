package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
