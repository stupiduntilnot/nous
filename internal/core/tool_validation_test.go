package core

import "testing"

func TestNormalizeReadArgsAcceptsAliasAndCoercesNumbers(t *testing.T) {
	got, err := normalizeToolArguments("read", map[string]any{
		"filePath": "/tmp/txt",
		"offset":   "2",
		"limit":    "10",
	})
	if err != nil {
		t.Fatalf("normalize read args failed: %v", err)
	}
	if p, _ := got["path"].(string); p != "/tmp/txt" {
		t.Fatalf("unexpected path: %#v", got["path"])
	}
	if off, _ := got["offset"].(int); off != 2 {
		t.Fatalf("unexpected offset: %#v", got["offset"])
	}
	if lim, _ := got["limit"].(int); lim != 10 {
		t.Fatalf("unexpected limit: %#v", got["limit"])
	}
}

func TestNormalizeReadArgsRequiresPath(t *testing.T) {
	_, err := normalizeToolArguments("read", map[string]any{"offset": 0})
	if err == nil {
		t.Fatalf("expected read path validation error")
	}
	if err.Error() != "validation_failed: read.path is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeFindArgsRequiresQuery(t *testing.T) {
	_, err := normalizeToolArguments("find", map[string]any{"path": "."})
	if err == nil {
		t.Fatalf("expected find query validation error")
	}
	if err.Error() != "validation_failed: find.query is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeGrepArgsAcceptsAliases(t *testing.T) {
	got, err := normalizeToolArguments("grep", map[string]any{
		"query":      "TODO",
		"directory":  ".",
		"maxResults": "3",
		"ignoreCase": "true",
	})
	if err != nil {
		t.Fatalf("normalize grep args failed: %v", err)
	}
	if p, _ := got["pattern"].(string); p != "TODO" {
		t.Fatalf("unexpected pattern: %#v", got["pattern"])
	}
	if path, _ := got["path"].(string); path != "." {
		t.Fatalf("unexpected path: %#v", got["path"])
	}
	if lim, _ := got["limit"].(int); lim != 3 {
		t.Fatalf("unexpected limit: %#v", got["limit"])
	}
	if ic, _ := got["ignore_case"].(bool); !ic {
		t.Fatalf("unexpected ignore_case: %#v", got["ignore_case"])
	}
}

func TestNormalizeGrepArgsRequiresPattern(t *testing.T) {
	_, err := normalizeToolArguments("grep", map[string]any{"path": "."})
	if err == nil {
		t.Fatalf("expected grep pattern validation error")
	}
	if err.Error() != "validation_failed: grep.pattern is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeWriteArgsAcceptsAliases(t *testing.T) {
	got, err := normalizeToolArguments("write", map[string]any{
		"filePath": "a.txt",
		"text":     "hello",
	})
	if err != nil {
		t.Fatalf("normalize write args failed: %v", err)
	}
	if p, _ := got["path"].(string); p != "a.txt" {
		t.Fatalf("unexpected path: %#v", got["path"])
	}
	if c, _ := got["content"].(string); c != "hello" {
		t.Fatalf("unexpected content: %#v", got["content"])
	}
}

func TestNormalizeWriteArgsRequiresPathAndContent(t *testing.T) {
	if _, err := normalizeToolArguments("write", map[string]any{"content": "x"}); err == nil || err.Error() != "validation_failed: write.path is required" {
		t.Fatalf("unexpected error for missing path: %v", err)
	}
	if _, err := normalizeToolArguments("write", map[string]any{"path": "a.txt"}); err == nil || err.Error() != "validation_failed: write.content is required" {
		t.Fatalf("unexpected error for missing content: %v", err)
	}
}

func TestNormalizeWriteArgsAllowsEmptyContent(t *testing.T) {
	got, err := normalizeToolArguments("write", map[string]any{"path": "a.txt", "content": ""})
	if err != nil {
		t.Fatalf("normalize write with empty content failed: %v", err)
	}
	if c, _ := got["content"].(string); c != "" {
		t.Fatalf("expected empty content, got: %#v", got["content"])
	}
}

func TestNormalizeEditArgsAcceptsAliases(t *testing.T) {
	got, err := normalizeToolArguments("edit", map[string]any{
		"filePath": "a.txt",
		"old_text": "before",
		"new_text": "after",
	})
	if err != nil {
		t.Fatalf("normalize edit args failed: %v", err)
	}
	if p, _ := got["path"].(string); p != "a.txt" {
		t.Fatalf("unexpected path: %#v", got["path"])
	}
	if oldText, _ := got["oldText"].(string); oldText != "before" {
		t.Fatalf("unexpected oldText: %#v", got["oldText"])
	}
	if newText, _ := got["newText"].(string); newText != "after" {
		t.Fatalf("unexpected newText: %#v", got["newText"])
	}
}

func TestNormalizeEditArgsRequiresFields(t *testing.T) {
	if _, err := normalizeToolArguments("edit", map[string]any{"oldText": "x", "newText": "y"}); err == nil || err.Error() != "validation_failed: edit.path is required" {
		t.Fatalf("unexpected error for missing path: %v", err)
	}
	if _, err := normalizeToolArguments("edit", map[string]any{"path": "a.txt", "newText": "y"}); err == nil || err.Error() != "validation_failed: edit.oldText is required" {
		t.Fatalf("unexpected error for missing oldText: %v", err)
	}
	if _, err := normalizeToolArguments("edit", map[string]any{"path": "a.txt", "oldText": "x"}); err == nil || err.Error() != "validation_failed: edit.newText is required" {
		t.Fatalf("unexpected error for missing newText: %v", err)
	}
}
