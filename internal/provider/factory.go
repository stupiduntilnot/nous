package provider

import (
	"fmt"
	"os"
)

func Build(name, model, baseURL string) (Adapter, error) {
	switch name {
	case "", "mock":
		return NewMockAdapter(), nil
	case "openai":
		return NewOpenAIAdapter(os.Getenv("OPENAI_API_KEY"), model, baseURL)
	case "gemini":
		return NewGeminiAdapter(os.Getenv("GEMINI_API_KEY"), model, baseURL)
	default:
		return nil, fmt.Errorf("unknown_provider: %s", name)
	}
}
