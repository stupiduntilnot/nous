package extension

import (
	"strings"
	"testing"
)

func TestInputHooksTransformAndHandled(t *testing.T) {
	m := NewManager()

	m.RegisterInputHook(func(in InputHookInput) (InputHookOutput, error) {
		return InputHookOutput{Text: strings.TrimSpace(in.Text) + " world"}, nil
	})
	thirdCalled := false
	m.RegisterInputHook(func(in InputHookInput) (InputHookOutput, error) {
		return InputHookOutput{Text: in.Text + "!", Handled: true}, nil
	})
	m.RegisterInputHook(func(in InputHookInput) (InputHookOutput, error) {
		thirdCalled = true
		return InputHookOutput{Text: in.Text + " should-not-run"}, nil
	})

	out, err := m.RunInputHooks(" hello ")
	if err != nil {
		t.Fatalf("run input hooks failed: %v", err)
	}
	if !out.Handled {
		t.Fatalf("expected handled=true")
	}
	if out.Text != "hello world!" {
		t.Fatalf("unexpected transformed text: %q", out.Text)
	}
	if thirdCalled {
		t.Fatalf("expected hook chain to stop after handled=true")
	}
}

func TestToolCallHookBlock(t *testing.T) {
	m := NewManager()
	m.RegisterToolCallHook(func(in ToolCallHookInput) (ToolCallHookOutput, error) {
		if in.ToolName == "dangerous" {
			return ToolCallHookOutput{Blocked: true, Reason: "policy_denied"}, nil
		}
		return ToolCallHookOutput{}, nil
	})

	out, err := m.RunToolCallHooks("dangerous", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("run tool call hooks failed: %v", err)
	}
	if !out.Blocked || out.Reason != "policy_denied" {
		t.Fatalf("unexpected block decision: %+v", out)
	}
}

func TestToolResultHookMutate(t *testing.T) {
	m := NewManager()
	m.RegisterToolResultHook(func(in ToolResultHookInput) (ToolResultHookOutput, error) {
		return ToolResultHookOutput{Result: strings.ToUpper(in.Result)}, nil
	})
	m.RegisterToolResultHook(func(in ToolResultHookInput) (ToolResultHookOutput, error) {
		return ToolResultHookOutput{Result: "[" + in.Result + "]"}, nil
	})

	out, err := m.RunToolResultHooks("echo", "ok")
	if err != nil {
		t.Fatalf("run tool result hooks failed: %v", err)
	}
	if out.Result != "[OK]" {
		t.Fatalf("unexpected mutated result: %q", out.Result)
	}
}

func TestTurnEndHookInvoked(t *testing.T) {
	m := NewManager()
	invoked := false
	m.RegisterTurnEndHook(func(in TurnEndHookInput) error {
		if in.RunID == "run-1" && in.Turn == 3 {
			invoked = true
		}
		return nil
	})

	if err := m.RunTurnEndHooks("run-1", 3); err != nil {
		t.Fatalf("run turn end hooks failed: %v", err)
	}
	if !invoked {
		t.Fatalf("expected turn end hook to be invoked")
	}
}

func TestRunStartAndRunEndHooksInvoked(t *testing.T) {
	m := NewManager()
	startCalled := false
	endCalled := false
	m.RegisterRunStartHook(func(in RunStartHookInput) error {
		if in.RunID == "run-1" {
			startCalled = true
		}
		return nil
	})
	m.RegisterRunEndHook(func(in RunEndHookInput) error {
		if in.RunID == "run-1" && in.Turn == 2 {
			endCalled = true
		}
		return nil
	})

	if err := m.RunRunStartHooks("run-1"); err != nil {
		t.Fatalf("run_start hooks failed: %v", err)
	}
	if err := m.RunRunEndHooks("run-1", 2); err != nil {
		t.Fatalf("run_end hooks failed: %v", err)
	}
	if !startCalled || !endCalled {
		t.Fatalf("expected run hooks to be invoked: start=%v end=%v", startCalled, endCalled)
	}
}

func TestRegisterToolAndCommand(t *testing.T) {
	m := NewManager()
	if err := m.RegisterTool("echo", func(args map[string]any) (string, error) { return "ok", nil }); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}
	if err := m.RegisterCommand("hello", func(payload map[string]any) (map[string]any, error) {
		return map[string]any{"ok": true}, nil
	}); err != nil {
		t.Fatalf("register command failed: %v", err)
	}
	if err := m.RegisterTool("", nil); err == nil {
		t.Fatalf("expected invalid tool registration to fail")
	}
	if err := m.RegisterCommand("", nil); err == nil {
		t.Fatalf("expected invalid command registration to fail")
	}
}

func TestExecuteRegisteredToolAndCommand(t *testing.T) {
	m := NewManager()
	if err := m.RegisterTool("echo", func(args map[string]any) (string, error) {
		in, _ := args["text"].(string)
		return "tool:" + in, nil
	}); err != nil {
		t.Fatalf("register tool failed: %v", err)
	}
	if err := m.RegisterCommand("hello", func(payload map[string]any) (map[string]any, error) {
		name, _ := payload["name"].(string)
		return map[string]any{"greeting": "hi " + name}, nil
	}); err != nil {
		t.Fatalf("register command failed: %v", err)
	}

	toolOut, handled, err := m.ExecuteTool("echo", map[string]any{"text": "x"})
	if err != nil || !handled || toolOut != "tool:x" {
		t.Fatalf("unexpected tool execute result: out=%q handled=%v err=%v", toolOut, handled, err)
	}
	_, handled, err = m.ExecuteTool("missing", map[string]any{})
	if err != nil || handled {
		t.Fatalf("missing tool should not be handled: handled=%v err=%v", handled, err)
	}

	cmdOut, handled, err := m.ExecuteCommand("hello", map[string]any{"name": "dev"})
	if err != nil || !handled {
		t.Fatalf("unexpected command execute result: out=%v handled=%v err=%v", cmdOut, handled, err)
	}
	if got, _ := cmdOut["greeting"].(string); got != "hi dev" {
		t.Fatalf("unexpected command payload: %v", cmdOut)
	}
	_, handled, err = m.ExecuteCommand("missing", map[string]any{})
	if err != nil || handled {
		t.Fatalf("missing command should not be handled: handled=%v err=%v", handled, err)
	}
}
