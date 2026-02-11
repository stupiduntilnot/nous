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

	id := fmt.Sprintf("sess-%d", time.Now().UTC().UnixNano())
	if err := os.WriteFile(m.sessionPath(id), nil, 0o644); err != nil {
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
	defer m.mu.Unlock()

	if m.activeID == "" {
		return fmt.Errorf("no_active_session")
	}

	f, err := os.OpenFile(m.sessionPath(m.activeID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
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
		records = append(records, raw)
	}
	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}
	return records, skipped, nil
}

func (m *Manager) sessionPath(id string) string {
	return filepath.Join(m.baseDir, id+".jsonl")
}
