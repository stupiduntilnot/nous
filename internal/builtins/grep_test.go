package builtins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nous/internal/core"
	"nous/internal/provider"
)

func TestGrepToolMatchesDirectory(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "hello\nTODO: one\nbye")
	mustWrite(t, filepath.Join(dir, "b.txt"), "TODO: two\nok")

	out, err := NewGrepTool(dir).Execute(context.Background(), map[string]any{
		"pattern": "TODO",
		"path":    ".",
	})
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	if !strings.Contains(out, "a.txt:2: TODO: one") {
		t.Fatalf("missing match in a.txt: %q", out)
	}
	if !strings.Contains(out, "b.txt:1: TODO: two") {
		t.Fatalf("missing match in b.txt: %q", out)
	}
}

func TestGrepToolIgnoreCaseAndLimit(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "todo\nTODO\nToDo")

	out, err := NewGrepTool(dir).Execute(context.Background(), map[string]any{
		"pattern":     "todo",
		"ignore_case": true,
		"limit":       2.0,
	})
	if err != nil {
		t.Fatalf("grep failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 matches, got %d: %q", len(lines), out)
	}
}

func TestGrepToolErrors(t *testing.T) {
	dir := t.TempDir()
	tool := NewGrepTool(dir)

	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil || err.Error() != "grep_invalid_pattern" {
		t.Fatalf("expected grep_invalid_pattern, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"pattern": "x", "limit": 0}); err == nil || err.Error() != "grep_invalid_limit" {
		t.Fatalf("expected grep_invalid_limit, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"pattern": "[", "path": "."}); err == nil || err.Error() != "grep_invalid_pattern" {
		t.Fatalf("expected grep_invalid_pattern for invalid regex, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"pattern": "x", "path": "missing"}); err == nil || !strings.Contains(err.Error(), "grep_failed") {
		t.Fatalf("expected grep_failed, got: %v", err)
	}
}

type grepToolCallProvider struct{}

func (grepToolCallProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if hasToolResult(req.Messages) {
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
			ID:   "t-grep",
			Name: "grep",
			Arguments: map[string]any{
				"pattern": "TODO",
				"path":    ".",
			},
		}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestGrepToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("TODO: ship\n"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	engine := core.NewEngine(core.NewRuntime(), grepToolCallProvider{})
	engine.SetTools(DefaultTools(dir))

	out, err := engine.Prompt(context.Background(), "run-grep-tool", "find todo")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(out, "note.txt:1: TODO: ship") {
		t.Fatalf("unexpected grep output: %q", out)
	}
}
