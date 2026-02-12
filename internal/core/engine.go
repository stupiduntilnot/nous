package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"nous/internal/extension"
	"nous/internal/provider"
)

type Engine struct {
	runtime  *Runtime
	provider provider.Adapter
	tools    map[string]Tool
	active   map[string]struct{}
	ext      *extension.Manager

	transformContext TransformContextFn
	convertToLLM     ConvertToLLMFn
}

type TransformContextFn func(ctx context.Context, messages []Message) ([]Message, error)
type ConvertToLLMFn func(messages []Message) ([]provider.Message, error)

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

func (e *Engine) ExtensionManager() *extension.Manager {
	return e.ext
}

func (e *Engine) SetTransformContext(fn TransformContextFn) {
	e.transformContext = fn
}

func (e *Engine) SetConvertToLLM(fn ConvertToLLMFn) {
	e.convertToLLM = fn
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

func (e *Engine) BeginRun(runID string) error {
	if err := e.runtime.StartRun(runID); err != nil {
		return err
	}
	if e.ext != nil {
		if err := e.ext.RunBeforeAgentStartHooks(runID); err != nil {
			e.runtime.Warning("extension_hook_error", fmt.Sprintf("before_agent_start: %v", err))
		}
		if err := e.ext.RunRunStartHooks(runID); err != nil {
			e.runtime.Warning("extension_hook_error", fmt.Sprintf("run_start: %v", err))
		}
	}
	return nil
}

func (e *Engine) EndRun(runID string) error {
	if state := e.runtime.State(); state != StateRunning && state != StateAborting {
		return fmt.Errorf("run_not_active")
	}
	if active := e.runtime.RunID(); active != runID {
		return fmt.Errorf("run_id_mismatch: active=%s requested=%s", active, runID)
	}
	if e.ext != nil {
		if err := e.ext.RunRunEndHooks(runID, e.runtime.TurnNumber()); err != nil {
			e.runtime.Warning("extension_hook_error", fmt.Sprintf("run_end: %v", err))
		}
	}
	return e.runtime.EndRun()
}

func (e *Engine) Prompt(ctx context.Context, runID, prompt string) (string, error) {
	if e.ext != nil {
		out, err := e.ext.RunInputHooks(prompt)
		if err != nil {
			if errors.Is(err, extension.ErrTimeout) {
				e.runtime.Warning("extension_timeout", fmt.Sprintf("input_hook: %v", err))
			} else {
				return "", err
			}
		} else {
			prompt = out.Text
			if out.Handled {
				return prompt, nil
			}
		}
	}

	managedRun := false
	switch e.runtime.State() {
	case StateIdle:
		if err := e.BeginRun(runID); err != nil {
			return "", err
		}
		managedRun = true
	case StateRunning, StateAborting:
		if active := e.runtime.RunID(); active != runID {
			return "", fmt.Errorf("run_id_mismatch: active=%s requested=%s", active, runID)
		}
	default:
		return "", fmt.Errorf("invalid_runtime_state: %s", e.runtime.State())
	}
	defer func() {
		if managedRun {
			_ = e.EndRun(runID)
		}
	}()

	if err := e.runtime.StartTurn(); err != nil {
		return "", err
	}
	if e.ext != nil {
		if err := e.ext.RunTurnStartHooks(runID, e.runtime.TurnNumber()); err != nil {
			e.runtime.Warning("extension_hook_error", fmt.Sprintf("turn_start: %v", err))
		}
	}
	defer func() {
		_ = e.runtime.EndTurn()
		if e.ext != nil {
			if err := e.ext.RunTurnEndHooks(runID, e.runtime.TurnNumber()); err != nil {
				e.runtime.Warning("extension_hook_error", fmt.Sprintf("turn_end: %v", err))
			}
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
	messages := []Message{{Role: RoleUser, Text: prompt}}
	llmMessages, err := e.buildProviderMessages(ctx, messages)
	if err != nil {
		return "", err
	}
	req := provider.Request{
		Messages:    llmMessages,
		ActiveTools: e.activeToolNames(),
	}
	type toolResult struct {
		CallID string
		Name   string
		Result string
	}
	for step := 0; step < 8; step++ {
		awaitNext := false
		stepToolResults := make([]toolResult, 0, 4)
		stepToolCalls := make([]provider.ToolCall, 0, 4)
		var stepAssistant string
		steeringQueued := steerPendingCheckerFromContext(ctx)
		interruptTools := false

		for ev := range e.provider.Stream(ctx, req) {
			switch ev.Type {
			case provider.EventTextDelta:
				final += ev.Delta
				stepAssistant += ev.Delta
				if err := e.runtime.MessageUpdate(assistantID, ev.Delta); err != nil {
					return "", err
				}
			case provider.EventToolCall:
				stepToolCalls = append(stepToolCalls, provider.ToolCall{
					ID:        ev.ToolCall.ID,
					Name:      ev.ToolCall.Name,
					Arguments: cloneMap(ev.ToolCall.Arguments),
				})
				var (
					res string
					err error
				)
				if interruptTools {
					res, err = e.skipToolCall(ev.ToolCall, "Skipped due to queued user message.")
				} else {
					res, err = e.executeToolCall(ctx, ev.ToolCall)
				}
				if err != nil {
					return "", err
				}
				final += res
				stepToolResults = append(stepToolResults, toolResult{
					CallID: ev.ToolCall.ID,
					Name:   ev.ToolCall.Name,
					Result: fmt.Sprintf("%s => %s", ev.ToolCall.Name, res),
				})
				if err := e.runtime.MessageUpdate(assistantID, res); err != nil {
					return "", err
				}
				if !interruptTools && steeringQueued != nil && steeringQueued() {
					interruptTools = true
				}
			case provider.EventAwaitNext:
				awaitNext = true
			case provider.EventDone:
				if ev.StopReason != "" &&
					ev.StopReason != provider.StopReasonUnknown &&
					ev.StopReason != provider.StopReasonStop {
					e.runtime.Status(fmt.Sprintf("provider_stop_reason: %s", ev.StopReason))
				}
				if ev.Usage != nil {
					e.runtime.Status(fmt.Sprintf(
						"provider_usage: input=%d output=%d total=%d",
						ev.Usage.InputTokens,
						ev.Usage.OutputTokens,
						ev.Usage.TotalTokens,
					))
				}
			case provider.EventStatus:
				if strings.TrimSpace(ev.Message) != "" {
					e.runtime.Status(strings.TrimSpace(ev.Message))
				}
			case provider.EventWarning:
				code := strings.TrimSpace(ev.Code)
				if code == "" {
					code = "provider_warning"
				}
				message := strings.TrimSpace(ev.Message)
				if message == "" {
					message = code
				}
				e.runtime.Warning(code, message)
			case provider.EventError:
				if ev.Err != nil {
					if provider.IsAbortedError(ev.Err) || errors.Is(ev.Err, context.Canceled) || errors.Is(ev.Err, context.DeadlineExceeded) {
						reason := strings.TrimSpace(provider.AbortReason(ev.Err))
						if reason == "" {
							reason = "provider request aborted"
						}
						e.runtime.Warning("provider_aborted", reason)
						return "", ev.Err
					}
					if provider.IsRetryExhaustedError(ev.Err) {
						e.runtime.Warning("provider_retry_exhausted", ev.Err.Error())
					}
					e.runtime.Error("provider_error", "provider stream returned error", ev.Err)
					return "", ev.Err
				}
				err := fmt.Errorf("provider_error")
				e.runtime.Error("provider_error", "provider stream returned error", err)
				return "", err
			}
		}
		if strings.TrimSpace(stepAssistant) != "" || len(stepToolCalls) > 0 {
			assistantMsg := Message{
				Role: RoleAssistant,
				Text: strings.TrimSpace(stepAssistant),
			}
			if assistantMsg.Text != "" {
				assistantMsg.Blocks = append(assistantMsg.Blocks, MessageBlock{Type: BlockTypeText, Text: assistantMsg.Text})
			}
			for _, call := range stepToolCalls {
				assistantMsg.Blocks = append(assistantMsg.Blocks, MessageBlock{
					Type:       BlockTypeToolCall,
					ToolCallID: call.ID,
					ToolName:   call.Name,
					Arguments:  cloneMap(call.Arguments),
				})
			}
			messages = append(messages, assistantMsg)
		}
		// Continue the run whenever this turn produced tool results.
		// Some providers do not emit explicit await-next markers even when
		// tool calls are present, but pi-style semantics still require
		// tool_result -> next model turn until convergence.
		if len(stepToolResults) > 0 {
			if step == 7 {
				err := fmt.Errorf("tool_loop_limit_exceeded")
				e.runtime.Error("tool_loop_limit_exceeded", "tool await-next loop exceeded max rounds", err)
				return "", err
			}
			for _, item := range stepToolResults {
				messages = appendToolResultMessage(messages, item.CallID, item.Name, item.Result)
			}
			next, err := e.buildProviderMessages(ctx, messages)
			if err != nil {
				return "", err
			}
			req.Messages = next
			if awaitNext {
				e.runtime.Status("await_next: continue_with_tool_results")
			}
			continue
		}
		break
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

	normalizedArgs, err := normalizeToolArguments(call.Name, call.Arguments)
	if err != nil {
		e.runtime.Warning("tool_validation_error", err.Error())
		return fmt.Sprintf("tool_error: %s", err.Error()), nil
	}
	call.Arguments = normalizedArgs

	if e.ext != nil {
		hookOut, err := e.ext.RunToolCallHooks(call.Name, call.Arguments)
		if err != nil {
			if errors.Is(err, extension.ErrTimeout) {
				e.runtime.Warning("extension_timeout", fmt.Sprintf("tool_call_hook(%s): %v", call.Name, err))
			} else {
				e.runtime.Error("extension_error", "tool_call hook failed", err)
				return "", err
			}
		}
		if hookOut.Blocked {
			err := fmt.Errorf("tool_blocked: %s", hookOut.Reason)
			e.runtime.Warning("tool_blocked", err.Error())
			return "", err
		}
	}
	tool, ok := e.tools[call.Name]
	if !ok {
		if e.ext != nil {
			extResult, handled, err := e.ext.ExecuteTool(call.Name, call.Arguments)
			if err != nil {
				if errors.Is(err, extension.ErrTimeout) {
					e.runtime.Warning("extension_timeout", fmt.Sprintf("extension_tool(%s): %v", call.Name, err))
					return fmt.Sprintf("tool_error: %v", err), nil
				}
				e.runtime.Error("extension_error", "extension tool execution failed", err)
				return "", err
			}
			if handled {
				result := extResult
				if e.ext != nil {
					mutated, err := e.ext.RunToolResultHooks(call.Name, result)
					if err != nil {
						if errors.Is(err, extension.ErrTimeout) {
							e.runtime.Warning("extension_timeout", fmt.Sprintf("tool_result_hook(%s): %v", call.Name, err))
						} else {
							e.runtime.Error("extension_error", "tool_result hook failed", err)
							return "", err
						}
					} else {
						result = mutated.Result
					}
				}
				if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
					return "", err
				}
				return result, nil
			}
		}
		err := fmt.Errorf("tool_not_found: %s", call.Name)
		e.runtime.Warning("tool_not_found", err.Error())
		return "", err
	}
	if _, active := e.active[call.Name]; !active {
		err := fmt.Errorf("tool_not_active: %s", call.Name)
		e.runtime.Warning("tool_not_active", err.Error())
		return fmt.Sprintf("tool_error: %s", err.Error()), nil
	}
	result, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		e.runtime.Warning("tool_execution_error", err.Error())
		return fmt.Sprintf("tool_error: %v", err), nil
	}
	if e.ext != nil {
		mutated, err := e.ext.RunToolResultHooks(call.Name, result)
		if err != nil {
			if errors.Is(err, extension.ErrTimeout) {
				e.runtime.Warning("extension_timeout", fmt.Sprintf("tool_result_hook(%s): %v", call.Name, err))
			} else {
				e.runtime.Error("extension_error", "tool_result hook failed", err)
				return "", err
			}
		} else {
			result = mutated.Result
		}
	}
	if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
		return "", err
	}
	return result, nil
}

func (e *Engine) skipToolCall(call provider.ToolCall, reason string) (string, error) {
	if err := e.runtime.ToolExecutionStart(call.ID, call.Name); err != nil {
		return "", err
	}
	defer func() { _ = e.runtime.ToolExecutionEnd(call.ID, call.Name) }()

	e.runtime.Warning("tool_execution_skipped", reason)
	result := "tool_error: " + reason
	if err := e.runtime.ToolExecutionUpdate(call.ID, call.Name, result); err != nil {
		return "", err
	}
	return result, nil
}

func (e *Engine) buildProviderMessages(ctx context.Context, messages []Message) ([]provider.Message, error) {
	current := cloneMessages(messages)
	if e.transformContext != nil {
		next, err := e.transformContext(ctx, cloneMessages(current))
		if err != nil {
			return nil, err
		}
		current = cloneMessages(next)
	}
	if e.convertToLLM != nil {
		return e.convertToLLM(cloneMessages(current))
	}
	return defaultConvertToLLMMessages(current), nil
}
