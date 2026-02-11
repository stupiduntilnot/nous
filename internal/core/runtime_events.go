package core

import "fmt"

func (r *Runtime) Subscribe(fn EventListener) func() {
	r.listeners = append(r.listeners, fn)
	idx := len(r.listeners) - 1
	return func() {
		if idx < 0 || idx >= len(r.listeners) || r.listeners[idx] == nil {
			return
		}
		r.listeners[idx] = nil
	}
}

func (r *Runtime) emit(ev Event) {
	for _, l := range r.listeners {
		if l != nil {
			l(ev)
		}
	}
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
	r.emit(Event{Type: EventAgentStart, RunID: r.runID, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) StartTurn() error {
	if r.state != StateRunning {
		return fmt.Errorf("invalid_transition: %s -> turn_start", r.state)
	}
	r.turnNumber++
	r.emit(Event{Type: EventTurnStart, RunID: r.runID, Turn: r.turnNumber, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) EndTurn() error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> turn_end", r.state)
	}
	if r.turnNumber <= 0 {
		return fmt.Errorf("invalid_turn")
	}
	r.emit(Event{Type: EventTurnEnd, RunID: r.runID, Turn: r.turnNumber, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) MessageStart(messageID, role string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> message_start", r.state)
	}
	if messageID == "" || role == "" {
		return fmt.Errorf("invalid_message")
	}
	r.emit(Event{Type: EventMessageStart, RunID: r.runID, Turn: r.turnNumber, MessageID: messageID, Role: role, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) MessageUpdate(messageID, delta string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> message_update", r.state)
	}
	if messageID == "" {
		return fmt.Errorf("invalid_message")
	}
	r.emit(Event{Type: EventMessageUpdate, RunID: r.runID, Turn: r.turnNumber, MessageID: messageID, Delta: delta, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) MessageEnd(messageID string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> message_end", r.state)
	}
	if messageID == "" {
		return fmt.Errorf("invalid_message")
	}
	r.emit(Event{Type: EventMessageEnd, RunID: r.runID, Turn: r.turnNumber, MessageID: messageID, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) ToolExecutionStart(toolCallID, toolName string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> tool_execution_start", r.state)
	}
	if toolCallID == "" || toolName == "" {
		return fmt.Errorf("invalid_tool_call")
	}
	r.emit(Event{Type: EventToolExecutionStart, RunID: r.runID, Turn: r.turnNumber, ToolCallID: toolCallID, ToolName: toolName, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) ToolExecutionUpdate(toolCallID, toolName, delta string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> tool_execution_update", r.state)
	}
	if toolCallID == "" || toolName == "" {
		return fmt.Errorf("invalid_tool_call")
	}
	r.emit(Event{Type: EventToolExecutionUpdate, RunID: r.runID, Turn: r.turnNumber, ToolCallID: toolCallID, ToolName: toolName, Delta: delta, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) ToolExecutionEnd(toolCallID, toolName string) error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> tool_execution_end", r.state)
	}
	if toolCallID == "" || toolName == "" {
		return fmt.Errorf("invalid_tool_call")
	}
	r.emit(Event{Type: EventToolExecutionEnd, RunID: r.runID, Turn: r.turnNumber, ToolCallID: toolCallID, ToolName: toolName, Timestamp: nowTS()})
	return nil
}

func (r *Runtime) EndRun() error {
	if r.state != StateRunning && r.state != StateAborting {
		return fmt.Errorf("invalid_transition: %s -> %s", r.state, StateIdle)
	}
	r.emit(Event{Type: EventAgentEnd, RunID: r.runID, Timestamp: nowTS()})
	r.state = StateIdle
	r.runID = ""
	r.turnNumber = 0
	return nil
}
