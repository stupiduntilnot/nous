package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type openAICompatAdapter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func newOpenAICompatAdapter(apiKey, model, baseURL string) (*openAICompatAdapter, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing_api_key")
	}
	if model == "" {
		return nil, fmt.Errorf("missing_model")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("missing_api_base")
	}
	return &openAICompatAdapter{
		apiKey:  apiKey,
		model:   model,
		baseURL: normalizeAPIBase(baseURL),
		client:  &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func normalizeAPIBase(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}

func (a *openAICompatAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	out := make(chan Event, 4)
	go func() {
		defer close(out)
		out <- Event{Type: EventStart}

		messages := buildOpenAIMessages(req)
		payload := map[string]any{
			"model":    a.model,
			"messages": messages,
		}
		if len(req.ActiveTools) > 0 {
			payload["tools"] = buildOpenAITools(req.ActiveTools)
		}
		b, err := json.Marshal(payload)
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(b))
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
		httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := a.client.Do(httpReq)
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
		if resp.StatusCode >= 400 {
			out <- Event{Type: EventError, Err: fmt.Errorf("openai_http_%d: %s", resp.StatusCode, string(body))}
			return
		}

		var decoded struct {
			Choices []struct {
				FinishReason string `json:"finish_reason"`
				Message      struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
		if len(decoded.Choices) == 0 {
			out <- Event{Type: EventError, Err: fmt.Errorf("openai_empty_choices")}
			return
		}
		msg := decoded.Choices[0].Message
		finishReason := decoded.Choices[0].FinishReason

		for _, tc := range msg.ToolCalls {
			args := map[string]any{}
			if strings.TrimSpace(tc.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					out <- Event{Type: EventError, Err: fmt.Errorf("openai_bad_tool_args: %w", err)}
					return
				}
			}
			out <- Event{
				Type: EventToolCall,
				ToolCall: ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: args,
				},
			}
		}

		if msg.Content != "" {
			out <- Event{Type: EventTextDelta, Delta: msg.Content}
		}
		if finishReason == "tool_calls" {
			out <- Event{Type: EventAwaitNext}
		}
		out <- Event{Type: EventDone}
	}()
	return out
}

func buildOpenAIMessages(req Request) []map[string]string {
	if len(req.Messages) > 0 {
		out := make([]map[string]string, 0, len(req.Messages))
		for _, msg := range req.Messages {
			role := strings.TrimSpace(msg.Role)
			content := strings.TrimSpace(msg.Content)
			if role == "" || content == "" {
				continue
			}
			switch role {
			case "assistant", "system", "user":
			case "tool_result":
				role = "user"
				content = "Tool result:\n" + content
			default:
				role = "user"
			}
			out = append(out, map[string]string{
				"role":    role,
				"content": content,
			})
		}
		if len(out) > 0 {
			return out
		}
	}

	out := []map[string]string{
		{"role": "user", "content": req.Prompt},
	}
	if len(req.ToolResults) > 0 {
		out = append(out, map[string]string{
			"role":    "user",
			"content": "Tool results:\n" + strings.Join(req.ToolResults, "\n"),
		})
	}
	return out
}

func buildOpenAITools(names []string) []map[string]any {
	tools := make([]map[string]any, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		description := "registered runtime tool"
		parameters := map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": true,
		}
		switch name {
		case "read":
			description = "Read file contents by path. Supports optional offset and limit."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file to read (relative or absolute)",
					},
					"offset": map[string]any{
						"type":        "number",
						"description": "Optional line offset (0-based)",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Optional max number of lines to read",
					},
				},
				"required":             []string{"path"},
				"additionalProperties": true,
			}
		case "ls":
			description = "List directory contents."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Directory path to list. Defaults to current directory.",
					},
				},
				"additionalProperties": true,
			}
		case "find":
			description = "Find files under a path using substring match."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Root directory to search. Defaults to current directory.",
					},
					"query": map[string]any{
						"type":        "string",
						"description": "Substring to match in relative file paths.",
					},
					"max_results": map[string]any{
						"type":        "number",
						"description": "Maximum number of matches to return.",
					},
					"max_depth": map[string]any{
						"type":        "number",
						"description": "Maximum depth to walk. -1 means unlimited.",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": true,
			}
		case "grep":
			description = "Search file contents with a pattern and return matching lines."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{
						"type":        "string",
						"description": "Pattern to search (regex).",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "File or directory path to search. Defaults to current directory.",
					},
					"ignore_case": map[string]any{
						"type":        "boolean",
						"description": "Case-insensitive search.",
					},
					"limit": map[string]any{
						"type":        "number",
						"description": "Maximum number of matching lines to return.",
					},
				},
				"required":             []string{"pattern"},
				"additionalProperties": true,
			}
		case "write":
			description = "Write text content into a file path."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to the file to write (relative or absolute).",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Text content to write.",
					},
				},
				"required":             []string{"path", "content"},
				"additionalProperties": true,
			}
		case "edit":
			description = "Edit a file by replacing exact oldText with newText."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{
						"type":        "string",
						"description": "Path to file to edit.",
					},
					"oldText": map[string]any{
						"type":        "string",
						"description": "Exact old text to replace.",
					},
					"newText": map[string]any{
						"type":        "string",
						"description": "Replacement text.",
					},
				},
				"required":             []string{"path", "oldText", "newText"},
				"additionalProperties": true,
			}
		case "bash":
			description = "Execute a shell command in current working directory."
			parameters = map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Shell command to execute.",
					},
					"timeout": map[string]any{
						"type":        "number",
						"description": "Optional timeout in seconds.",
					},
				},
				"required":             []string{"command"},
				"additionalProperties": true,
			}
		}
		tools = append(tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": description,
				"parameters":  parameters,
			},
		})
	}
	return tools
}
