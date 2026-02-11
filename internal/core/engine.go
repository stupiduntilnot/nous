package core

import (
	"context"
	"fmt"
	"slices"
	"time"

	"oh-my-agent/internal/provider"
)

type Engine struct {
	runtime  *Runtime
	provider provider.Adapter
	tools    map[string]Tool
	active   map[string]struct{}
}

func NewEngine(runtime *Runtime, p provider.Adapter) *Engine {
	return &Engine{
		runtime:  runtime,
		provider: p,
		tools:    map[string]Tool{},
		active:   map[string]struct{}{},
	}
}

func (e *Engine) SetTools(tools []Tool) {
	e.tools = map[string]Tool{}
	e.active = map[string]struct{}{}
	for _, t := range tools {
		e.tools[t.Name()] = t
		e.active[t.Name()] = struct{}{}
	}
}

func (e *Engine) SetActiveTools(names []string) error {
	next := map[string]struct{}{}
	for _, name := range names {
		if _, ok := e.tools[name]; !ok {
			return fmt.Errorf("tool_not_registered: %s", name)
		}
		next[name] = struct{}{}
	}
	e.active = next
	return nil
}

func (e *Engine) activeToolNames() []string {
	out := make([]string, 0, len(e.active))
	for name := range e.active {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
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
	req := provider.Request{
		Prompt:      prompt,
		ActiveTools: e.activeToolNames(),
	}
	for ev := range e.provider.Stream(ctx, req) {
		switch ev.Type {
		case provider.EventTextDelta:
			final += ev.Delta
			if err := e.runtime.MessageUpdate(assistantID, ev.Delta); err != nil {
				return "", err
			}
		case provider.EventToolCall:
			res, err := e.executeToolCall(ctx, ev.ToolCall)
			if err != nil {
				return "", err
			}
			final += res
			if err := e.runtime.MessageUpdate(assistantID, res); err != nil {
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

func (e *Engine) executeToolCall(ctx context.Context, call provider.ToolCall) (string, error) {
	if err := e.runtime.ToolExecutionStart(call.ID, call.Name); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.ToolExecutionEnd(call.ID, call.Name) }()

	if _, active := e.active[call.Name]; !active {
		return "", fmt.Errorf("tool_not_active: %s", call.Name)
	}
	tool, ok := e.tools[call.Name]
	if !ok {
		return "", fmt.Errorf("tool_not_found: %s", call.Name)
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return "", err
	}
	if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
		return "", err
	}
	return result, nil
}
