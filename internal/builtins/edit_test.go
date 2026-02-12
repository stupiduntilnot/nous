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

func TestEditToolReplacesUniqueText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	out, err := NewEditTool(dir).Execute(context.Background(), map[string]any{
		"path":    "a.txt",
		"oldText": "world",
		"newText": "agent",
	})
	if err != nil {
		t.Fatalf("edit failed: %v", err)
	}
	if !strings.Contains(out, "edited a.txt") {
		t.Fatalf("unexpected edit output: %q", out)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read edited file failed: %v", err)
	}
	if string(b) != "hello agent" {
		t.Fatalf("unexpected edited content: %q", string(b))
	}
}

func TestEditToolErrors(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("x x"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	tool := NewEditTool(dir)

	if _, err := tool.Execute(context.Background(), map[string]any{}); err == nil || err.Error() != "edit_invalid_path" {
		t.Fatalf("expected edit_invalid_path, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt"}); err == nil || err.Error() != "edit_invalid_old_text" {
		t.Fatalf("expected edit_invalid_old_text, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt", "oldText": "x"}); err == nil || err.Error() != "edit_invalid_new_text" {
		t.Fatalf("expected edit_invalid_new_text, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt", "oldText": "z", "newText": "k"}); err == nil || err.Error() != "edit_old_text_not_found" {
		t.Fatalf("expected edit_old_text_not_found, got: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "a.txt", "oldText": "x", "newText": "k"}); err == nil || err.Error() != "edit_old_text_not_unique" {
		t.Fatalf("expected edit_old_text_not_unique, got: %v", err)
	}
}

type editToolCallProvider struct {
	calls int
}

func (p *editToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
				ID:   "t-edit",
				Name: "edit",
				Arguments: map[string]any{
					"path":    "note.txt",
					"oldText": "before",
					"newText": "after",
				},
			}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestEditToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("before"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	engine := core.NewEngine(core.NewRuntime(), &editToolCallProvider{})
	engine.SetTools(DefaultTools(dir))
	if _, err := engine.Prompt(context.Background(), "run-edit-tool", "edit file"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "note.txt"))
	if err != nil {
		t.Fatalf("read edited file failed: %v", err)
	}
	if string(b) != "after" {
		t.Fatalf("unexpected final content: %q", string(b))
	}
}
