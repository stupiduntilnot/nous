package core

import (
	"context"
	"strings"
	"testing"

	"oh-my-agent/internal/provider"
)

type scriptedProvider struct{}

func (scriptedProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
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

type captureProvider struct {
	last provider.Request
}

func (c *captureProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	c.last = req
	out := make(chan provider.Event, 1)
	out <- provider.Event{Type: provider.EventDone}
	close(out)
	return out
}

func TestSetActiveToolsFiltersProviderPayload(t *testing.T) {
	r := NewRuntime()
	p := &captureProvider{}
	e := NewEngine(r, p)
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
	})

	if err := e.SetActiveTools([]string{"second"}); err != nil {
		t.Fatalf("set active tools failed: %v", err)
	}
	if _, err := e.Prompt(context.Background(), "run-active", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if len(p.last.ActiveTools) != 1 || p.last.ActiveTools[0] != "second" {
		t.Fatalf("unexpected active tools payload: %+v", p.last.ActiveTools)
	}
}

type inactiveToolCallProvider struct{}

func (inactiveToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "second"}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestInactiveToolCallIsRejected(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, inactiveToolCallProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
	})
	if err := e.SetActiveTools([]string{"first"}); err != nil {
		t.Fatalf("set active tools failed: %v", err)
	}
	if _, err := e.Prompt(context.Background(), "run-inactive", "x"); err == nil {
		t.Fatalf("expected inactive tool call to fail")
	}
}

type awaitNextProvider struct {
	calls []provider.Request
}

func (p *awaitNextProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	p.calls = append(p.calls, req)
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if len(p.calls) == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "first"}}
			out <- provider.Event{Type: provider.EventAwaitNext}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: "final-answer"}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestAwaitNextTurnLoopsWithToolResults(t *testing.T) {
	r := NewRuntime()
	p := &awaitNextProvider{}
	e := NewEngine(r, p)
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-ok", nil
		}},
	})

	out, err := e.Prompt(context.Background(), "run-await-next", "hello")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if len(p.calls) != 2 {
		t.Fatalf("expected two provider calls, got %d", len(p.calls))
	}
	if len(p.calls[1].ToolResults) != 1 || p.calls[1].ToolResults[0] != "first => tool-ok" {
		t.Fatalf("unexpected tool results in second call: %+v", p.calls[1].ToolResults)
	}
	if !strings.Contains(p.calls[1].Prompt, "Tool results:") {
		t.Fatalf("expected second prompt to include tool results, got: %q", p.calls[1].Prompt)
	}
	if out != "tool-okfinal-answer" {
		t.Fatalf("unexpected output: %q", out)
	}
}

type infiniteAwaitProvider struct{}

func (p infiniteAwaitProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "first"}}
		out <- provider.Event{Type: provider.EventAwaitNext}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestAwaitNextTurnLimitExceeded(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, infiniteAwaitProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-ok", nil
		}},
	})

	if _, err := e.Prompt(context.Background(), "run-await-limit", "hello"); err == nil {
		t.Fatalf("expected loop limit error")
	} else if err.Error() != "tool_loop_limit_exceeded" {
		t.Fatalf("unexpected error: %v", err)
	}
}
