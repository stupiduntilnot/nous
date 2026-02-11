package main

import (
	"testing"

	"oh-my-agent/internal/extension"
)

func TestRegisterDemoExtension(t *testing.T) {
	m := extension.NewManager()
	registerDemoExtension(m)

	out, handled, err := m.ExecuteCommand("echo", map[string]any{"text": "hi"})
	if err != nil || !handled {
		t.Fatalf("demo command execute failed: out=%v handled=%v err=%v", out, handled, err)
	}
	if got, _ := out["echo"].(string); got != "hi" {
		t.Fatalf("unexpected command output: %+v", out)
	}

	toolOut, handled, err := m.ExecuteTool("demo.echo", map[string]any{"text": "x"})
	if err != nil || !handled {
		t.Fatalf("demo tool execute failed: out=%q handled=%v err=%v", toolOut, handled, err)
	}
	if toolOut != "demo.echo:x" {
		t.Fatalf("unexpected tool output: %q", toolOut)
	}
}
