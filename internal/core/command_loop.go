package core

import (
	"context"
	"fmt"
	"sync"
)

type TurnExecutor interface {
	Prompt(ctx context.Context, runID, prompt string) (string, error)
}

type RunCoordinator interface {
	BeginRun(runID string) error
	EndRun(runID string) error
}

type TurnKind string

const (
	TurnPrompt   TurnKind = "prompt"
	TurnSteer    TurnKind = "steer"
	TurnFollowUp TurnKind = "follow_up"
)

type TurnResult struct {
	RunID  string
	Kind   TurnKind
	Input  string
	Output string
	Err    error
}

type queuedTurn struct {
	kind TurnKind
	text string
}

type CommandLoop struct {
	executor TurnExecutor

	mu         sync.Mutex
	state      RunState
	runCounter int
	runID      string

	steers    []queuedTurn
	followUps []queuedTurn

	currentCancel context.CancelFunc
	onTurnEnd     func(TurnResult)
}

func NewCommandLoop(executor TurnExecutor) *CommandLoop {
	return &CommandLoop{
		executor: executor,
		state:    StateIdle,
	}
}

func (l *CommandLoop) SetOnTurnEnd(fn func(TurnResult)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onTurnEnd = fn
}

func (l *CommandLoop) State() RunState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

func (l *CommandLoop) CurrentRunID() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.runID
}

func (l *CommandLoop) Prompt(text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("empty_prompt")
	}

	l.mu.Lock()
	if l.state != StateIdle {
		l.mu.Unlock()
		return "", fmt.Errorf("run_in_progress")
	}

	l.runCounter++
	l.runID = fmt.Sprintf("run-%d", l.runCounter)
	runID := l.runID
	l.state = StateRunning
	initial := queuedTurn{kind: TurnPrompt, text: text}
	l.mu.Unlock()

	go l.process(initial)
	return runID, nil
}

func (l *CommandLoop) Steer(text string) error {
	if text == "" {
		return fmt.Errorf("empty_steer")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != StateRunning {
		return fmt.Errorf("no_active_run")
	}
	l.steers = append(l.steers, queuedTurn{kind: TurnSteer, text: text})
	return nil
}

func (l *CommandLoop) FollowUp(text string) error {
	if text == "" {
		return fmt.Errorf("empty_follow_up")
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != StateRunning {
		return fmt.Errorf("no_active_run")
	}
	l.followUps = append(l.followUps, queuedTurn{kind: TurnFollowUp, text: text})
	return nil
}

func (l *CommandLoop) Abort() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.state != StateRunning {
		return fmt.Errorf("no_active_run")
	}

	l.state = StateAborting
	l.steers = nil
	l.followUps = nil
	if l.currentCancel != nil {
		l.currentCancel()
	}
	return nil
}

func (l *CommandLoop) process(next queuedTurn) {
	l.mu.Lock()
	runID := l.runID
	onTurnEnd := l.onTurnEnd
	l.mu.Unlock()

	if coordinator, ok := l.executor.(RunCoordinator); ok {
		if err := coordinator.BeginRun(runID); err != nil {
			if onTurnEnd != nil {
				onTurnEnd(TurnResult{
					RunID: runID,
					Kind:  next.kind,
					Input: next.text,
					Err:   err,
				})
			}
			l.mu.Lock()
			l.finishLocked()
			l.mu.Unlock()
			return
		}
		defer func() { _ = coordinator.EndRun(runID) }()
	}

	for {
		l.mu.Lock()
		if l.state == StateAborting {
			l.finishLocked()
			l.mu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		l.currentCancel = cancel
		l.mu.Unlock()

		out, err := l.executor.Prompt(ctx, runID, next.text)
		cancel()

		result := TurnResult{
			RunID:  runID,
			Kind:   next.kind,
			Input:  next.text,
			Output: out,
			Err:    err,
		}
		if onTurnEnd != nil {
			onTurnEnd(result)
		}

		l.mu.Lock()
		l.currentCancel = nil
		if l.state == StateAborting {
			l.finishLocked()
			l.mu.Unlock()
			return
		}
		if len(l.steers) > 0 {
			next = l.steers[0]
			l.steers = l.steers[1:]
			l.mu.Unlock()
			continue
		}
		if len(l.followUps) > 0 {
			next = l.followUps[0]
			l.followUps = l.followUps[1:]
			l.mu.Unlock()
			continue
		}
		l.finishLocked()
		l.mu.Unlock()
		return
	}
}

func (l *CommandLoop) finishLocked() {
	l.state = StateIdle
	l.runID = ""
	l.steers = nil
	l.followUps = nil
	l.currentCancel = nil
}
