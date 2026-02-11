package provider

import (
	"context"
	"fmt"
)

type MockAdapter struct{}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{}
}

func (m *MockAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			out <- Event{Type: EventError, Err: ctx.Err()}
			return
		default:
		}

		out <- Event{Type: EventStart}
		text := fmt.Sprintf("mock response: %s", req.Prompt)
		out <- Event{Type: EventTextDelta, Delta: text}
		out <- Event{Type: EventDone}
	}()
	return out
}
