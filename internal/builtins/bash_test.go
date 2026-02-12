package builtins

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"nous/internal/core"
	"nous/internal/provider"
)

func TestBashToolExecutesCommand(t *testing.T) {
	tool := NewBashTool(t.TempDir())
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "printf 'OK'",
	})
	if err != nil {
		t.Fatalf("bash execute failed: %v", err)
	}
	if out != "OK" {
		t.Fatalf("unexpected bash output: %q", out)
	}
}

func TestBashToolErrors(t *testing.T) {
	tool := NewBashTool(t.TempDir())
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil || err.Error() != "bash_invalid_command" {
		t.Fatalf("expected bash_invalid_command, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"command": "printf x", "timeout": -1}); err == nil || err.Error() != "bash_invalid_timeout" {
		t.Fatalf("expected bash_invalid_timeout, got: %v", err)
	}
}

func TestBashToolTimeout(t *testing.T) {
	tool := NewBashTool(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if _, err := tool.Execute(ctx, map[string]any{
		"command": "sleep 2",
		"timeout": 1,
	}); err == nil || !strings.Contains(err.Error(), "Command timed out after 1 seconds") {
		t.Fatalf("expected timeout message, got: %v", err)
	}
}

func TestBashToolTruncatesOutputAndWritesTempFile(t *testing.T) {
	tool := NewBashTool(t.TempDir())
	out, err := tool.Execute(context.Background(), map[string]any{
		"command": "for i in {1..2200}; do printf \"line-%s\\n\" \"$i\"; done",
	})
	if err != nil {
		t.Fatalf("bash execute failed: %v", err)
	}
	if !strings.Contains(out, "line-2200") {
		t.Fatalf("expected tail output, got: %q", out)
	}
	if !strings.Contains(out, "[Showing lines ") || !strings.Contains(out, "Full output: ") || !strings.Contains(out, "pi-bash-") {
		t.Fatalf("expected truncation notice with full output path, got: %q", out)
	}
	start := strings.LastIndex(out, "Full output: ")
	if start == -1 {
		t.Fatalf("missing full output path in: %q", out)
	}
	path := strings.TrimSuffix(out[start+len("Full output: "):], "]")
	path = strings.TrimSpace(path)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected temp file to exist at %q: %v", path, err)
	}
}

type bashToolCallProvider struct {
	calls int
}

func (p *bashToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
				ID:   "t-bash",
				Name: "bash",
				Arguments: map[string]any{
					"command": "printf 'BASH_OK'",
				},
			}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestBashToolWithEngineLoop(t *testing.T) {
	engine := core.NewEngine(core.NewRuntime(), &bashToolCallProvider{})
	engine.SetTools(DefaultTools(t.TempDir()))

	out, err := engine.Prompt(context.Background(), "run-bash-tool", "run bash")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(out, "BASH_OK") {
		t.Fatalf("unexpected output: %q", out)
	}
}
