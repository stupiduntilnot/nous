package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const metaType = "session_meta"

type SessionMeta struct {
	Type          string `json:"type"`
	ID            string `json:"id"`
	ParentID      string `json:"parent_id,omitempty"`
	SchemaVersion int    `json:"schema_version,omitempty"`
}

type Manager struct {
	baseDir string

	mu           sync.Mutex
	activeID     string
	activeLeaves map[string]string
}

func NewManager(baseDir string) (*Manager, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("empty_base_dir")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &Manager{
		baseDir:      baseDir,
		activeLeaves: make(map[string]string),
	}, nil
}

func (m *Manager) ActiveSession() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

func (m *Manager) NewSession() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.createSessionLocked("")
}

func (m *Manager) BranchFrom(parentID string) (string, error) {
	if parentID == "" {
		return "", fmt.Errorf("empty_parent_session_id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.sessionPath(parentID)); err != nil {
		return "", err
	}
	return m.createSessionLocked(parentID)
}

func (m *Manager) createSessionLocked(parentID string) (string, error) {
	id := fmt.Sprintf("sess-%d", time.Now().UTC().UnixNano())
	meta, err := json.Marshal(SessionMeta{
		Type:          metaType,
		ID:            id,
		ParentID:      parentID,
		SchemaVersion: CurrentSchemaVersion,
	})
	if err != nil {
		return "", err
	}
	content := append(meta, '\n')
	if err := os.WriteFile(m.sessionPath(id), content, 0o644); err != nil {
		return "", err
	}
	m.activeID = id
	return id, nil
}

func (m *Manager) SwitchSession(id string) error {
	if id == "" {
		return fmt.Errorf("empty_session_id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, err := os.Stat(m.sessionPath(id)); err != nil {
		return err
	}
	m.activeID = id
	return nil
}

func (m *Manager) Append(record any) error {
	m.mu.Lock()
	activeID := m.activeID
	m.mu.Unlock()

	if activeID == "" {
		return fmt.Errorf("no_active_session")
	}
	return m.AppendTo(activeID, record)
}

func (m *Manager) AppendMessage(entry MessageEntry) error {
	_, err := m.AppendMessageResolved(entry)
	return err
}

func (m *Manager) AppendMessageResolved(entry MessageEntry) (MessageEntry, error) {
	m.mu.Lock()
	activeID := m.activeID
	m.mu.Unlock()
	if activeID == "" {
		return MessageEntry{}, fmt.Errorf("no_active_session")
	}
	return m.AppendMessageToResolved(activeID, entry)
}

func (m *Manager) AppendMessageTo(sessionID string, entry MessageEntry) error {
	_, err := m.AppendMessageToResolved(sessionID, entry)
	return err
}

func (m *Manager) AppendCompaction(entry CompactionEntry) error {
	_, err := m.AppendCompactionResolved(entry)
	return err
}

func (m *Manager) AppendCompactionResolved(entry CompactionEntry) (CompactionEntry, error) {
	m.mu.Lock()
	activeID := m.activeID
	m.mu.Unlock()
	if activeID == "" {
		return CompactionEntry{}, fmt.Errorf("no_active_session")
	}
	return m.AppendCompactionToResolved(activeID, entry)
}

func (m *Manager) AppendCompactionTo(sessionID string, entry CompactionEntry) error {
	_, err := m.AppendCompactionToResolved(sessionID, entry)
	return err
}

func (m *Manager) AppendCompactionToResolved(sessionID string, entry CompactionEntry) (CompactionEntry, error) {
	if sessionID == "" {
		return CompactionEntry{}, fmt.Errorf("empty_session_id")
	}
	if entry.Type == "" {
		entry.Type = EntryTypeCompaction
	}
	if entry.Type != EntryTypeCompaction {
		return CompactionEntry{}, fmt.Errorf("invalid_compaction_entry_type")
	}
	entry.Summary = strings.TrimSpace(entry.Summary)
	entry.FirstKeptEntryID = strings.TrimSpace(entry.FirstKeptEntryID)
	entry.Instruction = strings.TrimSpace(entry.Instruction)
	entry.Trigger = strings.TrimSpace(entry.Trigger)
	if entry.Summary == "" || entry.FirstKeptEntryID == "" {
		return CompactionEntry{}, fmt.Errorf("invalid_compaction_entry")
	}
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("cmp-%d", time.Now().UTC().UnixNano())
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := m.AppendTo(sessionID, entry); err != nil {
		return CompactionEntry{}, err
	}
	return entry, nil
}

func (m *Manager) AppendMessageToResolved(sessionID string, entry MessageEntry) (MessageEntry, error) {
	if sessionID == "" {
		return MessageEntry{}, fmt.Errorf("empty_session_id")
	}
	if entry.Type == "" {
		entry.Type = EntryTypeMessage
	}
	if entry.Type != EntryTypeMessage {
		return MessageEntry{}, fmt.Errorf("invalid_message_entry_type")
	}
	if strings.TrimSpace(entry.Text) == "" || entry.Role == "" {
		return MessageEntry{}, fmt.Errorf("invalid_message_entry")
	}
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("msg-%d", time.Now().UTC().UnixNano())
	}
	if entry.ParentID == "" {
		history, err := m.BuildMessageContext(sessionID)
		if err != nil {
			return MessageEntry{}, err
		}
		if len(history) > 0 {
			entry.ParentID = history[len(history)-1].ID
		}
	}
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := m.AppendTo(sessionID, entry); err != nil {
		return MessageEntry{}, err
	}
	m.mu.Lock()
	m.activeLeaves[sessionID] = entry.ID
	m.mu.Unlock()
	return entry, nil
}

func (m *Manager) AppendTo(sessionID string, record any) error {
	if sessionID == "" {
		return fmt.Errorf("empty_session_id")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(m.sessionPath(sessionID)); err != nil {
		return err
	}

	f, err := os.OpenFile(m.sessionPath(sessionID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (m *Manager) Recover(sessionID string) (records []json.RawMessage, skipped int, err error) {
	if sessionID == "" {
		return nil, 0, fmt.Errorf("empty_session_id")
	}

	f, err := os.Open(m.sessionPath(sessionID))
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var raw json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			skipped++
			continue
		}
		var probe map[string]any
		if err := json.Unmarshal(raw, &probe); err == nil {
			if t, _ := probe["type"].(string); t == metaType {
				continue
			}
		}
		records = append(records, raw)
	}
	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}
	return records, skipped, nil
}

func (m *Manager) BuildContext(sessionID string) ([]json.RawMessage, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("empty_session_id")
	}

	chain := make([]string, 0, 8)
	seen := map[string]struct{}{}
	current := sessionID
	for current != "" {
		if _, ok := seen[current]; ok {
			return nil, fmt.Errorf("session_cycle_detected: %s", current)
		}
		seen[current] = struct{}{}
		chain = append(chain, current)

		meta, err := m.readMeta(current)
		if err != nil {
			return nil, err
		}
		current = meta.ParentID
	}

	out := make([]json.RawMessage, 0, 16)
	for i := len(chain) - 1; i >= 0; i-- {
		records, _, err := m.Recover(chain[i])
		if err != nil {
			return nil, err
		}
		out = append(out, records...)
	}
	return out, nil
}

func (m *Manager) BuildMessageContext(sessionID string) ([]MessageEntry, error) {
	raw, err := m.BuildContext(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]MessageEntry, 0, len(raw))
	for _, line := range raw {
		rec, ok := DecodeMessageEntry(line)
		if ok {
			out = append(out, rec)
			continue
		}
		compaction, ok := DecodeCompactionEntry(line)
		if !ok {
			continue
		}
		out = applyCompactionToMessages(out, compaction)
	}
	return NormalizeMessageChain(out), nil
}

