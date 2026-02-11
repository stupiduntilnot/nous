package core

import (
	"context"
	"strings"
	"testing"

	"oh-my-agent/internal/extension"
	"oh-my-agent/internal/provider"
)

type capturePromptProvider struct {
	last provider.Request
}

func (c *capturePromptProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	c.last = req
	out := make(chan provider.Event, 2)
	out <- provider.Event{Type: provider.EventTextDelta, Delta: "ok"}
	out <- provider.Event{Type: provider.EventDone}
	close(out)
	return out
}

func TestEngineAppliesInputHookBeforeProviderCall(t *testing.T) {
	r := NewRuntime()
	p := &capturePromptProvider{}
	e := NewEngine(r, p)
	m := extension.NewManager()
	m.RegisterInputHook(func(in extension.InputHookInput) (extension.InputHookOutput, error) {
		return extension.InputHookOutput{Text: strings.ToUpper(in.Text)}, nil
	})
	e.SetExtensionManager(m)

	if _, err := e.Prompt(context.Background(), "run-ext-1", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if p.last.Prompt != "HELLO" {
		t.Fatalf("input hook did not transform prompt, got %q", p.last.Prompt)
	}
}

func TestEngineToolCallBlockedByExtension(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, scriptedProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "second-ok", nil
		}},
	})

	m := extension.NewManager()
	m.RegisterToolCallHook(func(in extension.ToolCallHookInput) (extension.ToolCallHookOutput, error) {
		if in.ToolName == "second" {
			return extension.ToolCallHookOutput{Blocked: true, Reason: "policy"}, nil
		}
		return extension.ToolCallHookOutput{}, nil
	})
	e.SetExtensionManager(m)

	if _, err := e.Prompt(context.Background(), "run-ext-2", "go"); err == nil {
		t.Fatalf("expected blocked tool call to fail")
	}
}

func TestEngineToolResultMutatedByExtension(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, scriptedProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "second-ok", nil
		}},
	})

	m := extension.NewManager()
	m.RegisterToolResultHook(func(in extension.ToolResultHookInput) (extension.ToolResultHookOutput, error) {
		return extension.ToolResultHookOutput{Result: strings.ToUpper(in.Result)}, nil
	})
	e.SetExtensionManager(m)

	got, err := e.Prompt(context.Background(), "run-ext-3", "go")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if got != "FIRST-OKSECOND-OK" {
		t.Fatalf("unexpected mutated output: %q", got)
	}
}

func TestEngineRunsTurnEndHook(t *testing.T) {
	r := NewRuntime()
	p := provider.NewMockAdapter()
	e := NewEngine(r, p)

	m := extension.NewManager()
	called := false
	m.RegisterTurnEndHook(func(in extension.TurnEndHookInput) error {
		if in.RunID == "run-ext-4" && in.Turn == 1 {
			called = true
		}
		return nil
	})
	e.SetExtensionManager(m)

	if _, err := e.Prompt(context.Background(), "run-ext-4", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !called {
		t.Fatalf("expected turn_end hook to be called")
	}
}
