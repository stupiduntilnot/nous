package core

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"nous/internal/extension"
	"nous/internal/provider"
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
	if len(p.last.Messages) != 1 || p.last.Messages[0].Role != "user" || p.last.Messages[0].Content != "HELLO" {
		t.Fatalf("structured messages not populated from transformed prompt: %+v", p.last.Messages)
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

func TestEngineRunsRunLifecycleHooks(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, provider.NewMockAdapter())

	m := extension.NewManager()
	startCalled := false
	endCalled := false
	m.RegisterRunStartHook(func(in extension.RunStartHookInput) error {
		if in.RunID == "run-ext-lifecycle" {
			startCalled = true
		}
		return nil
	})
	m.RegisterRunEndHook(func(in extension.RunEndHookInput) error {
		if in.RunID == "run-ext-lifecycle" && in.Turn == 1 {
			endCalled = true
		}
		return nil
	})
	e.SetExtensionManager(m)

	if _, err := e.Prompt(context.Background(), "run-ext-lifecycle", "hello"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !startCalled || !endCalled {
		t.Fatalf("expected run lifecycle hooks to be called: start=%v end=%v", startCalled, endCalled)
	}
}

func TestEngineIsolatesLifecycleHookErrorsAsWarnings(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, provider.NewMockAdapter())

	m := extension.NewManager()
	m.RegisterRunStartHook(func(extension.RunStartHookInput) error {
		return fmt.Errorf("start failed")
	})
	m.RegisterTurnEndHook(func(extension.TurnEndHookInput) error {
		return fmt.Errorf("turn failed")
	})
	m.RegisterRunEndHook(func(extension.RunEndHookInput) error {
		return fmt.Errorf("end failed")
	})
	e.SetExtensionManager(m)

	events := make([]Event, 0, 8)
	unsub := e.Subscribe(func(ev Event) {
		events = append(events, ev)
	})
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-ext-hook-isolation", "hello"); err != nil {
		t.Fatalf("prompt should not fail on lifecycle hook errors: %v", err)
	}

	warnings := make([]string, 0, 3)
	for _, ev := range events {
		if ev.Type == EventWarning && ev.Code == "extension_hook_error" {
			warnings = append(warnings, ev.Message)
		}
	}
	if len(warnings) < 3 {
		t.Fatalf("expected lifecycle hook warnings, got: %+v", warnings)
	}
	joined := strings.Join(warnings, "\n")
	for _, want := range []string{"run_start:", "turn_end:", "run_end:"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing warning for %s in %q", want, joined)
		}
	}
}

func TestEngineExecutesExtensionRegisteredTool(t *testing.T) {
	r := NewRuntime()
	e := NewEngine(r, scriptedProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "first-ok", nil
		}},
	})

	m := extension.NewManager()
	if err := m.RegisterTool("second", func(args map[string]any) (string, error) {
		return "second-from-ext", nil
	}); err != nil {
		t.Fatalf("register extension tool failed: %v", err)
	}
	e.SetExtensionManager(m)

	got, err := e.Prompt(context.Background(), "run-ext-tool", "go")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if got != "first-oksecond-from-ext" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestEngineExecutesExtensionCommand(t *testing.T) {
	e := NewEngine(NewRuntime(), provider.NewMockAdapter())
	m := extension.NewManager()
	if err := m.RegisterCommand("hello", func(payload map[string]any) (map[string]any, error) {
		name, _ := payload["name"].(string)
		return map[string]any{"msg": "hi " + name}, nil
	}); err != nil {
		t.Fatalf("register extension command failed: %v", err)
	}
	e.SetExtensionManager(m)

	out, err := e.ExecuteExtensionCommand("hello", map[string]any{"name": "agent"})
	if err != nil {
		t.Fatalf("execute extension command failed: %v", err)
	}
	if got, _ := out["msg"].(string); got != "hi agent" {
		t.Fatalf("unexpected extension command output: %v", out)
	}
}
