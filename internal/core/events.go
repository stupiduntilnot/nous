package core

import "time"

type EventType string

const (
	EventAgentStart   EventType = "agent_start"
	EventAgentEnd     EventType = "agent_end"
	EventTurnStart    EventType = "turn_start"
	EventTurnEnd      EventType = "turn_end"
	EventMessageStart EventType = "message_start"
	EventMessageUpdate EventType = "message_update"
	EventMessageEnd   EventType = "message_end"
)

type Event struct {
	Type      EventType `json:"type"`
	RunID     string    `json:"run_id,omitempty"`
	Turn      int       `json:"turn,omitempty"`
	MessageID string    `json:"message_id,omitempty"`
	Role      string    `json:"role,omitempty"`
	Delta     string    `json:"delta,omitempty"`
	Timestamp string    `json:"ts"`
}

type EventListener func(Event)

func nowTS() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
