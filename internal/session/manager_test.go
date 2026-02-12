package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestManagerAppendToTargetsSpecificSession(t *testing.T) {
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
	if err := m.SwitchSession(id2); err != nil {
		t.Fatalf("switch failed: %v", err)
	}

	if err := m.AppendTo(id1, map[string]any{"id": "targeted"}); err != nil {
		t.Fatalf("append to specific session failed: %v", err)
	}

	recs1, skipped1, err := m.Recover(id1)
	if err != nil {
		t.Fatalf("recover id1 failed: %v", err)
	}
	if skipped1 != 0 || len(recs1) != 1 {
		t.Fatalf("unexpected id1 recover result: recs=%d skipped=%d", len(recs1), skipped1)
	}
	recs2, skipped2, err := m.Recover(id2)
	if err != nil {
		t.Fatalf("recover id2 failed: %v", err)
	}
	if skipped2 != 0 || len(recs2) != 0 {
		t.Fatalf("unexpected id2 recover result: recs=%d skipped=%d", len(recs2), skipped2)
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

func TestManagerNewSessionWritesSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	id, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	f, err := os.Open(filepath.Join(dir, id+".jsonl"))
	if err != nil {
		t.Fatalf("open session failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatalf("session file should contain metadata line")
	}
	var meta SessionMeta
	if err := json.Unmarshal(scanner.Bytes(), &meta); err != nil {
		t.Fatalf("decode metadata failed: %v", err)
	}
	if meta.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("unexpected schema version: got=%d want=%d", meta.SchemaVersion, CurrentSchemaVersion)
	}
}

func TestBuildMessageContextSupportsLegacySessionLines(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	sessionID := "sess-legacy"
	path := filepath.Join(dir, sessionID+".jsonl")
	lines := []string{
		`{"type":"session_meta","id":"sess-legacy"}`,
		`{"type":"message","role":"user","text":"hello","created_at":"2026-01-01T00:00:00Z"}`,
		`{"role":"assistant","text":"world","created_at":"2026-01-01T00:00:01Z"}`,
		`{"type":"other","role":"user","text":"skip me"}`,
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"+lines[3]+"\n"), 0o644); err != nil {
		t.Fatalf("write legacy session failed: %v", err)
	}

	msgs, err := m.BuildMessageContext(sessionID)
	if err != nil {
		t.Fatalf("build message context failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 legacy-compatible message entries, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[1].Role != "assistant" {
		t.Fatalf("unexpected decoded message roles: %+v", msgs)
	}
	if msgs[0].ID == "" || msgs[1].ID == "" {
		t.Fatalf("expected legacy lines to be normalized with ids: %+v", msgs)
	}
	if msgs[1].ParentID != msgs[0].ID {
		t.Fatalf("expected parent linkage after normalization: %+v", msgs)
	}
}

func TestAppendMessageToAssignsIDAndParent(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	if err := m.AppendMessageTo(sessionID, NewMessageEntry("user", "hello", "run-1", "prompt")); err != nil {
		t.Fatalf("append first message failed: %v", err)
	}
	if err := m.AppendMessageTo(sessionID, NewMessageEntry("assistant", "world", "run-1", "prompt")); err != nil {
		t.Fatalf("append second message failed: %v", err)
	}

	msgs, err := m.BuildMessageContext(sessionID)
	if err != nil {
		t.Fatalf("build message context failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected two messages, got %d", len(msgs))
	}
	if msgs[0].ID == "" || msgs[1].ID == "" {
		t.Fatalf("expected ids to be assigned: %+v", msgs)
	}
	if msgs[1].ParentID != msgs[0].ID {
		t.Fatalf("expected second message parent_id to point to first: %+v", msgs)
	}
}

func TestAppendMessageToResolvedHonorsExplicitParent(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	root, err := m.AppendMessageToResolved(sessionID, MessageEntry{
		Type:      EntryTypeMessage,
		ID:        "root",
		Role:      "user",
		Text:      "root",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("append root failed: %v", err)
	}
	if root.ID != "root" {
		t.Fatalf("expected explicit id to be preserved: %+v", root)
	}

	child, err := m.AppendMessageToResolved(sessionID, MessageEntry{
		Type:      EntryTypeMessage,
		Role:      "assistant",
		Text:      "branch",
		ParentID:  "root",
		CreatedAt: "2026-01-01T00:00:01Z",
	})
	if err != nil {
		t.Fatalf("append child failed: %v", err)
	}
	if child.ParentID != "root" {
		t.Fatalf("expected explicit parent id to be preserved: %+v", child)
	}
}

func TestBuildMessageContextFromLeaf(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID := "sess-tree"
	path := filepath.Join(dir, sessionID+".jsonl")
	lines := []string{
		`{"type":"session_meta","id":"sess-tree","schema_version":3}`,
		`{"type":"message","id":"a","role":"user","text":"root","created_at":"2026-01-01T00:00:00Z"}`,
		`{"type":"message","id":"b","parent_id":"a","role":"assistant","text":"left","created_at":"2026-01-01T00:00:01Z"}`,
		`{"type":"message","id":"c","parent_id":"a","role":"assistant","text":"right","created_at":"2026-01-01T00:00:02Z"}`,
	}
	if err := os.WriteFile(path, []byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"+lines[3]+"\n"), 0o644); err != nil {
		t.Fatalf("write session failed: %v", err)
	}

	msgs, err := m.BuildMessageContextFromLeaf(sessionID, "c")
	if err != nil {
		t.Fatalf("build message context from leaf failed: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != "a" || msgs[1].ID != "c" {
		t.Fatalf("unexpected leaf path context: %+v", msgs)
	}
}

func TestSetActiveLeafAndBuildMessageContextFromActiveLeaf(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID := "sess-active-leaf"
	path := filepath.Join(dir, sessionID+".jsonl")
	lines := []string{
		`{"type":"session_meta","id":"sess-active-leaf","schema_version":3}`,
		`{"type":"message","id":"a","role":"user","text":"root","created_at":"2026-01-01T00:00:00Z"}`,
		`{"type":"message","id":"b","parent_id":"a","role":"assistant","text":"left","created_at":"2026-01-01T00:00:01Z"}`,
		`{"type":"message","id":"c","parent_id":"a","role":"assistant","text":"right","created_at":"2026-01-01T00:00:02Z"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session failed: %v", err)
	}

	if err := m.SetActiveLeaf(sessionID, "c"); err != nil {
		t.Fatalf("set active leaf failed: %v", err)
	}
	leafID, err := m.ActiveLeaf(sessionID)
	if err != nil {
		t.Fatalf("active leaf failed: %v", err)
	}
	if leafID != "c" {
		t.Fatalf("unexpected active leaf: got=%q want=%q", leafID, "c")
	}

	msgs, err := m.BuildMessageContextFromActiveLeaf(sessionID)
	if err != nil {
		t.Fatalf("build message context from active leaf failed: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != "a" || msgs[1].ID != "c" {
		t.Fatalf("unexpected active leaf path context: %+v", msgs)
	}
}

func TestSetActiveLeafRejectsUnknownLeaf(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	if err := m.AppendMessageTo(sessionID, NewMessageEntry("user", "hello", "run-1", "prompt")); err != nil {
		t.Fatalf("append message failed: %v", err)
	}
	if err := m.SetActiveLeaf(sessionID, "missing"); err == nil {
		t.Fatalf("expected set active leaf with unknown id to fail")
	}
}

func TestAppendMessageToResolvedTracksLeafTip(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	first, err := m.AppendMessageToResolved(sessionID, NewMessageEntry("user", "one", "run-1", "prompt"))
	if err != nil {
		t.Fatalf("append first failed: %v", err)
	}
	second, err := m.AppendMessageToResolved(sessionID, NewMessageEntry("assistant", "two", "run-1", "prompt"))
	if err != nil {
		t.Fatalf("append second failed: %v", err)
	}
	if second.ParentID != first.ID {
		t.Fatalf("unexpected parent linkage: first=%+v second=%+v", first, second)
	}
	leafID, err := m.ActiveLeaf(sessionID)
	if err != nil {
		t.Fatalf("active leaf failed: %v", err)
	}
	if leafID != second.ID {
		t.Fatalf("unexpected tracked active leaf: got=%q want=%q", leafID, second.ID)
	}
}

func TestBuildMessageContextAppliesCompactionMarker(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID := "sess-compact"
	path := filepath.Join(dir, sessionID+".jsonl")
	lines := []string{
		`{"type":"session_meta","id":"sess-compact","schema_version":3}`,
		`{"type":"message","id":"m1","role":"user","text":"old-user","created_at":"2026-01-01T00:00:00Z"}`,
		`{"type":"message","id":"m2","parent_id":"m1","role":"assistant","text":"old-assistant","created_at":"2026-01-01T00:00:01Z"}`,
		`{"type":"message","id":"m3","parent_id":"m2","role":"user","text":"recent-user","created_at":"2026-01-01T00:00:02Z"}`,
		`{"type":"compaction","id":"cmp-1","first_kept_entry_id":"m3","summary":"Compaction summary:\n- user: old-user","created_at":"2026-01-01T00:00:03Z"}`,
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write session failed: %v", err)
	}

	msgs, err := m.BuildMessageContext(sessionID)
	if err != nil {
		t.Fatalf("build message context failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected summary + kept message, got %d entries: %+v", len(msgs), msgs)
	}
	if msgs[0].ID != "cmp-1" || msgs[0].Role != "assistant" {
		t.Fatalf("unexpected synthetic summary message: %+v", msgs[0])
	}
	if msgs[1].ID != "m3" || msgs[1].ParentID != "cmp-1" {
		t.Fatalf("unexpected kept message linkage after compaction: %+v", msgs[1])
	}
}

func TestAppendCompactionEntryPersistsAndRecovers(t *testing.T) {
	m, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	sessionID, err := m.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	first, err := m.AppendMessageToResolved(sessionID, MessageEntry{
		Type:      EntryTypeMessage,
		ID:        "keep-1",
		Role:      "user",
		Text:      "hello",
		CreatedAt: "2026-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("append message failed: %v", err)
	}
	if first.ID != "keep-1" {
		t.Fatalf("unexpected first message id: %+v", first)
	}

	entry, err := m.AppendCompactionToResolved(sessionID, NewCompactionEntry("Compaction summary:\n- user: hello", "keep-1", "", 120, "manual"))
	if err != nil {
		t.Fatalf("append compaction failed: %v", err)
	}
	if entry.ID == "" {
		t.Fatalf("expected persisted compaction entry id")
	}

	raw, skipped, err := m.Recover(sessionID)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("expected skipped=0, got %d", skipped)
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 recovered records, got %d", len(raw))
	}
	if _, ok := DecodeCompactionEntry(raw[1]); !ok {
		t.Fatalf("expected second record to decode as compaction entry: %s", string(raw[1]))
	}
}
