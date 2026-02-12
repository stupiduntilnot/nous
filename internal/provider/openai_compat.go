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

		messages := buildOpenAIMessages(req.Messages)
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
		policy := defaultRetryPolicy()
		var lastErr error
		for attempt := 1; attempt <= policy.maxAttempts; attempt++ {
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/chat/completions", bytes.NewReader(b))
			if err != nil {
				out <- Event{Type: EventError, Err: err}
				return
			}
			httpReq.Header.Set("Authorization", "Bearer "+a.apiKey)
			httpReq.Header.Set("Content-Type", "application/json")

			resp, err := a.client.Do(httpReq)
			if err != nil {
				if ctx.Err() != nil {
					out <- Event{Type: EventError, Err: NewAbortedError("request_aborted", ctx.Err())}
					return
				}
				lastErr = err
				if shouldRetryTransportError(err) && attempt < policy.maxAttempts {
					delay := retryDelayForAttempt(policy, attempt)
					out <- Event{
						Type:    EventWarning,
						Code:    "provider_retry",
						Message: fmt.Sprintf("openai retry attempt %d/%d after transport failure: %v", attempt, policy.maxAttempts, err),
					}
					if waitErr := waitRetry(ctx, delay); waitErr != nil {
						out <- Event{Type: EventError, Err: NewAbortedError("request_aborted", waitErr)}
						return
					}
					continue
				}
				if shouldRetryTransportError(err) {
					out <- Event{Type: EventError, Err: &RetryExhaustedError{Attempts: attempt, LastErr: err}}
					return
				}
				out <- Event{Type: EventError, Err: err}
				return
			}

			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				out <- Event{Type: EventError, Err: readErr}
				return
			}
			if resp.StatusCode >= 400 {
				httpErr := fmt.Errorf("openai_http_%d: %s", resp.StatusCode, string(body))
				lastErr = httpErr
				if shouldRetryHTTPStatus(resp.StatusCode) && attempt < policy.maxAttempts {
					delay := retryDelayForAttempt(policy, attempt)
					out <- Event{
						Type:    EventWarning,
						Code:    "provider_retry",
						Message: fmt.Sprintf("openai retry attempt %d/%d after http %d", attempt, policy.maxAttempts, resp.StatusCode),
					}
					if waitErr := waitRetry(ctx, delay); waitErr != nil {
						out <- Event{Type: EventError, Err: NewAbortedError("request_aborted", waitErr)}
						return
					}
					continue
				}
				if shouldRetryHTTPStatus(resp.StatusCode) {
					out <- Event{Type: EventError, Err: &RetryExhaustedError{Attempts: attempt, LastErr: httpErr}}
					return
				}
				out <- Event{Type: EventError, Err: httpErr}
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
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
					TotalTokens      int `json:"total_tokens"`
				} `json:"usage"`
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
			var usage *Usage
			if decoded.Usage.PromptTokens > 0 || decoded.Usage.CompletionTokens > 0 || decoded.Usage.TotalTokens > 0 {
				usage = &Usage{
					InputTokens:  decoded.Usage.PromptTokens,
					OutputTokens: decoded.Usage.CompletionTokens,
					TotalTokens:  decoded.Usage.TotalTokens,
				}
			}
			out <- Event{
				Type:       EventDone,
				StopReason: mapOpenAIStopReason(finishReason),
				Usage:      usage,
			}
			return
		}
		if lastErr != nil {
			out <- Event{Type: EventError, Err: &RetryExhaustedError{Attempts: policy.maxAttempts, LastErr: lastErr}}
		}
	}()
	return out
}

func mapOpenAIStopReason(reason string) StopReason {
	switch strings.TrimSpace(reason) {
	case "tool_calls":
		return StopReasonToolUse
	case "length":
		return StopReasonLength
	case "stop":
		return StopReasonStop
	case "":
		return StopReasonUnknown
	default:
		return StopReasonUnknown
	}
}

func buildOpenAIMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = strings.TrimSpace(renderProviderBlocksAsText(msg.Blocks))
		}
		if role == "" {
			continue
		}
		switch role {
		case "assistant", "system", "user":
		case "tool_result":
			if strings.TrimSpace(msg.ToolCallID) != "" {
				if content == "" {
					continue
				}
				out = append(out, map[string]any{
					"role":         "tool",
					"tool_call_id": strings.TrimSpace(msg.ToolCallID),
					"content":      content,
				})
				continue
			}
			if content == "" {
				continue
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": "Tool result:\n" + content,
			})
			continue
		default:
			role = "user"
		}
		if role == "assistant" && len(msg.ToolCalls) > 0 {
			assistant := map[string]any{
				"role":       "assistant",
				"tool_calls": openAIToolCalls(msg.ToolCalls),
				"content":    content,
			}
			out = append(out, assistant)
			continue
		}
		if content == "" {
			continue
		}
		out = append(out, map[string]any{
			"role":    role,
			"content": content,
		})
	}
	return out
}

func openAIToolCalls(toolCalls []ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(toolCalls))
	for _, call := range toolCalls {
		id := strings.TrimSpace(call.ID)
		name := strings.TrimSpace(call.Name)
		if id == "" || name == "" {
			continue
		}
		args := "{}"
		if len(call.Arguments) > 0 {
			if b, err := json.Marshal(call.Arguments); err == nil {
				args = string(b)
			}
		}
		out = append(out, map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": args,
			},
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
