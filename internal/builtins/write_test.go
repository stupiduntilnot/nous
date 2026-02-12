package builtins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/provider"
)

func TestWriteToolWritesFile(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteTool(dir)

	out, err := tool.Execute(context.Background(), map[string]any{
		"path":    "a.txt",
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.Contains(out, "wrote 5 bytes") {
		t.Fatalf("unexpected write output: %q", out)
	}
	b, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("unexpected file content: %q", string(b))
	}
}

func TestWriteToolErrors(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteTool(dir)

	if _, err := tool.Execute(context.Background(), map[string]any{"content": "x"}); err == nil || err.Error() != "write_invalid_path" {
		t.Fatalf("expected write_invalid_path, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt"}); err == nil || err.Error() != "write_invalid_content" {
		t.Fatalf("expected write_invalid_content, got: %v", err)
	}
}

type writeToolCallProvider struct {
	calls int
}

func (p *writeToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
				ID:   "t-write",
				Name: "write",
				Arguments: map[string]any{
					"path":    "out.txt",
					"content": "hello-write",
				},
			}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestWriteToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	p := &writeToolCallProvider{}
	engine := core.NewEngine(core.NewRuntime(), p)
	engine.SetTools(DefaultTools(dir))

	if _, err := engine.Prompt(context.Background(), "run-write-tool", "write file"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("read out file failed: %v", err)
	}
	if string(b) != "hello-write" {
		t.Fatalf("unexpected output file content: %q", string(b))
	}
}
