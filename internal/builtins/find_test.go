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

func TestFindToolMatchesBySubstring(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "x", "y"))
	mustWrite(t, filepath.Join(dir, "x", "alpha.txt"), "a")
	mustWrite(t, filepath.Join(dir, "x", "y", "beta.txt"), "b")

	out, err := NewFindTool(dir).Execute(context.Background(), map[string]any{
		"path":  ".",
		"query": ".txt",
	})
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	var got []string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %v", len(got), got)
	}
	if got[0] != filepath.Join("x", "alpha.txt") || got[1] != filepath.Join("x", "y", "beta.txt") {
		t.Fatalf("unexpected matches: %v", got)
	}
}

func TestFindToolMaxDepth(t *testing.T) {
	dir := t.TempDir()
	mustMkdirAll(t, filepath.Join(dir, "x", "y"))
	mustWrite(t, filepath.Join(dir, "x", "one.txt"), "1")
	mustWrite(t, filepath.Join(dir, "x", "y", "two.txt"), "2")

	out, err := NewFindTool(dir).Execute(context.Background(), map[string]any{
		"path":      ".",
		"query":     ".txt",
		"max_depth": 1,
	})
	if err != nil {
		t.Fatalf("find failed: %v", err)
	}
	var got []string
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no matches at depth 1, got %v", got)
	}
}

func TestFindToolErrors(t *testing.T) {
	dir := t.TempDir()
	tool := NewFindTool(dir)

	if _, err := tool.Execute(context.Background(), map[string]any{"path": ".", "query": ""}); err == nil || err.Error() != "find_invalid_query" {
		t.Fatalf("expected find_invalid_query, got %v", err)
	}
	if _, err := tool.Execute(context.Background(), map[string]any{"path": "missing", "query": "x"}); err == nil || !strings.Contains(err.Error(), "find_failed") {
		t.Fatalf("expected find_failed, got %v", err)
	}
	filePath := filepath.Join(dir, "a.txt")
	mustWrite(t, filePath, "x")
	if _, err := tool.Execute(context.Background(), map[string]any{"path": filePath, "query": "x"}); err == nil || err.Error() != "find_not_directory" {
		t.Fatalf("expected find_not_directory, got %v", err)
	}
}

type findToolCallProvider struct{}

func (findToolCallProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if len(req.ToolResults) > 0 {
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{
			ID:   "t-find",
			Name: "find",
			Arguments: map[string]any{
				"path":  ".",
				"query": ".txt",
			},
		}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestFindToolWithEngineLoop(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "ok")

	engine := core.NewEngine(core.NewRuntime(), findToolCallProvider{})
	engine.SetTools(DefaultTools(dir))
	out, err := engine.Prompt(context.Background(), "run-find-tool", "find files")
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !strings.Contains(out, "a.txt") {
		t.Fatalf("unexpected find output: %q", out)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", path, err)
	}
}
