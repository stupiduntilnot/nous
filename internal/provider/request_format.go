package provider

import "strings"

func RenderMessages(messages []Message) string {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		lines = append(lines, role+": "+content)
	}
	return strings.Join(lines, "\n")
}

func ResolvePrompt(req Request) string {
	if len(req.Messages) > 0 {
		if rendered := RenderMessages(req.Messages); rendered != "" {
			return rendered
		}
	}
	if strings.TrimSpace(req.Prompt) != "" {
		return req.Prompt
	}
	if len(req.ToolResults) == 0 {
		return ""
	}
	return "Tool results:\n" + strings.Join(req.ToolResults, "\n")
}
