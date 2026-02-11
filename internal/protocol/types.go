package protocol

import (
	"encoding/json"
	"fmt"
)

const Version = "1"

type CommandType string

type EventType string

const (
	CmdPing           CommandType = "ping"
	CmdPrompt         CommandType = "prompt"
	CmdSteer          CommandType = "steer"
	CmdFollowUp       CommandType = "follow_up"
	CmdAbort          CommandType = "abort"
	CmdSetActiveTools CommandType = "set_active_tools"
	CmdNewSession     CommandType = "new_session"
	CmdSwitchSession  CommandType = "switch_session"
	CmdBranchSession  CommandType = "branch_session"
)

const (
	EvAgentStart          EventType = "agent_start"
	EvAgentEnd            EventType = "agent_end"
	EvTurnStart           EventType = "turn_start"
	EvTurnEnd             EventType = "turn_end"
	EvMessageStart        EventType = "message_start"
	EvMessageUpdate       EventType = "message_update"
	EvMessageEnd          EventType = "message_end"
	EvToolExecutionStart  EventType = "tool_execution_start"
	EvToolExecutionUpdate EventType = "tool_execution_update"
	EvToolExecutionEnd    EventType = "tool_execution_end"
	EvStatus              EventType = "status"
	EvWarning             EventType = "warning"
	EvError               EventType = "error"
)

type Envelope struct {
	V       string                 `json:"v"`
	ID      string                 `json:"id"`
	Type    string                 `json:"type"`
	Payload map[string]any         `json:"payload"`
	TS      string                 `json:"ts,omitempty"`
	Meta    map[string]interface{} `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ResponseEnvelope struct {
	Envelope
	OK    bool       `json:"ok"`
	Error *ErrorBody `json:"error,omitempty"`
}

var validCommands = map[CommandType]struct{}{
	CmdPing: {}, CmdPrompt: {}, CmdSteer: {}, CmdFollowUp: {}, CmdAbort: {}, CmdSetActiveTools: {}, CmdNewSession: {}, CmdSwitchSession: {}, CmdBranchSession: {},
}

var validEvents = map[EventType]struct{}{
	EvAgentStart: {}, EvAgentEnd: {}, EvTurnStart: {}, EvTurnEnd: {}, EvMessageStart: {}, EvMessageUpdate: {}, EvMessageEnd: {},
	EvToolExecutionStart: {}, EvToolExecutionUpdate: {}, EvToolExecutionEnd: {}, EvStatus: {}, EvWarning: {}, EvError: {},
}

func DecodeCommand(line []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("invalid_json: %w", err)
	}

	if env.V == "" {
		env.V = Version
	}
	if env.V != Version {
		return Envelope{}, fmt.Errorf("invalid_version: expected %s, got %s", Version, env.V)
	}
	if env.ID == "" {
		return Envelope{}, fmt.Errorf("missing_id")
	}
	if env.Type == "" {
		return Envelope{}, fmt.Errorf("missing_type")
	}
	if _, ok := validCommands[CommandType(env.Type)]; !ok {
		return Envelope{}, fmt.Errorf("invalid_command: %s", env.Type)
	}
	if env.Payload == nil {
		env.Payload = map[string]any{}
	}
	return env, nil
}

func ValidateEventType(t EventType) error {
	if _, ok := validEvents[t]; !ok {
		return fmt.Errorf("invalid_event: %s", t)
	}
	return nil
}
