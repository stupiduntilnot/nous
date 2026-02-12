package core

import (
	"encoding/json"
	"fmt"
	"strings"

	"nous/internal/provider"
)

type MessageRole string

const (
	RoleUser       MessageRole = "user"
	RoleAssistant  MessageRole = "assistant"
	RoleToolResult MessageRole = "tool_result"
	RoleCustom     MessageRole = "custom"
)

const (
	BlockTypeText       = "text"
	BlockTypeToolCall   = "tool_call"
	BlockTypeToolResult = "tool_result"
	BlockTypeThinking   = "thinking"
)

type MessageBlock struct {
	Type       string
	Text       string
	ToolCallID string
	ToolName   string
	Arguments  map[string]any
}

type Message struct {
	Role       MessageRole
	Text       string
	Blocks     []MessageBlock
	ToolCallID string
}

func appendMessage(messages []Message, role MessageRole, text string) []Message {
	text = strings.TrimSpace(text)
	if text == "" {
		return messages
	}
	return append(messages, Message{
		Role: role,
		Text: text,
		Blocks: []MessageBlock{
			{Type: BlockTypeText, Text: text},
		},
	})
}

func appendToolResultMessage(messages []Message, toolCallID, toolName, result string) []Message {
	result = strings.TrimSpace(result)
	if result == "" {
		return messages
	}
	return append(messages, Message{
		Role:       RoleToolResult,
		Text:       result,
		ToolCallID: strings.TrimSpace(toolCallID),
		Blocks: []MessageBlock{
			{
				Type:       BlockTypeToolResult,
				Text:       result,
				ToolCallID: strings.TrimSpace(toolCallID),
				ToolName:   strings.TrimSpace(toolName),
			},
		},
	})
}

func providerMessagesFromCore(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "" {
			continue
		}
		content := strings.TrimSpace(msg.Text)
		if content == "" {
			content = renderBlocksAsText(msg.Blocks)
		}
		if content == "" && len(msg.Blocks) == 0 && len(msg.ToolCallID) == 0 {
			continue
		}
		out = append(out, provider.Message{
			Role:       string(msg.Role),
			Content:    content,
			Blocks:     providerBlocksFromCore(msg.Blocks),
			ToolCallID: strings.TrimSpace(msg.ToolCallID),
		})
	}
	return out
}

func defaultConvertToLLMMessages(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case RoleUser, RoleAssistant, RoleToolResult:
			out = appendConvertedProviderMessage(out, msg)
		}
	}
	return out
}

func appendConvertedProviderMessage(out []provider.Message, msg Message) []provider.Message {
	content := strings.TrimSpace(msg.Text)
	if content == "" {
		content = renderBlocksAsText(msg.Blocks)
	}
	toolCalls := extractToolCalls(msg.Blocks)
	if content == "" && len(msg.Blocks) == 0 && len(toolCalls) == 0 {
		return out
	}
	return append(out, provider.Message{
		Role:       string(msg.Role),
		Content:    content,
		Blocks:     providerBlocksFromCore(msg.Blocks),
		ToolCallID: strings.TrimSpace(msg.ToolCallID),
		ToolCalls:  toolCalls,
	})
}

func providerBlocksFromCore(blocks []MessageBlock) []provider.ContentBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]provider.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, provider.ContentBlock{
			Type:       strings.TrimSpace(block.Type),
			Text:       strings.TrimSpace(block.Text),
			ToolCallID: strings.TrimSpace(block.ToolCallID),
			ToolName:   strings.TrimSpace(block.ToolName),
			Arguments:  cloneMap(block.Arguments),
		})
	}
	return out
}

func extractToolCalls(blocks []MessageBlock) []provider.ToolCall {
	out := make([]provider.ToolCall, 0, len(blocks))
	for _, block := range blocks {
		if block.Type != BlockTypeToolCall {
			continue
		}
		id := strings.TrimSpace(block.ToolCallID)
		name := strings.TrimSpace(block.ToolName)
		if id == "" || name == "" {
			continue
		}
		out = append(out, provider.ToolCall{
			ID:        id,
			Name:      name,
			Arguments: cloneMap(block.Arguments),
		})
	}
	return out
}

func renderBlocksAsText(blocks []MessageBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	lines := make([]string, 0, len(blocks))
	for _, block := range blocks {
		blockType := strings.TrimSpace(block.Type)
		switch blockType {
		case BlockTypeText, BlockTypeToolResult, BlockTypeThinking:
			if text := strings.TrimSpace(block.Text); text != "" {
				lines = append(lines, text)
			}
		case BlockTypeToolCall:
			name := strings.TrimSpace(block.ToolName)
			id := strings.TrimSpace(block.ToolCallID)
			if name == "" && id == "" {
				continue
			}
			args := "{}"
			if len(block.Arguments) > 0 {
				if b, err := json.Marshal(block.Arguments); err == nil {
					args = string(b)
				}
			}
			lines = append(lines, fmt.Sprintf("tool_call[%s]: %s %s", id, name, args))
		default:
			if text := strings.TrimSpace(block.Text); text != "" {
				lines = append(lines, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}
