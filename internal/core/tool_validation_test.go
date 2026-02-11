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
