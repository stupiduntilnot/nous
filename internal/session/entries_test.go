package session

import (
	"encoding/json"
	"testing"
)

func TestDecodeMessageEntrySupportsLegacyShape(t *testing.T) {
	raw := json.RawMessage(`{"role":"user","text":"hello"}`)
	rec, ok := DecodeMessageEntry(raw)
	if !ok {
		t.Fatalf("expected legacy message line to decode")
	}
	if rec.Type != EntryTypeMessage || rec.Role != "user" || rec.Text != "hello" {
		t.Fatalf("unexpected decoded entry: %+v", rec)
	}
}

func TestBuildPromptContext(t *testing.T) {
	got := BuildPromptContext([]MessageEntry{
		{Type: EntryTypeMessage, Role: "user", Text: "first"},
		{Type: EntryTypeMessage, Role: "assistant", Text: "second"},
	}, "next", 20)
	want := "Conversation so far:\nuser: first\nassistant: second\nuser: next"
	if got != want {
		t.Fatalf("unexpected prompt context\nwant=%q\ngot=%q", want, got)
	}
}

func TestNormalizeMessageChainBackfillsLegacyIDsAndParents(t *testing.T) {
	entries := NormalizeMessageChain([]MessageEntry{
		{Type: EntryTypeMessage, Role: "user", Text: "one"},
		{Type: EntryTypeMessage, Role: "assistant", Text: "two"},
	})
	if len(entries) != 2 {
		t.Fatalf("unexpected entries len: %d", len(entries))
	}
	if entries[0].ID == "" || entries[1].ID == "" {
		t.Fatalf("expected normalized ids, got: %+v", entries)
	}
	if entries[0].ParentID != "" {
		t.Fatalf("expected root parent id to be empty, got: %q", entries[0].ParentID)
	}
	if entries[1].ParentID != entries[0].ID {
		t.Fatalf("expected second parent id to point to first, got: %+v", entries)
	}
}

func TestBuildMessagePathByLeaf(t *testing.T) {
	entries := []MessageEntry{
		{Type: EntryTypeMessage, ID: "a", Role: "user", Text: "root"},
		{Type: EntryTypeMessage, ID: "b", ParentID: "a", Role: "assistant", Text: "left"},
		{Type: EntryTypeMessage, ID: "c", ParentID: "a", Role: "assistant", Text: "right"},
	}
	path := BuildMessagePath(entries, "c")
	if len(path) != 2 {
		t.Fatalf("expected two entries in branch path, got %d (%+v)", len(path), path)
	}
	if path[0].ID != "a" || path[1].ID != "c" {
		t.Fatalf("unexpected branch path: %+v", path)
	}
}
