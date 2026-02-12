package extension

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrTimeout = errors.New("extension_timeout")

type TimeoutError struct {
	Operation string
	Timeout   time.Duration
}

func (e TimeoutError) Error() string {
	return fmt.Sprintf("extension_timeout: %s exceeded %s", e.Operation, e.Timeout)
}

func (e TimeoutError) Is(target error) bool {
	return target == ErrTimeout
}

type ToolHandler func(args map[string]any) (string, error)
type CommandHandler func(payload map[string]any) (map[string]any, error)

type InputHookInput struct {
	Text string
}

type InputHookOutput struct {
	Text    string
	Handled bool
}

type ToolCallHookInput struct {
	ToolName string
	Args     map[string]any
}

type ToolCallHookOutput struct {
	Blocked bool
	Reason  string
}

type ToolResultHookInput struct {
	ToolName string
	Result   string
}

type ToolResultHookOutput struct {
	Result string
}

type TurnEndHookInput struct {
	RunID string
	Turn  int
}

type RunStartHookInput struct {
	RunID string
}

type BeforeAgentStartHookInput struct {
	RunID string
}

type RunEndHookInput struct {
	RunID string
	Turn  int
}

type TurnStartHookInput struct {
	RunID string
	Turn  int
}

type SessionBeforeSwitchHookInput struct {
	CurrentSessionID string
	TargetSessionID  string
	Reason           string
}

type SessionBeforeSwitchHookOutput struct {
	Cancel bool
	Reason string
}

type SessionBeforeForkHookInput struct {
	ParentSessionID string
}

type SessionBeforeForkHookOutput struct {
	Cancel bool
	Reason string
}

type InputHook func(InputHookInput) (InputHookOutput, error)
type ToolCallHook func(ToolCallHookInput) (ToolCallHookOutput, error)
type ToolResultHook func(ToolResultHookInput) (ToolResultHookOutput, error)
type TurnEndHook func(TurnEndHookInput) error
type RunStartHook func(RunStartHookInput) error
type BeforeAgentStartHook func(BeforeAgentStartHookInput) error
type RunEndHook func(RunEndHookInput) error
type TurnStartHook func(TurnStartHookInput) error
type SessionBeforeSwitchHook func(SessionBeforeSwitchHookInput) (SessionBeforeSwitchHookOutput, error)
type SessionBeforeForkHook func(SessionBeforeForkHookInput) (SessionBeforeForkHookOutput, error)

type Manager struct {
	mu sync.RWMutex

	tools    map[string]ToolHandler
	commands map[string]CommandHandler

	inputHooks               []InputHook
	toolCallHooks            []ToolCallHook
	toolResultHooks          []ToolResultHook
	turnEndHooks             []TurnEndHook
	runStartHooks            []RunStartHook
	beforeAgentStartHooks    []BeforeAgentStartHook
	runEndHooks              []RunEndHook
	turnStartHooks           []TurnStartHook
	sessionBeforeSwitchHooks []SessionBeforeSwitchHook
	sessionBeforeForkHooks   []SessionBeforeForkHook

	hookTimeout time.Duration
	toolTimeout time.Duration
}

func NewManager() *Manager {
	return &Manager{
		tools:    map[string]ToolHandler{},
		commands: map[string]CommandHandler{},
	}
}

func (m *Manager) RegisterTool(name string, handler ToolHandler) error {
	if name == "" || handler == nil {
		return fmt.Errorf("invalid_tool_registration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[name] = handler
	return nil
}

func (m *Manager) SetHookTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("invalid_hook_timeout")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hookTimeout = timeout
	return nil
}

func (m *Manager) HookTimeout() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hookTimeout
}

func (m *Manager) SetToolTimeout(timeout time.Duration) error {
	if timeout < 0 {
		return fmt.Errorf("invalid_tool_timeout")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolTimeout = timeout
	return nil
}

func (m *Manager) ToolTimeout() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.toolTimeout
}

