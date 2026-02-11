package core

import (
	"context"
	"testing"

	"oh-my-agent/internal/provider"
)

type scriptedProvider struct{}

func (scriptedProvider) Stream(_ context.Context, _ string) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventStart}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "first", Arguments: map[string]any{}}}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t2", Name: "second", Arguments: map[string]any{}}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestToolCallSequentialLoop(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, scriptedProvider{})

	order := make([]string, 0, 2)
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			order = append(order, "first")
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			order = append(order, "second")
			return "second-ok", nil
		}},
	})

	result, err := e.Prompt(context.Background(), "run-tools", "go")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("tools not executed sequentially: %v", order)
	}
	if result != "first-oksecond-ok" {
		t.Fatalf("unexpected final result: %q", result)
	}
}
