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
	Type     string `json:"type"`
	ID       string `json:"id"`
	ParentID string `json:"parent_id,omitempty"`
}

type Manager struct {
	baseDir string

	mu       sync.Mutex
	activeID string
}

func NewManager(baseDir string) (*Manager, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("empty_base_dir")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	return &Manager{baseDir: baseDir}, nil
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
		Type:     metaType,
		ID:       id,
		ParentID: parentID,
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
			return SessionMeta{ID: sessionID, Type: metaType}, nil
		}
		if meta.ID == "" {
			meta.ID = sessionID
		}
		return meta, nil
	}
	if err := scanner.Err(); err != nil {
		return SessionMeta{}, err
	}
	return SessionMeta{ID: sessionID, Type: metaType}, nil
}

func (m *Manager) sessionPath(id string) string {
	return filepath.Join(m.baseDir, id+".jsonl")
}
