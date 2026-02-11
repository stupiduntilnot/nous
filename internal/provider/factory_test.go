package provider

import "testing"

func TestBuildMockProvider(t *testing.T) {
	p, err := Build("mock", "", "")
	if err != nil {
		t.Fatalf("build mock failed: %v", err)
	}
	if _, ok := p.(*MockAdapter); !ok {
		t.Fatalf("expected *MockAdapter, got %T", p)
	}
}

func TestBuildUnknownProvider(t *testing.T) {
	if _, err := Build("unknown", "", ""); err == nil {
		t.Fatalf("expected unknown provider to fail")
	}
}

func TestBuildOpenAIRequiresKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := Build("openai", "gpt-test", "http://localhost"); err == nil {
		t.Fatalf("expected openai build without key to fail")
	}
}

func TestBuildGeminiRequiresKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	if _, err := Build("gemini", "gemini-test", "http://localhost"); err == nil {
		t.Fatalf("expected gemini build without key to fail")
	}
}
