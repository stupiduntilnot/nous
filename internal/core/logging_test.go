package core

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteLogEventContainsContextFields(t *testing.T) {
	ev := NewLogEvent("info", "tool started")
	ev.RunID = "run-1"
	ev.TurnID = "turn-2"
	ev.MessageID = "msg-3"
	ev.ToolCallID = "tool-4"

	var buf bytes.Buffer
	if err := WriteLogEvent(&buf, ev); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var decoded map[string]any
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("unmarshal log failed: %v", err)
	}

	for _, key := range []string{"ts", "level", "message", "run_id", "turn_id", "message_id", "tool_call_id"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in log payload: %s", key, line)
		}
	}
}
