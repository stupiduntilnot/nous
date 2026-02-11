package provider

import "context"

type EventType string

const (
	EventStart    EventType = "start"
	EventTextDelta EventType = "text_delta"
	EventDone     EventType = "done"
	EventError    EventType = "error"
)

type Event struct {
	Type  EventType
	Delta string
	Err   error
}

type Adapter interface {
	Stream(ctx context.Context, prompt string) <-chan Event
}
