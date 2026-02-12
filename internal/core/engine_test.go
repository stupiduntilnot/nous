package core

import (
	"context"
	"strings"
	"testing"

	"nous/internal/provider"
)

type captureRequestProvider struct {
	last provider.Request
}

func (c *captureRequestProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	c.last = req
	out := make(chan provider.Event, 2)
	out <- provider.Event{Type: provider.EventTextDelta, Delta: "ok"}
	out <- provider.Event{Type: provider.EventDone}
	close(out)
	return out
}

func TestPromptWithFakeProvider(t *testing.T) {
	r := NewRuntime()
	p := provider.NewMockAdapter()
	e := NewEngine(r, p)

	var events []EventType
	r.Subscribe(func(ev Event) {
		events = append(events, ev.Type)
	})

	result, err := e.Prompt(context.Background(), "run-test", "hello")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(result, "mock response") {
		t.Fatalf("unexpected result: %q", result)
	}

	want := []EventType{
		EventAgentStart,
		EventTurnStart,
		EventMessageStart,
		EventMessageEnd,
		EventMessageStart,
		EventMessageUpdate,
		EventMessageEnd,
		EventTurnEnd,
		EventAgentEnd,
	}
	if len(events) != len(want) {
		t.Fatalf("unexpected events len: got=%d want=%d events=%v", len(events), len(want), events)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("unexpected event at %d: got=%s want=%s", i, events[i], want[i])
		}
	}
}

func TestPromptWithinExternalRunEmitsSingleRunLifecycle(t *testing.T) {
	r := NewRuntime()
	p := provider.NewMockAdapter()
	e := NewEngine(r, p)

	var events []EventType
	r.Subscribe(func(ev Event) {
		events = append(events, ev.Type)
	})

	const runID = "run-external"
	if err := e.BeginRun(runID); err != nil {
		t.Fatalf("begin run failed: %v", err)
	}
	if _, err := e.Prompt(context.Background(), runID, "hello"); err != nil {
		t.Fatalf("first prompt failed: %v", err)
	}
	if _, err := e.Prompt(context.Background(), runID, "follow up"); err != nil {
		t.Fatalf("second prompt failed: %v", err)
	}
	if err := e.EndRun(runID); err != nil {
		t.Fatalf("end run failed: %v", err)
	}

	count := func(kind EventType) int {
		n := 0
		for _, ev := range events {
			if ev == kind {
				n++
			}
		}
		return n
	}
	if got := count(EventAgentStart); got != 1 {
		t.Fatalf("agent_start count mismatch: got=%d events=%v", got, events)
	}
	if got := count(EventAgentEnd); got != 1 {
		t.Fatalf("agent_end count mismatch: got=%d events=%v", got, events)
	}
	if got := count(EventTurnStart); got != 2 {
		t.Fatalf("turn_start count mismatch: got=%d events=%v", got, events)
	}
	if got := count(EventTurnEnd); got != 2 {
		t.Fatalf("turn_end count mismatch: got=%d events=%v", got, events)
	}
}

func TestPromptFallbackRendersUnsupportedBlocks(t *testing.T) {
	r := NewRuntime()
	p := &captureRequestProvider{}
	e := NewEngine(r, p)

	e.SetTransformContext(func(_ context.Context, messages []Message) ([]Message, error) {
		return append(messages, Message{
			Role: RoleUser,
			Blocks: []MessageBlock{
				{Type: "unknown_block", Text: "fallback-text"},
			},
		}), nil
	})

	if _, err := e.Prompt(context.Background(), "run-block-fallback", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if len(p.last.Messages) < 2 {
		t.Fatalf("expected transformed message in provider payload, got: %+v", p.last.Messages)
	}
	last := p.last.Messages[len(p.last.Messages)-1]
	if last.Content != "fallback-text" {
		t.Fatalf("expected fallback block rendering, got: %+v", last)
	}
}
