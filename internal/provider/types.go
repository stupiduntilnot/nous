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
	Prompt      string
	Messages    []Message
	ActiveTools []string
	ToolResults []string
}

type Event struct {
	Type     EventType
	Delta    string
	ToolCall ToolCall
	Err      error
}

type Adapter interface {
	Stream(ctx context.Context, req Request) <-chan Event
}
