package core

import "testing"

func TestMessageEventOrdering(t *testing.T) {
	r := NewRuntime()
	seen := make([]EventType, 0, 8)
	r.Subscribe(func(ev Event) {
		seen = append(seen, ev.Type)
	})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	must(r.StartRun("run-1"))
	must(r.StartTurn())
	must(r.MessageStart("m-1", "assistant"))
	must(r.MessageUpdate("m-1", "hello"))
	must(r.MessageEnd("m-1"))
	must(r.EndTurn())
	must(r.EndRun())

	want := []EventType{
		EventAgentStart,
		EventTurnStart,
		EventMessageStart,
		EventMessageUpdate,
		EventMessageEnd,
		EventTurnEnd,
		EventAgentEnd,
	}

	if len(seen) != len(want) {
		t.Fatalf("unexpected event count: got %d want %d (%v)", len(seen), len(want), seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("unexpected event at %d: got %s want %s", i, seen[i], want[i])
		}
	}
}

func TestMessageEventOrderingInvalid(t *testing.T) {
	r := NewRuntime()
	if err := r.MessageStart("m-1", "assistant"); err == nil {
		t.Fatalf("expected message_start to fail while idle")
	}
	if err := r.MessageUpdate("m-1", "x"); err == nil {
		t.Fatalf("expected message_update to fail while idle")
	}
	if err := r.MessageEnd("m-1"); err == nil {
		t.Fatalf("expected message_end to fail while idle")
	}
}

func TestMessageEventOrderingAllowsMultipleUpdates(t *testing.T) {
	r := NewRuntime()
	updates := 0
	r.Subscribe(func(ev Event) {
		if ev.Type == EventMessageUpdate {
			updates++
		}
	})

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	must(r.StartRun("run-1"))
	must(r.StartTurn())
	must(r.MessageStart("m-1", "assistant"))
	must(r.MessageUpdate("m-1", "a"))
	must(r.MessageUpdate("m-1", "b"))
	must(r.MessageUpdate("m-1", "c"))
	must(r.MessageEnd("m-1"))
	must(r.EndTurn())
	must(r.EndRun())

	if updates != 3 {
		t.Fatalf("expected 3 message_update events, got %d", updates)
	}
}
