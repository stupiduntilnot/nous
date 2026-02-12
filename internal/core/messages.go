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

func renderPromptFromMessages(messages []Message) string {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "" || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		lines = append(lines, string(msg.Role)+": "+strings.TrimSpace(msg.Text))
	}
	return strings.Join(lines, "\n")
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
