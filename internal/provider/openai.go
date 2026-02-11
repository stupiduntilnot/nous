package provider

import (
	"context"
	"fmt"
)

type OpenAIAdapter struct {
	impl *openAICompatAdapter
}

func NewOpenAIAdapter(apiKey, model, baseURL string) (*OpenAIAdapter, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("missing_openai_api_key")
	}
	if model == "" {
		return nil, fmt.Errorf("missing_openai_model")
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	impl, err := newOpenAICompatAdapter(apiKey, model, baseURL, false)
	if err != nil {
		return nil, err
	}
	return &OpenAIAdapter{impl: impl}, nil
}

func (a *OpenAIAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	return a.impl.Stream(ctx, req)
}
