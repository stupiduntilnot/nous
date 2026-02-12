package provider

import "context"

type EventType string

const (
	EventStart     EventType = "start"
	EventTextDelta EventType = "text_delta"
	EventToolCall  EventType = "tool_call"
	EventAwaitNext EventType = "await_next_turn"
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

type Message struct {
	Role    string
	Content string
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
	Err        error
}

type Adapter interface {
	Stream(ctx context.Context, req Request) <-chan Event
}
