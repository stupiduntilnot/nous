package core

import "testing"

func TestStateTransitions(t *testing.T) {
	r := NewRuntime()

	if got := r.State(); got != StateIdle {
		t.Fatalf("expected initial idle state, got %s", got)
	}

	if err := r.StartRun("run-1"); err != nil {
		t.Fatalf("start run failed: %v", err)
	}
	if r.State() != StateRunning {
		t.Fatalf("expected running state, got %s", r.State())
	}
	if r.RunID() != "run-1" {
		t.Fatalf("expected run id run-1, got %q", r.RunID())
	}

	if err := r.StartTurn(); err != nil {
		t.Fatalf("start turn failed: %v", err)
	}
	if r.TurnNumber() != 1 {
		t.Fatalf("expected turn number 1, got %d", r.TurnNumber())
	}

	if err := r.AbortRun(); err != nil {
		t.Fatalf("abort run failed: %v", err)
	}
	if r.State() != StateAborting {
		t.Fatalf("expected aborting state, got %s", r.State())
	}

	if err := r.EndRun(); err != nil {
		t.Fatalf("end run failed: %v", err)
	}
	if r.State() != StateIdle {
		t.Fatalf("expected idle state after end, got %s", r.State())
	}
	if r.RunID() != "" || r.TurnNumber() != 0 {
		t.Fatalf("expected runtime reset after end run")
	}
}

func TestStateTransitionsInvalidPaths(t *testing.T) {
	r := NewRuntime()

	if err := r.StartTurn(); err == nil {
		t.Fatalf("expected StartTurn to fail from idle")
	}
	if err := r.AbortRun(); err == nil {
		t.Fatalf("expected AbortRun to fail from idle")
	}
	if err := r.EndRun(); err == nil {
		t.Fatalf("expected EndRun to fail from idle")
	}

	if err := r.StartRun("run-1"); err != nil {
		t.Fatalf("unexpected StartRun failure: %v", err)
	}
	if err := r.StartRun("run-2"); err == nil {
		t.Fatalf("expected StartRun to fail while already running")
	}

	if err := r.EndRun(); err != nil {
		t.Fatalf("unexpected EndRun failure: %v", err)
	}
	if err := r.StartRun(""); err == nil {
		t.Fatalf("expected StartRun to reject empty run id")
	}
}
