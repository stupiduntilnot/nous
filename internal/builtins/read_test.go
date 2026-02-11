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

func TestReadToolReadsTextFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("l1\nl2\nl3\n"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	got, err := NewReadTool(dir).Execute(context.Background(), map[string]any{"path": p})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got != "l1\nl2\nl3" {
		t.Fatalf("unexpected read output: %q", got)
	}
}

func TestReadToolResolvesRelativePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	got, err := NewReadTool(dir).Execute(context.Background(), map[string]any{"path": "a.txt"})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got != "hello" {
		t.Fatalf("unexpected read output: %q", got)
	}
}

func TestReadToolOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\nb\nc\nd"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	got, err := NewReadTool(dir).Execute(context.Background(), map[string]any{
		"path":   "a.txt",
		"offset": 1.0,
		"limit":  2.0,
	})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if got != "b\nc" {
		t.Fatalf("unexpected read output: %q", got)
	}
}

func TestReadToolNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := NewReadTool(dir).Execute(context.Background(), map[string]any{"path": "missing.txt"})
	if err == nil || !strings.Contains(err.Error(), "read_failed") {
		t.Fatalf("expected read_failed, got: %v", err)
	}
}

func TestReadToolRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := NewReadTool(dir).Execute(context.Background(), map[string]any{"path": dir})
	if err == nil || err.Error() != "read_is_directory" {
		t.Fatalf("expected read_is_directory, got: %v", err)
	}
}

func TestReadToolRejectsInvalidArgs(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadTool(dir)
	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil || err.Error() != "read_invalid_path" {
		t.Fatalf("expected read_invalid_path, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "x", "offset": -1}); err == nil || err.Error() != "read_invalid_offset" {
		t.Fatalf("expected read_invalid_offset, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "x", "limit": 0}); err == nil || err.Error() != "read_invalid_limit" {
		t.Fatalf("expected read_invalid_limit, got: %v", err)
	}
}

type readToolCallProvider struct{}

func (readToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
			ID:   "t-read",
			Name: "read",
			Arguments: map[string]any{
				"path": "note.txt",
			},
		}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestReadToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello from read"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	engine := core.NewEngine(core.NewRuntime(), readToolCallProvider{})
	engine.SetTools(DefaultTools(dir))

	out, err := engine.Prompt(context.Background(), "run-read-tool", "read this file")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if out != "hello from read" {
		t.Fatalf("unexpected read tool output: %q", out)
	}
}
