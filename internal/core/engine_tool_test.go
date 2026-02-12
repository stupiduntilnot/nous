package core

import (
	"context"
	"strings"
	"testing"

	"nous/internal/provider"
)

type scriptedProvider struct{}

func (scriptedProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if hasToolResult(req.Messages) {
			out <- provider.Event{Type: provider.EventDone}
			return
		}
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

func TestSteerQueuedSkipsRemainingToolCallsInSameAssistantMessage(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, scriptedProvider{})

	executed := make([]string, 0, 2)
	firstDone := false
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			executed = append(executed, "first")
			firstDone = true
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			executed = append(executed, "second")
			return "second-ok", nil
		}},
	})

	ctx := withSteerPendingChecker(context.Background(), func() bool {
		return firstDone
	})

	out, err := e.Prompt(ctx, "run-steer-interrupt", "go")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	if len(executed) != 1 || executed[0] != "first" {
		t.Fatalf("expected only first tool execution, got: %v", executed)
	}
	if !strings.Contains(out, "first-ok") {
		t.Fatalf("expected first tool result in output, got: %q", out)
	}
	if !strings.Contains(out, "Skipped due to queued user message.") {
		t.Fatalf("expected skipped tool marker in output, got: %q", out)
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

func (inactiveToolCallProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if hasToolResult(req.Messages) {
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "second"}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestInactiveToolCallReturnsToolErrorMessage(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, inactiveToolCallProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) { return "", nil }},
	})
	if err := e.SetActiveTools([]string{"first"}); err != nil {
		t.Fatalf("set active tools failed: %v", err)
	}
	out, err := e.Prompt(context.Background(), "run-inactive", "x")
	if err != nil {
		t.Fatalf("expected no fatal error, got: %v", err)
	}
	if !strings.Contains(out, "tool_error: tool_not_active: second") {
		t.Fatalf("unexpected output: %q", out)
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
	if len(p.calls[1].Messages) < 2 {
		t.Fatalf("expected second call to include structured messages, got: %+v", p.calls[1].Messages)
	}
	last := p.calls[1].Messages[len(p.calls[1].Messages)-1]
	if last.Role != "tool_result" || last.Content != "first => tool-ok" {
		t.Fatalf("unexpected structured message tail: %+v", last)
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

type toolCallNoAwaitProvider struct {
	calls int
}

func (p *toolCallNoAwaitProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t1", Name: "first"}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: "done"}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestToolCallWithoutAwaitStillContinuesNextTurn(t *testing.T) {
	r := NewRuntime()
	p := &toolCallNoAwaitProvider{}
	e := NewEngine(r, p)
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "tool-ok", nil
		}},
	})

	out, err := e.Prompt(context.Background(), "run-tool-no-await", "hello")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if p.calls != 2 {
		t.Fatalf("expected two provider calls, got %d", p.calls)
	}
	if out != "tool-okdone" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func hasToolResult(messages []provider.Message) bool {
	for _, msg := range messages {
		if msg.Role == "tool_result" {
			return true
		}
	}
	return false
}

type progressToolProvider struct {
	calls int
}

func (p *progressToolProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event, 4)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "t-progress", Name: "progressive"}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestProgressiveToolEmitsMultipleExecutionUpdates(t *testing.T) {
	r := NewRuntime()
	p := &progressToolProvider{}
	e := NewEngine(r, p)
	e.SetTools([]Tool{
		ProgressiveToolFunc{
			ToolName: "progressive",
			Run: func(_ context.Context, _ map[string]any, progress ToolProgressFunc) (string, error) {
				progress("10%")
				progress("50%")
				return "complete", nil
			},
		},
	})

	updates := make([]string, 0, 4)
	unsub := e.Subscribe(func(ev Event) {
		if ev.Type == EventToolExecutionUpdate && ev.ToolCallID == "t-progress" {
			updates = append(updates, ev.Delta)
		}
	})
	defer unsub()

	out, err := e.Prompt(context.Background(), "run-tool-progress", "go")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(out, "complete") {
		t.Fatalf("expected final tool result in output, got: %q", out)
	}
	if len(updates) < 3 {
		t.Fatalf("expected progress + final updates, got: %v", updates)
	}
	if updates[0] != "10%" || updates[1] != "50%" {
		t.Fatalf("unexpected progress update order: %v", updates)
	}
	if updates[len(updates)-1] != "complete" {
		t.Fatalf("expected final update to carry tool result, got: %v", updates)
	}
}

func TestNonProgressToolStillEmitsSingleExecutionUpdate(t *testing.T) {
	r := NewRuntime()
	p := &progressToolProvider{}
	e := NewEngine(r, p)
	e.SetTools([]Tool{
		ToolFunc{
			ToolName: "progressive",
			Run: func(_ context.Context, _ map[string]any) (string, error) {
				return "complete", nil
			},
		},
	})

	updates := make([]string, 0, 2)
	unsub := e.Subscribe(func(ev Event) {
		if ev.Type == EventToolExecutionUpdate && ev.ToolCallID == "t-progress" {
			updates = append(updates, ev.Delta)
		}
	})
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-tool-single-update", "go"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if len(updates) != 1 || updates[0] != "complete" {
		t.Fatalf("expected single final update for non-progress tool, got: %v", updates)
	}
}
