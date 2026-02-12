package core

import (
	"context"
	"testing"

	"nous/internal/provider"
)

type captureSingleRequestProvider struct {
	last provider.Request
}

func (c *captureSingleRequestProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	c.last = req
	out := make(chan provider.Event, 2)
	out <- provider.Event{Type: provider.EventTextDelta, Delta: "ok"}
	out <- provider.Event{Type: provider.EventDone}
	close(out)
	return out
}

func TestEngineAppliesTransformContextBeforeConvertToLLM(t *testing.T) {
	p := &captureSingleRequestProvider{}
	e := NewEngine(NewRuntime(), p)

	sawTransform := false
	e.SetTransformContext(func(_ context.Context, messages []Message) ([]Message, error) {
		return append(messages, Message{Role: RoleUser, Text: "transform-added"}), nil
	})
	e.SetConvertToLLM(func(messages []Message) ([]provider.Message, error) {
		for _, msg := range messages {
			if msg.Role == RoleUser && msg.Text == "transform-added" {
				sawTransform = true
			}
		}
		return []provider.Message{{Role: "user", Content: "converted-only"}}, nil
	})

	if _, err := e.Prompt(context.Background(), "run-transform-convert", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !sawTransform {
		t.Fatalf("expected convertToLLM to observe transformed context")
	}
	if len(p.last.Messages) != 1 || p.last.Messages[0].Content != "converted-only" {
		t.Fatalf("expected provider request to use convertToLLM output, got: %+v", p.last.Messages)
	}
}

func TestEngineDefaultConvertFiltersCustomMessages(t *testing.T) {
	p := &captureSingleRequestProvider{}
	e := NewEngine(NewRuntime(), p)

	e.SetTransformContext(func(_ context.Context, messages []Message) ([]Message, error) {
		return append(messages, Message{Role: RoleCustom, Text: "ui-only-note"}), nil
	})

	if _, err := e.Prompt(context.Background(), "run-filter-custom", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if len(p.last.Messages) != 1 {
		t.Fatalf("expected custom message to be filtered from provider request, got: %+v", p.last.Messages)
	}
	if p.last.Messages[0].Role != "user" || p.last.Messages[0].Content != "hello" {
		t.Fatalf("unexpected provider message payload: %+v", p.last.Messages[0])
	}
}
