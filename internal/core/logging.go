package core

import (
	"encoding/json"
	"io"
	"time"
)

// LogEvent is the shared structured log shape used by core runtime.
type LogEvent struct {
	Timestamp  string `json:"ts"`
	Level      string `json:"level"`
	Message    string `json:"message"`
	RunID      string `json:"run_id,omitempty"`
	TurnID     string `json:"turn_id,omitempty"`
	MessageID  string `json:"message_id,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

func NewLogEvent(level, message string) LogEvent {
	return LogEvent{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level,
		Message:   message,
	}
}

func WriteLogEvent(w io.Writer, ev LogEvent) error {
	enc := json.NewEncoder(w)
	return enc.Encode(ev)
}
