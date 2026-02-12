package core

import (
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

type Message struct {
	Role MessageRole
	Text string
}

func appendMessage(messages []Message, role MessageRole, text string) []Message {
	text = strings.TrimSpace(text)
	if text == "" {
		return messages
	}
	return append(messages, Message{Role: role, Text: text})
}

func providerMessagesFromCore(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "" || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		out = append(out, provider.Message{Role: string(msg.Role), Content: strings.TrimSpace(msg.Text)})
	}
	return out
}

func defaultConvertToLLMMessages(messages []Message) []provider.Message {
	out := make([]provider.Message, 0, len(messages))
	for _, msg := range messages {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		switch msg.Role {
		case RoleUser, RoleAssistant, RoleToolResult:
			out = append(out, provider.Message{Role: string(msg.Role), Content: text})
		}
	}
	return out
}

func cloneMessages(messages []Message) []Message {
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}
