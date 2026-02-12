package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
