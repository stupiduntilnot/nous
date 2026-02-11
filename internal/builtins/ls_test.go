package builtins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/provider"
)

func TestLSToolListsDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "a-dir"), 0o755); err != nil {
		t.Fatalf("mkdir fixture failed: %v", err)
	}

	out, err := NewLSTool(dir).Execute(context.Background(), map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode ls output failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(got), got)
	}
	if got[0]["name"] != "a-dir" || got[0]["type"] != "dir" {
		t.Fatalf("unexpected first entry: %v", got[0])
	}
	if got[1]["name"] != "b.txt" || got[1]["type"] != "file" {
		t.Fatalf("unexpected second entry: %v", got[1])
	}
}

func TestLSToolEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	out, err := NewLSTool(dir).Execute(context.Background(), map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("ls failed: %v", err)
	}
	if out != "[]" {
		t.Fatalf("expected empty json array, got: %q", out)
	}
}

func TestLSToolPathErrors(t *testing.T) {
	dir := t.TempDir()
	tool := NewLSTool(dir)

	if _, err := tool.Execute(context.Background(), map[string]any{"path": "missing"}); err == nil || !strings.Contains(err.Error(), "ls_failed") {
		t.Fatalf("expected ls_failed for missing path, got: %v", err)
	}

	filePath := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": filePath}); err == nil || err.Error() != "ls_not_directory" {
		t.Fatalf("expected ls_not_directory, got: %v", err)
	}
}

type lsToolCallProvider struct{}

func (lsToolCallProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
			ID:   "t-ls",
			Name: "ls",
			Arguments: map[string]any{
				"path": ".",
			},
		}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestLSToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "only.txt"), []byte("z"), 0o644); err != nil {
		t.Fatalf("write fixture failed: %v", err)
	}

	engine := core.NewEngine(core.NewRuntime(), lsToolCallProvider{})
	engine.SetTools(DefaultTools(dir))

	out, err := engine.Prompt(context.Background(), "run-ls-tool", "list files")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(out, "\"name\":\"only.txt\"") {
		t.Fatalf("unexpected ls output: %q", out)
	}
}
