package core

import "fmt"

type RunState string

const (
	StateIdle     RunState = "idle"
	StateRunning  RunState = "running"
	StateAborting RunState = "aborting"
)

type Runtime struct {
	state      RunState
	runID      string
	turnNumber int
}

func NewRuntime() *Runtime {
	return &Runtime{state: StateIdle}
}

func (r *Runtime) State() RunState {
	return r.state
}

func (r *Runtime) RunID() string {
	return r.runID
}

func (r *Runtime) TurnNumber() int {
	return r.turnNumber
}

func (r *Runtime) StartRun(runID string) error {
	if runID == "" {
		return fmt.Errorf("invalid_run_id")
	}
	if r.state != StateIdle {
		return fmt.Errorf("invalid_transition: %s -> %s", r.state, StateRunning)
	}
	r.state = StateRunning
	r.runID = runID
	r.turnNumber = 0
	return nil
}

func (r *Runtime) StartTurn() error {
	if r.state != StateRunning {
		return fmt.Errorf("invalid_transition: %s -> turn_start", r.state)
	}
	r.turnNumber++
	return nil
}

func (r *Runtime) AbortRun() error {
	if r.state != StateRunning {
		return fmt.Errorf("invalid_transition: %s -> %s", r.state, StateAborting)
	}
	r.state = StateAborting
	return nil
}

func (r *Runtime) EndRun() error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> %s", r.state, StateIdle)
	}
	r.state = StateIdle
	r.runID = ""
	r.turnNumber = 0
	return nil
}
