package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"nous/internal/protocol"
)

func CaptureRunTrace(socketPath, runID string, timeout time.Duration) ([]protocol.Envelope, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("empty_socket_path")
	}
	if runID == "" {
		return nil, fmt.Errorf("empty_run_id")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("invalid_timeout")
	}

	conn, err := net.DialTimeout("unix", socketPath+".events", timeout)
	if err != nil {
		return nil, fmt.Errorf("dial event socket: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	events := make([]protocol.Envelope, 0, 64)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return nil, fmt.Errorf("read event: %w", err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		if evRunID, _ := env.Payload["run_id"].(string); evRunID != runID {
			continue
		}
		events = append(events, env)
		// A subscriber may attach after agent_start but before agent_end.
		// For trace capture, agent_end is the only terminal signal required.
		if env.Type == string(protocol.EvAgentEnd) {
			return events, nil
		}
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("trace_timeout_no_events: run=%s", runID)
	}
	return nil, fmt.Errorf("trace_timeout_incomplete: run=%s events=%d", runID, len(events))
}

func WriteTraceNDJSON(w io.Writer, events []protocol.Envelope) error {
	for _, env := range events {
		b, err := json.Marshal(env)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func ReadTraceNDJSON(r io.Reader) ([]protocol.Envelope, error) {
	scanner := bufio.NewScanner(r)
	out := make([]protocol.Envelope, 0, 64)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env protocol.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func ValidateRunTrace(events []protocol.Envelope, runID string) error {
	if runID == "" {
		return fmt.Errorf("empty_run_id")
	}
	if len(events) == 0 {
		return fmt.Errorf("empty_trace")
	}

	agentStart := 0
	agentEnd := 0
	turnDepth := 0
	messageDepth := 0
	toolDepth := 0
	ended := false

	for i, env := range events {
		evRunID, _ := env.Payload["run_id"].(string)
		if evRunID != runID {
			return fmt.Errorf("trace_event_run_mismatch at %d: got=%q want=%q", i, evRunID, runID)
		}
		if ended {
			return fmt.Errorf("trace_has_events_after_agent_end at %d", i)
		}
		switch env.Type {
		case string(protocol.EvAgentStart):
			agentStart++
		case string(protocol.EvAgentEnd):
			agentEnd++
			if turnDepth != 0 || messageDepth != 0 || toolDepth != 0 {
				return fmt.Errorf("trace_unbalanced_before_agent_end: turn=%d message=%d tool=%d", turnDepth, messageDepth, toolDepth)
			}
			ended = true
		case string(protocol.EvTurnStart):
			turnDepth++
		case string(protocol.EvTurnEnd):
			turnDepth--
			if turnDepth < 0 {
				return fmt.Errorf("trace_turn_underflow at %d", i)
			}
		case string(protocol.EvMessageStart):
			messageDepth++
		case string(protocol.EvMessageUpdate):
			if messageDepth <= 0 {
				return fmt.Errorf("trace_message_update_outside_message at %d", i)
			}
		case string(protocol.EvMessageEnd):
			messageDepth--
			if messageDepth < 0 {
				return fmt.Errorf("trace_message_underflow at %d", i)
			}
		case string(protocol.EvToolExecutionStart):
			toolDepth++
		case string(protocol.EvToolExecutionUpdate):
			if toolDepth <= 0 {
				return fmt.Errorf("trace_tool_update_outside_tool at %d", i)
			}
		case string(protocol.EvToolExecutionEnd):
			toolDepth--
			if toolDepth < 0 {
				return fmt.Errorf("trace_tool_underflow at %d", i)
			}
		}
	}
	if agentStart != 1 || agentEnd != 1 {
		return fmt.Errorf("trace_agent_lifecycle_invalid: start=%d end=%d", agentStart, agentEnd)
	}
	if turnDepth != 0 || messageDepth != 0 || toolDepth != 0 {
		return fmt.Errorf("trace_unbalanced_end_state: turn=%d message=%d tool=%d", turnDepth, messageDepth, toolDepth)
	}
	return nil
}
