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
	listeners  []EventListener
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

func (r *Runtime) AbortRun() error {
	if r.state != StateRunning {
		return fmt.Errorf("invalid_transition: %s -> %s", r.state, StateAborting)
	}
	r.state = StateAborting
	return nil
}
