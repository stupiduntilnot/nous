package provider

import (
	"context"
	"fmt"
)

type OllamaAdapter struct {
	impl *openAICompatAdapter
}

func NewOllamaAdapter(apiKey, model, baseURL string) (*OllamaAdapter, error) {
	if model == "" {
		return nil, fmt.Errorf("missing_ollama_model")
	}
	if apiKey == "" {
		apiKey = "ollama"
	}
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434/v1"
	}
	impl, err := newOpenAICompatAdapter(apiKey, model, baseURL, true)
	if err != nil {
		return nil, err
	}
	return &OllamaAdapter{impl: impl}, nil
}

func (a *OllamaAdapter) Stream(ctx context.Context, req Request) <-chan Event {
	return a.impl.Stream(ctx, req)
}
