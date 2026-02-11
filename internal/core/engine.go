package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"oh-my-agent/internal/extension"
	"oh-my-agent/internal/provider"
)

type Engine struct {
	runtime  *Runtime
	provider provider.Adapter
	tools    map[string]Tool
	active   map[string]struct{}
	ext      *extension.Manager
}

func NewEngine(runtime *Runtime, p provider.Adapter) *Engine {
	return &Engine{
		runtime:  runtime,
		provider: p,
		tools:    map[string]Tool{},
		active:   map[string]struct{}{},
	}
}

func (e *Engine) SetExtensionManager(m *extension.Manager) {
	e.ext = m
}

func (e *Engine) ExecuteExtensionCommand(name string, payload map[string]any) (map[string]any, error) {
	if e.ext == nil {
		return nil, fmt.Errorf("extension_not_configured")
	}
	out, handled, err := e.ext.ExecuteCommand(name, payload)
	if err != nil {
		return nil, err
	}
	if !handled {
		return nil, fmt.Errorf("extension_command_not_found: %s", name)
	}
	return out, nil
}

func (e *Engine) Subscribe(fn EventListener) func() {
	return e.runtime.Subscribe(fn)
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
	if e.ext != nil {
		out, err := e.ext.RunInputHooks(prompt)
		if err != nil {
			return "", err
		}
		prompt = out.Text
		if out.Handled {
			return prompt, nil
		}
	}

	if err := e.runtime.StartRun(runID); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.EndRun() }()

	if err := e.runtime.StartTurn(); err != nil {
		return "", err
	}
	defer func() {
		_ = e.runtime.EndTurn()
		if e.ext != nil {
			_ = e.ext.RunTurnEndHooks(runID, e.runtime.TurnNumber())
		}
	}()

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
	for step := 0; step < 8; step++ {
		awaitNext := false
		stepToolResults := make([]string, 0, 4)

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
				stepToolResults = append(stepToolResults, fmt.Sprintf("%s => %s", ev.ToolCall.Name, res))
				if err := e.runtime.MessageUpdate(assistantID, res); err != nil {
					return "", err
				}
			case provider.EventAwaitNext:
				awaitNext = true
			case provider.EventError:
				if ev.Err != nil {
					return "", ev.Err
				}
				return "", fmt.Errorf("provider_error")
			}
		}
		if awaitNext && len(stepToolResults) > 0 {
			if step == 7 {
				return "", fmt.Errorf("tool_loop_limit_exceeded")
			}
			req.ToolResults = append(req.ToolResults, stepToolResults...)
			req.Prompt = appendToolResultsToPrompt(req.Prompt, stepToolResults)
			continue
		}
		break
	}

	if err := e.runtime.MessageEnd(assistantID); err != nil {
		return "", err
	}
	return final, nil
}

func appendToolResultsToPrompt(prompt string, toolResults []string) string {
	if len(toolResults) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString(prompt)
	b.WriteString("\n\nTool results:\n")
	for _, line := range toolResults {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

func (e *Engine) executeToolCall(ctx context.Context, call provider.ToolCall) (string, error) {
	if err := e.runtime.ToolExecutionStart(call.ID, call.Name); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.ToolExecutionEnd(call.ID, call.Name) }()

	if e.ext != nil {
		hookOut, err := e.ext.RunToolCallHooks(call.Name, call.Arguments)
		if err != nil {
			return "", err
		}
		if hookOut.Blocked {
			return "", fmt.Errorf("tool_blocked: %s", hookOut.Reason)
		}
	}
	tool, ok := e.tools[call.Name]
	if !ok {
		if e.ext != nil {
			extResult, handled, err := e.ext.ExecuteTool(call.Name, call.Arguments)
			if err != nil {
				return "", err
			}
			if handled {
				result := extResult
				if e.ext != nil {
					mutated, err := e.ext.RunToolResultHooks(call.Name, result)
					if err != nil {
						return "", err
					}
					result = mutated.Result
				}
				if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
					return "", err
				}
				return result, nil
			}
		}
		return "", fmt.Errorf("tool_not_found: %s", call.Name)
	}
	if _, active := e.active[call.Name]; !active {
		return "", fmt.Errorf("tool_not_active: %s", call.Name)
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return "", err
	}
	if e.ext != nil {
		mutated, err := e.ext.RunToolResultHooks(call.Name, result)
		if err != nil {
			return "", err
		}
		result = mutated.Result
	}
	if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
		return "", err
	}
	return result, nil
}