func (m *Manager) BuildMessageContextFromLeaf(sessionID, leafID string) ([]MessageEntry, error) {
	entries, err := m.BuildMessageContext(sessionID)
	if err != nil {
		return nil, err
	}
	return BuildMessagePath(entries, leafID), nil
}

func (m *Manager) BuildMessageContextFromActiveLeaf(sessionID string) ([]MessageEntry, error) {
	entries, err := m.BuildMessageContext(sessionID)
	if err != nil {
		return nil, err
	}
	activeLeaf := m.activeLeafLocked(sessionID)
	if activeLeaf == "" {
		return entries, nil
	}
	return BuildMessagePath(entries, activeLeaf), nil
}

func (m *Manager) BuildMessageTree(sessionID string) ([]MessageEntry, error) {
	return m.BuildMessageContext(sessionID)
}

func (m *Manager) SetActiveLeaf(sessionID, leafID string) error {
	if sessionID == "" {
		return fmt.Errorf("empty_session_id")
	}
	if _, err := os.Stat(m.sessionPath(sessionID)); err != nil {
		return err
	}

	leafID = strings.TrimSpace(leafID)
	if leafID == "" {
		m.mu.Lock()
		delete(m.activeLeaves, sessionID)
		m.mu.Unlock()
		return nil
	}

	entries, err := m.BuildMessageContext(sessionID)
	if err != nil {
		return err
	}
	found := false
	for _, rec := range entries {
		if rec.ID == leafID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("leaf_not_found")
	}

	m.mu.Lock()
	m.activeLeaves[sessionID] = leafID
	m.mu.Unlock()
	return nil
}

