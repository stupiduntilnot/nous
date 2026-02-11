package core

import (
	"fmt"
	"slices"
	"testing"

	iproto "oh-my-agent/internal/protocol"
)

func TestCoreEventTypesParityWithProtocol(t *testing.T) {
	coreEvents := []EventType{
		EventAgentStart,
		EventAgentEnd,
		EventTurnStart,
		EventTurnEnd,
		EventMessageStart,
		EventMessageUpdate,
		EventMessageEnd,
		EventToolExecutionStart,
		EventToolExecutionUpdate,
		EventToolExecutionEnd,
		EventStatus,
		EventWarning,
		EventError,
	}

	got := make([]string, 0, len(coreEvents))
	for _, ev := range coreEvents {
		got = append(got, fmt.Sprintf("%s", ev))
	}
	slices.Sort(got)

	want := []string{
		fmt.Sprintf("%s", iproto.EvAgentStart),
		fmt.Sprintf("%s", iproto.EvAgentEnd),
		fmt.Sprintf("%s", iproto.EvTurnStart),
		fmt.Sprintf("%s", iproto.EvTurnEnd),
		fmt.Sprintf("%s", iproto.EvMessageStart),
		fmt.Sprintf("%s", iproto.EvMessageUpdate),
		fmt.Sprintf("%s", iproto.EvMessageEnd),
		fmt.Sprintf("%s", iproto.EvToolExecutionStart),
		fmt.Sprintf("%s", iproto.EvToolExecutionUpdate),
		fmt.Sprintf("%s", iproto.EvToolExecutionEnd),
		fmt.Sprintf("%s", iproto.EvStatus),
		fmt.Sprintf("%s", iproto.EvWarning),
		fmt.Sprintf("%s", iproto.EvError),
	}
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Fatalf("event enum mismatch\ncore=%v\nprotocol=%v", got, want)
	}
}
