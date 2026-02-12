package provider

import "testing"

func TestResolvePromptPrefersStructuredMessages(t *testing.T) {
	got := ResolvePrompt(Request{
		Prompt: "legacy prompt",
		Messages: []Message{
			{Role: "user", Content: "hello"},
			{Role: "tool_result", Content: "first => ok"},
		},
	})
	want := "user: hello\ntool_result: first => ok"
	if got != want {
		t.Fatalf("unexpected rendered prompt\nwant=%q\ngot=%q", want, got)
	}
}

func TestResolvePromptFallsBackToLegacyPrompt(t *testing.T) {
	got := ResolvePrompt(Request{Prompt: "legacy prompt"})
	if got != "legacy prompt" {
		t.Fatalf("unexpected prompt fallback: %q", got)
	}
}

func TestResolvePromptFallsBackToToolResults(t *testing.T) {
	got := ResolvePrompt(Request{
		ToolResults: []string{"first => ok", "second => ok"},
	})
	want := "Tool results:\nfirst => ok\nsecond => ok"
	if got != want {
		t.Fatalf("unexpected tool results fallback\nwant=%q\ngot=%q", want, got)
	}
}
