package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestManagerNewSessionAppendRecover(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	id, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	if id == "" {
		t.Fatalf("expected non-empty session id")
	}
	if m.ActiveSession() != id {
		t.Fatalf("expected active session %q, got %q", id, m.ActiveSession())
	}

	if err := m.Append(map[string]any{"id": "m1", "text": "hello"}); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := m.Append(map[string]any{"id": "m2", "text": "world"}); err != nil {
		t.Fatalf("append failed: %v", err)
	}

	recs, skipped, err := m.Recover(id)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", skipped)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}

	var first map[string]any
	if err := json.Unmarshal(recs[0], &first); err != nil {
		t.Fatalf("unmarshal first failed: %v", err)
	}
	if first["id"] != "m1" {
		t.Fatalf("unexpected first record: %v", first)
	}
}

func TestManagerRecoverSkipsCorruptedLines(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	id, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	path := filepath.Join(dir, id+".jsonl")
	content := []byte("{\"id\":\"ok-1\"}\nnot-json\n{\"id\":\"ok-2\"}\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write test file failed: %v", err)
	}

	recs, skipped, err := m.Recover(id)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if skipped != 1 {
		t.Fatalf("expected skipped=1, got %d", skipped)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(recs))
	}
}

func TestManagerSwitchSession(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	id1, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	id2, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	if err := m.SwitchSession(id1); err != nil {
		t.Fatalf("switch to id1 failed: %v", err)
	}
	if m.ActiveSession() != id1 {
		t.Fatalf("expected active %q got %q", id1, m.ActiveSession())
	}
	if err := m.SwitchSession(id2); err != nil {
		t.Fatalf("switch to id2 failed: %v", err)
	}
	if m.ActiveSession() != id2 {
		t.Fatalf("expected active %q got %q", id2, m.ActiveSession())
	}
	if err := m.SwitchSession("missing"); err == nil {
		t.Fatalf("expected missing session switch to fail")
	}
}

func TestManagerAppendRequiresActiveSession(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	if err := m.Append(map[string]any{"id": "x"}); err == nil {
		t.Fatalf("expected append without active session to fail")
	}
}

func TestManagerBranchAndBuildContext(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	parentID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new parent session failed: %v", err)
	}
	if err := m.Append(map[string]any{"id": "p1"}); err != nil {
		t.Fatalf("append parent record failed: %v", err)
	}

	childID, err := m.BranchFrom(parentID)
	if err != nil {
		t.Fatalf("branch from parent failed: %v", err)
	}
	if err := m.Append(map[string]any{"id": "c1"}); err != nil {
		t.Fatalf("append child record failed: %v", err)
	}

	ctxRecords, err := m.BuildContext(childID)
	if err != nil {
		t.Fatalf("build context failed: %v", err)
	}
	if len(ctxRecords) != 2 {
		t.Fatalf("expected 2 context records, got %d", len(ctxRecords))
	}

	var first map[string]any
	if err := json.Unmarshal(ctxRecords[0], &first); err != nil {
		t.Fatalf("unmarshal first record failed: %v", err)
	}
	var second map[string]any
	if err := json.Unmarshal(ctxRecords[1], &second); err != nil {
		t.Fatalf("unmarshal second record failed: %v", err)
	}
	if first["id"] != "p1" || second["id"] != "c1" {
		t.Fatalf("unexpected context order: first=%v second=%v", first, second)
	}
}
