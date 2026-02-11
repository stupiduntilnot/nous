package extension

import (
	"fmt"
	"sync"
)

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

type InputHook func(InputHookInput) (InputHookOutput, error)
type ToolCallHook func(ToolCallHookInput) (ToolCallHookOutput, error)
type ToolResultHook func(ToolResultHookInput) (ToolResultHookOutput, error)
type TurnEndHook func(TurnEndHookInput) error

type Manager struct {
	mu sync.RWMutex

	tools    map[string]ToolHandler
	commands map[string]CommandHandler

	inputHooks      []InputHook
	toolCallHooks   []ToolCallHook
	toolResultHooks []ToolResultHook
	turnEndHooks    []TurnEndHook
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

func (m *Manager) RegisterCommand(name string, handler CommandHandler) error {
	if name == "" || handler == nil {
		return fmt.Errorf("invalid_command_registration")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commands[name] = handler
	return nil
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

func (m *Manager) RunInputHooks(text string) (InputHookOutput, error) {
	m.mu.RLock()
	hooks := append([]InputHook(nil), m.inputHooks...)
	m.mu.RUnlock()

	out := InputHookOutput{Text: text}
	for _, h := range hooks {
		next, err := h(InputHookInput{Text: out.Text})
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
	m.mu.RUnlock()

	for _, h := range hooks {
		out, err := h(ToolCallHookInput{ToolName: toolName, Args: args})
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
	m.mu.RUnlock()

	out := ToolResultHookOutput{Result: result}
	for _, h := range hooks {
		next, err := h(ToolResultHookInput{ToolName: toolName, Result: out.Result})
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
	m.mu.RUnlock()

	for _, h := range hooks {
		if err := h(TurnEndHookInput{RunID: runID, Turn: turn}); err != nil {
			return err
		}
	}
	return nil
}
