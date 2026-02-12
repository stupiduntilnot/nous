package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const CurrentSchemaVersion = 3

const EntryTypeMessage = "message"

type MessageEntry struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
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

func NormalizeMessageChain(entries []MessageEntry) []MessageEntry {
	out := make([]MessageEntry, len(entries))
	copy(out, entries)

	parentID := ""
	autoID := 0
	for i := range out {
		if out[i].ID == "" {
			autoID++
			out[i].ID = fmt.Sprintf("legacy-msg-%d", autoID)
		}
		if out[i].ParentID == "" && parentID != "" {
			out[i].ParentID = parentID
		}
		if out[i].CreatedAt == "" {
			out[i].CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		parentID = out[i].ID
	}
	return out
}

func BuildMessagePath(entries []MessageEntry, leafID string) []MessageEntry {
	if len(entries) == 0 {
		return nil
	}
	normalized := NormalizeMessageChain(entries)
	byID := make(map[string]MessageEntry, len(normalized))
	for _, rec := range normalized {
		byID[rec.ID] = rec
	}
	if leafID == "" {
		leafID = normalized[len(normalized)-1].ID
	}
	if _, ok := byID[leafID]; !ok {
		return normalized
	}
	reversed := make([]MessageEntry, 0, len(normalized))
	currentID := leafID
	seen := map[string]struct{}{}
	for currentID != "" {
		if _, ok := seen[currentID]; ok {
			return normalized
		}
		seen[currentID] = struct{}{}
		rec, ok := byID[currentID]
		if !ok {
			return normalized
		}
		reversed = append(reversed, rec)
		currentID = rec.ParentID
	}
	out := make([]MessageEntry, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		out = append(out, reversed[i])
	}
	return out
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
