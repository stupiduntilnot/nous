package provider

import "context"

type EventType string

const (
	EventStart     EventType = "start"
	EventTextDelta EventType = "text_delta"
	EventToolCall  EventType = "tool_call"
	EventAwaitNext EventType = "await_next_turn"
	EventStatus    EventType = "status"
	EventWarning   EventType = "warning"
	EventDone      EventType = "done"
	EventError     EventType = "error"
)

type StopReason string

const (
	StopReasonUnknown StopReason = "unknown"
	StopReasonStop    StopReason = "stop"
	StopReasonLength  StopReason = "length"
	StopReasonToolUse StopReason = "tool_use"
	StopReasonError   StopReason = "error"
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type ContentBlock struct {
	Type       string         `json:"type"`
	Text       string         `json:"text,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolName   string         `json:"tool_name,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty"`
}

type Message struct {
	Role       string
	Content    string
	Blocks     []ContentBlock
	ToolCallID string
	ToolCalls  []ToolCall
}

type Request struct {
	Messages    []Message
	ActiveTools []string
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type Event struct {
	Type       EventType
	Delta      string
	ToolCall   ToolCall
	StopReason StopReason
	Usage      *Usage
	Code       string
	Message    string
	Err        error
}

type Adapter interface {
	Stream(ctx context.Context, req Request) <-chan Event
}
