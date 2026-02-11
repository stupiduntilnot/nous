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

type GeminiAdapter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewGeminiAdapter(apiKey, model, baseURL string) (*GeminiAdapter, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing_gemini_api_key")
	}
	if model == "" {
		return nil, fmt.Errorf("missing_gemini_model")
	}
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	return &GeminiAdapter{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 60 * time.Second},
	}, nil
}

func (a *GeminiAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	out := make(chan Event, 4)
	go func() {
		defer close(out)
		out <- Event{Type: EventStart}

		payload := map[string]any{
			"contents": []map[string]any{
				{
					"parts": []map[string]string{
						{"text": req.Prompt},
					},
				},
			},
		}
		b, err := json.Marshal(payload)
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}

		url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", a.baseURL, a.model, a.apiKey)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
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
			out <- Event{Type: EventError, Err: fmt.Errorf("gemini_http_%d: %s", resp.StatusCode, string(body))}
			return
		}

		var decoded struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if err := json.Unmarshal(body, &decoded); err != nil {
			out <- Event{Type: EventError, Err: err}
			return
		}
		if len(decoded.Candidates) == 0 {
			out <- Event{Type: EventError, Err: fmt.Errorf("gemini_empty_candidates")}
			return
		}

		var text strings.Builder
		for _, p := range decoded.Candidates[0].Content.Parts {
			text.WriteString(p.Text)
		}
		out <- Event{Type: EventTextDelta, Delta: text.String()}
		out <- Event{Type: EventDone}
	}()
	return out
}
