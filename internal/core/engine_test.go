package core

import (
	"context"
	"strings"
	"testing"

	"oh-my-agent/internal/provider"
)

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
