package core

import (
	"context"
	"errors"
	"testing"

	"nous/internal/provider"
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

type providerDoneMetadataAdapter struct{}

func (providerDoneMetadataAdapter) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 3)
	out <- provider.Event{Type: provider.EventStart}
	out <- provider.Event{Type: provider.EventTextDelta, Delta: "ok"}
	out <- provider.Event{
		Type:       provider.EventDone,
		StopReason: provider.StopReasonLength,
		Usage: &provider.Usage{
			InputTokens:  3,
			OutputTokens: 2,
			TotalTokens:  5,
		},
	}
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

func TestPromptEmitsStatusEventsForProviderDoneMetadata(t *testing.T) {
	e := NewEngine(NewRuntime(), providerDoneMetadataAdapter{})
	events := make([]Event, 0, 8)
	unsub := e.Subscribe(func(ev Event) { events = append(events, ev) })
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-provider-done-metadata", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	foundStop := false
	foundUsage := false
	for _, ev := range events {
		if ev.Type != EventStatus {
			continue
		}
		if ev.Message == "provider_stop_reason: length" {
			foundStop = true
		}
		if ev.Message == "provider_usage: input=3 output=2 total=5" {
			foundUsage = true
		}
	}
	if !foundStop || !foundUsage {
		t.Fatalf("expected provider done metadata status events, got: %+v", events)
	}
}
