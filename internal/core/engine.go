package core

import (
	"context"
	"fmt"
	"time"

	"oh-my-agent/internal/provider"
)

type Engine struct {
	runtime  *Runtime
	provider provider.Adapter
}

func NewEngine(runtime *Runtime, p provider.Adapter) *Engine {
	return &Engine{runtime: runtime, provider: p}
}

func (e *Engine) Prompt(ctx context.Context, runID, prompt string) (string, error) {
	if err := e.runtime.StartRun(runID); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.EndRun() }()

	if err := e.runtime.StartTurn(); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.EndTurn() }()

	userID := fmt.Sprintf("user-%d", time.Now().UnixNano())
	if err := e.runtime.MessageStart(userID, "user"); err != nil {
		return "", err
	}
	if err := e.runtime.MessageEnd(userID); err != nil {
		return "", err
	}

	assistantID := fmt.Sprintf("assistant-%d", time.Now().UnixNano())
	if err := e.runtime.MessageStart(assistantID, "assistant"); err != nil {
		return "", err
	}

	var final string
	for ev := range e.provider.Stream(ctx, prompt) {
		switch ev.Type {
		case provider.EventTextDelta:
			final += ev.Delta
			if err := e.runtime.MessageUpdate(assistantID, ev.Delta); err != nil {
				return "", err
			}
		case provider.EventError:
			if ev.Err != nil {
				return "", ev.Err
			}
			return "", fmt.Errorf("provider_error")
		}
	}

	if err := e.runtime.MessageEnd(assistantID); err != nil {
		return "", err
	}
	return final, nil
}
