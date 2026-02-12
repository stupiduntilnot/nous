package core

import (
	"context"
	"fmt"
	"strings"
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

type QueueMode string

const (
	TurnPrompt   TurnKind = "prompt"
	TurnSteer    TurnKind = "steer"
	TurnFollowUp TurnKind = "follow_up"

	QueueModeOneAtATime QueueMode = "one-at-a-time"
	QueueModeAll        QueueMode = "all"
)

type TurnResult struct {
	RunID  string
	Kind   TurnKind
	Input  string
	Output string
	Err    error
}

type queuedTurn struct {
	kind      TurnKind
	inputText string
	execText  string
}

type CommandLoop struct {
	executor TurnExecutor

	mu         sync.Mutex
	state      RunState
	runCounter int
	runID      string

	steers    []queuedTurn
	followUps []queuedTurn

	steeringMode QueueMode
	followUpMode QueueMode

	currentCancel context.CancelFunc
	onTurnEnd     func(TurnResult)
}

func NewCommandLoop(executor TurnExecutor) *CommandLoop {
	return &CommandLoop{
		executor:     executor,
		state:        StateIdle,
		steeringMode: QueueModeOneAtATime,
		followUpMode: QueueModeOneAtATime,
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
	return l.PromptWithExecutionText(text, text)
}

func (l *CommandLoop) PromptWithExecutionText(inputText, executionText string) (string, error) {
	if inputText == "" {
		return "", fmt.Errorf("empty_prompt")
	}
	if executionText == "" {
		return "", fmt.Errorf("empty_prompt")
	}

	return l.enqueuePrompt(inputText, executionText)
}

func (l *CommandLoop) enqueuePrompt(inputText, executionText string) (string, error) {
	l.mu.Lock()
	if l.state != StateIdle {
		l.mu.Unlock()
		return "", fmt.Errorf("run_in_progress")
	}

	l.runCounter++
	l.runID = fmt.Sprintf("run-%d", l.runCounter)
	runID := l.runID
	l.state = StateRunning
	initial := queuedTurn{kind: TurnPrompt, inputText: inputText, execText: executionText}
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
	l.steers = append(l.steers, queuedTurn{kind: TurnSteer, inputText: text, execText: text})
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
	l.followUps = append(l.followUps, queuedTurn{kind: TurnFollowUp, inputText: text, execText: text})
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

func (l *CommandLoop) SetSteeringMode(mode QueueMode) error {
	if mode != QueueModeOneAtATime && mode != QueueModeAll {
		return fmt.Errorf("invalid_mode")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.steeringMode = mode
	return nil
}

func (l *CommandLoop) SetFollowUpMode(mode QueueMode) error {
	if mode != QueueModeOneAtATime && mode != QueueModeAll {
		return fmt.Errorf("invalid_mode")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.followUpMode = mode
	return nil
}

func (l *CommandLoop) SteeringMode() QueueMode {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.steeringMode
}

func (l *CommandLoop) FollowUpMode() QueueMode {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.followUpMode
}

func (l *CommandLoop) PendingCounts() (steers, followUps int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.steers), len(l.followUps)
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
					Input: next.inputText,
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
		ctx = withSteerPendingChecker(ctx, func() bool {
			l.mu.Lock()
			defer l.mu.Unlock()
			return len(l.steers) > 0
		})
		l.currentCancel = cancel
		l.mu.Unlock()

		out, err := l.executor.Prompt(ctx, runID, next.execText)
		cancel()

		result := TurnResult{
			RunID:  runID,
			Kind:   next.kind,
			Input:  next.inputText,
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
			next = l.dequeueSteerLocked()
			l.mu.Unlock()
			continue
		}
		if len(l.followUps) > 0 {
			next = l.dequeueFollowUpLocked()
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

func (l *CommandLoop) dequeueSteerLocked() queuedTurn {
	return dequeueQueueLocked(&l.steers, TurnSteer, l.steeringMode)
}

func (l *CommandLoop) dequeueFollowUpLocked() queuedTurn {
	return dequeueQueueLocked(&l.followUps, TurnFollowUp, l.followUpMode)
}

func dequeueQueueLocked(queue *[]queuedTurn, kind TurnKind, mode QueueMode) queuedTurn {
	items := *queue
	if len(items) == 0 {
		return queuedTurn{kind: kind, inputText: "", execText: ""}
	}
	if mode != QueueModeAll || len(items) == 1 {
		next := items[0]
		*queue = items[1:]
		return next
	}
	inputs := make([]string, 0, len(items))
	executions := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.inputText) == "" || strings.TrimSpace(item.execText) == "" {
			continue
		}
		inputs = append(inputs, item.inputText)
		executions = append(executions, item.execText)
	}
	*queue = nil
	return queuedTurn{
		kind:      kind,
		inputText: strings.Join(inputs, "\n"),
		execText:  strings.Join(executions, "\n"),
	}
}
