package core

import (
	"context"
	"errors"
	"testing"

	"oh-my-agent/internal/provider"
)

type providerErrorAdapter struct{}

func (providerErrorAdapter) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 1)
	out <- provider.Event{Type: provider.EventError, Err: errors.New("upstream_down")}
	close(out)
	return out
}

type missingToolAdapter struct{}

func (missingToolAdapter) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 2)
	out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "tc-1", Name: "missing.tool", Arguments: map[string]any{}}}
	out <- provider.Event{Type: provider.EventDone}
	close(out)
	return out
}

func TestPromptEmitsErrorEventOnProviderError(t *testing.T) {
	e := NewEngine(NewRuntime(), providerErrorAdapter{})
	events := make([]Event, 0, 8)
	unsub := e.Subscribe(func(ev Event) { events = append(events, ev) })
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-provider-error", "hello"); err == nil {
		t.Fatalf("expected provider error")
	}

	found := false
	for _, ev := range events {
		if ev.Type == EventError {
			found = true
			if ev.Code != "provider_error" {
				t.Fatalf("unexpected error code: %s", ev.Code)
			}
			if ev.Cause == "" {
				t.Fatalf("expected non-empty error cause")
			}
		}
	}
	if !found {
		t.Fatalf("expected EventError to be emitted, got: %+v", events)
	}
}

func TestPromptEmitsWarningEventOnUnknownTool(t *testing.T) {
	e := NewEngine(NewRuntime(), missingToolAdapter{})
	events := make([]Event, 0, 8)
	unsub := e.Subscribe(func(ev Event) { events = append(events, ev) })
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-missing-tool", "hello"); err == nil {
		t.Fatalf("expected missing tool error")
	}

	found := false
	for _, ev := range events {
		if ev.Type == EventWarning {
			found = true
			if ev.Code != "tool_not_found" {
				t.Fatalf("unexpected warning code: %s", ev.Code)
			}
		}
	}
	if !found {
		t.Fatalf("expected EventWarning to be emitted, got: %+v", events)
	}
}