func (m *Manager) RegisterCommand(name string, handler CommandHandler) error {
	if name == "" || handler == nil {
		return fmt.Errorf("invalid_command_registration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[name] = handler
	return nil
}

func (m *Manager) ExecuteTool(name string, args map[string]any) (result string, handled bool, err error) {
	m.mu.RLock()
	h, ok := m.tools[name]
	timeout := m.toolTimeout
	m.mu.RUnlock()
	if !ok {
		return "", false, nil
	}
	out, err := runWithTimeoutValue(timeout, "tool:"+name, func() (string, error) {
		return h(args)
	})
	if err != nil {
		return "", true, err
	}
	return out, true, nil
}

func (m *Manager) ExecuteCommand(name string, payload map[string]any) (result map[string]any, handled bool, err error) {
	m.mu.RLock()
	h, ok := m.commands[name]
	m.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	out, err := h(payload)
	if err != nil {
		return nil, true, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, true, nil
}

func (m *Manager) RegisterInputHook(h InputHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inputHooks = append(m.inputHooks, h)
}

func (m *Manager) RegisterToolCallHook(h ToolCallHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolCallHooks = append(m.toolCallHooks, h)
}

func (m *Manager) RegisterToolResultHook(h ToolResultHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolResultHooks = append(m.toolResultHooks, h)
}

func (m *Manager) RegisterTurnEndHook(h TurnEndHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnEndHooks = append(m.turnEndHooks, h)
}

func (m *Manager) RegisterRunStartHook(h RunStartHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runStartHooks = append(m.runStartHooks, h)
}

func (m *Manager) RegisterBeforeAgentStartHook(h BeforeAgentStartHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beforeAgentStartHooks = append(m.beforeAgentStartHooks, h)
}

func (m *Manager) RegisterRunEndHook(h RunEndHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runEndHooks = append(m.runEndHooks, h)
}

func (m *Manager) RegisterTurnStartHook(h TurnStartHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.turnStartHooks = append(m.turnStartHooks, h)
}

func (m *Manager) RegisterSessionBeforeSwitchHook(h SessionBeforeSwitchHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionBeforeSwitchHooks = append(m.sessionBeforeSwitchHooks, h)
}

func (m *Manager) RegisterSessionBeforeForkHook(h SessionBeforeForkHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionBeforeForkHooks = append(m.sessionBeforeForkHooks, h)
}

func (m *Manager) RunInputHooks(text string) (InputHookOutput, error) {
	m.mu.RLock()
	hooks := append([]InputHook(nil), m.inputHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	out := InputHookOutput{Text: text}
	for _, h := range hooks {
		next, err := runWithTimeoutValue(timeout, "input_hook", func() (InputHookOutput, error) {
			return h(InputHookInput{Text: out.Text})
		})
		if err != nil {
			return InputHookOutput{}, err
		}
		if next.Text != "" {
			out.Text = next.Text
		}
		if next.Handled {
			out.Handled = true
			return out, nil
		}
	}
	return out, nil
}

func (m *Manager) RunToolCallHooks(toolName string, args map[string]any) (ToolCallHookOutput, error) {
	m.mu.RLock()
	hooks := append([]ToolCallHook(nil), m.toolCallHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		out, err := runWithTimeoutValue(timeout, "tool_call_hook:"+toolName, func() (ToolCallHookOutput, error) {
			return h(ToolCallHookInput{ToolName: toolName, Args: args})
		})
		if err != nil {
			return ToolCallHookOutput{}, err
		}
		if out.Blocked {
			if out.Reason == "" {
				out.Reason = "blocked_by_extension"
			}
			return out, nil
		}
	}
	return ToolCallHookOutput{}, nil
}

func (m *Manager) RunToolResultHooks(toolName, result string) (ToolResultHookOutput, error) {
	m.mu.RLock()
	hooks := append([]ToolResultHook(nil), m.toolResultHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	out := ToolResultHookOutput{Result: result}
	for _, h := range hooks {
		next, err := runWithTimeoutValue(timeout, "tool_result_hook:"+toolName, func() (ToolResultHookOutput, error) {
			return h(ToolResultHookInput{ToolName: toolName, Result: out.Result})
		})
		if err != nil {
			return ToolResultHookOutput{}, err
		}
		if next.Result != "" {
			out.Result = next.Result
		}
	}
	return out, nil
}

func (m *Manager) RunTurnEndHooks(runID string, turn int) error {
	m.mu.RLock()
	hooks := append([]TurnEndHook(nil), m.turnEndHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := runWithTimeoutErr(timeout, "turn_end_hook", func() error {
			return h(TurnEndHookInput{RunID: runID, Turn: turn})
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RunRunStartHooks(runID string) error {
	m.mu.RLock()
	hooks := append([]RunStartHook(nil), m.runStartHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := runWithTimeoutErr(timeout, "run_start_hook", func() error {
			return h(RunStartHookInput{RunID: runID})
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RunBeforeAgentStartHooks(runID string) error {
	m.mu.RLock()
	hooks := append([]BeforeAgentStartHook(nil), m.beforeAgentStartHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := runWithTimeoutErr(timeout, "before_agent_start_hook", func() error {
			return h(BeforeAgentStartHookInput{RunID: runID})
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RunRunEndHooks(runID string, turn int) error {
	m.mu.RLock()
	hooks := append([]RunEndHook(nil), m.runEndHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := runWithTimeoutErr(timeout, "run_end_hook", func() error {
			return h(RunEndHookInput{RunID: runID, Turn: turn})
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RunTurnStartHooks(runID string, turn int) error {
	m.mu.RLock()
	hooks := append([]TurnStartHook(nil), m.turnStartHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := runWithTimeoutErr(timeout, "turn_start_hook", func() error {
			return h(TurnStartHookInput{RunID: runID, Turn: turn})
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) RunSessionBeforeSwitchHooks(currentSessionID, targetSessionID, reason string) (SessionBeforeSwitchHookOutput, error) {
	m.mu.RLock()
	hooks := append([]SessionBeforeSwitchHook(nil), m.sessionBeforeSwitchHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		out, err := runWithTimeoutValue(timeout, "session_before_switch_hook", func() (SessionBeforeSwitchHookOutput, error) {
			return h(SessionBeforeSwitchHookInput{
				CurrentSessionID: currentSessionID,
				TargetSessionID:  targetSessionID,
				Reason:           reason,
			})
		})
		if err != nil {
			return SessionBeforeSwitchHookOutput{}, err
		}
		if out.Cancel {
			if out.Reason == "" {
				out.Reason = "cancelled_by_extension"
			}
			return out, nil
		}
	}
	return SessionBeforeSwitchHookOutput{}, nil
}

func (m *Manager) RunSessionBeforeForkHooks(parentSessionID string) (SessionBeforeForkHookOutput, error) {
	m.mu.RLock()
	hooks := append([]SessionBeforeForkHook(nil), m.sessionBeforeForkHooks...)
	timeout := m.hookTimeout
	m.mu.RUnlock()

	for _, h := range hooks {
		out, err := runWithTimeoutValue(timeout, "session_before_fork_hook", func() (SessionBeforeForkHookOutput, error) {
			return h(SessionBeforeForkHookInput{
				ParentSessionID: parentSessionID,
			})
		})
		if err != nil {
			return SessionBeforeForkHookOutput{}, err
		}
		if out.Cancel {
			if out.Reason == "" {
				out.Reason = "cancelled_by_extension"
			}
			return out, nil
		}
	}
	return SessionBeforeForkHookOutput{}, nil
}

func runWithTimeoutErr(timeout time.Duration, operation string, fn func() error) error {
	if timeout <= 0 {
		return fn()
	}
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return TimeoutError{Operation: operation, Timeout: timeout}
	}
}

func runWithTimeoutValue[T any](timeout time.Duration, operation string, fn func() (T, error)) (T, error) {
	if timeout <= 0 {
		return fn()
	}
	type valueResult struct {
		val T
		err error
	}
	done := make(chan valueResult, 1)
	go func() {
		v, err := fn()
		done <- valueResult{val: v, err: err}
	}()
	select {
	case out := <-done:
		return out.val, out.err
	case <-time.After(timeout):
		var zero T
		return zero, TimeoutError{Operation: operation, Timeout: timeout}
	}
}
