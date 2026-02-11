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

type OpenAIAdapter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewOpenAIAdapter(apiKey, model, baseURL string) (*OpenAIAdapter, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing_openai_api_key")
	}
	if model == "" {
		return nil, fmt.Errorf("missing_openai_model")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIAdapter{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (a *OpenAIAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	out := make(chan Event, 4)
	go func() {
		defer close(out)
		out <- Event{Type: EventStart}

		payload := map[string]any{
			"model": a.model,
			"messages": []map[string]string{
				{"role": "user", "content": req.Prompt},
			},
		}
		b, err := json.Marshal(payload)
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(b))
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
				Message struct {
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
		out <- Event{Type: EventDone}
	}()
	return out
}
