package session

import (
	"encoding/json"
	"strings"
	"time"
)

const CurrentSchemaVersion = 2

const EntryTypeMessage = "message"

type MessageEntry struct {
	Type      string `json:"type"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	RunID     string `json:"run_id,omitempty"`
	TurnKind  string `json:"turn_kind,omitempty"`
	CreatedAt string `json:"created_at"`
}

func NewMessageEntry(role, text, runID, turnKind string) MessageEntry {
	return MessageEntry{
		Type:      EntryTypeMessage,
		Role:      role,
		Text:      text,
		RunID:     runID,
		TurnKind:  turnKind,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func DecodeMessageEntry(raw json.RawMessage) (MessageEntry, bool) {
	var rec MessageEntry
	if err := json.Unmarshal(raw, &rec); err != nil {
		return MessageEntry{}, false
	}
	if rec.Type == "" {
		// Legacy compatibility: treat legacy role/text lines as message entry.
		rec.Type = EntryTypeMessage
	}
	if rec.Type != EntryTypeMessage || rec.Role == "" || strings.TrimSpace(rec.Text) == "" {
		return MessageEntry{}, false
	}
	return rec, true
}

func BuildPromptContext(entries []MessageEntry, prompt string, maxLines int) string {
	if len(entries) == 0 {
		return prompt
	}
	lines := make([]string, 0, len(entries))
	for _, rec := range entries {
		if rec.Role == "" || strings.TrimSpace(rec.Text) == "" {
			continue
		}
		lines = append(lines, rec.Role+": "+rec.Text)
	}
	if len(lines) == 0 {
		return prompt
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	var b strings.Builder
	b.WriteString("Conversation so far:\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("user: ")
	b.WriteString(prompt)
	return b.String()
}