func (m *Manager) ActiveLeaf(sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("empty_session_id")
	}
	if _, err := os.Stat(m.sessionPath(sessionID)); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeLeaves[sessionID], nil
}

func (m *Manager) readMeta(sessionID string) (SessionMeta, error) {
	f, err := os.Open(m.sessionPath(sessionID))
	if err != nil {
		return SessionMeta{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal([]byte(line), &meta); err != nil {
			return SessionMeta{}, err
		}
		if meta.Type != metaType {
			return SessionMeta{ID: sessionID, Type: metaType, SchemaVersion: 1}, nil
		}
		if meta.ID == "" {
			meta.ID = sessionID
		}
		if meta.SchemaVersion <= 0 {
			meta.SchemaVersion = 1
		}
		return meta, nil
	}
	if err := scanner.Err(); err != nil {
		return SessionMeta{}, err
	}
	return SessionMeta{ID: sessionID, Type: metaType, SchemaVersion: 1}, nil
}

func (m *Manager) sessionPath(id string) string {
	return filepath.Join(m.baseDir, id+".jsonl")
}

func (m *Manager) activeLeafLocked(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeLeaves[sessionID]
}

func applyCompactionToMessages(entries []MessageEntry, compaction CompactionEntry) []MessageEntry {
	if len(entries) == 0 {
		return entries
	}
	normalized := NormalizeMessageChain(entries)
	firstKeptIdx := -1
	for i := range normalized {
		if normalized[i].ID == compaction.FirstKeptEntryID {
			firstKeptIdx = i
			break
		}
	}
	if firstKeptIdx <= 0 {
		return normalized
	}
	summaryID := compaction.ID
	if summaryID == "" {
		summaryID = fmt.Sprintf("cmp-synth-%d", time.Now().UTC().UnixNano())
	}
	summaryText := strings.TrimSpace(compaction.Summary)
	if summaryText == "" {
		return normalized
	}
	summary := MessageEntry{
		Type:      EntryTypeMessage,
		ID:        summaryID,
		Role:      "assistant",
		Text:      summaryText,
		CreatedAt: compaction.CreatedAt,
	}
	if summary.CreatedAt == "" {
		summary.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}

	kept := make([]MessageEntry, len(normalized[firstKeptIdx:]))
	copy(kept, normalized[firstKeptIdx:])
	kept[0].ParentID = summary.ID
	out := make([]MessageEntry, 0, 1+len(kept))
	out = append(out, summary)
	out = append(out, kept...)
	return out
}
