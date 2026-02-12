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
