package provider

import "testing"

func TestResolvePromptPrefersStructuredMessages(t *testing.T) {
	got := RenderMessages([]Message{
		{Role: "user", Content: "hello"},
		{Role: "tool_result", Content: "first => ok"},
	})
	want := "user: hello\ntool_result: first => ok"
	if got != want {
		t.Fatalf("unexpected rendered prompt\nwant=%q\ngot=%q", want, got)
	}
}

func TestRenderMessagesSkipsEmptyFields(t *testing.T) {
	got := RenderMessages([]Message{
		{Role: "", Content: "x"},
		{Role: "user", Content: ""},
		{Role: "user", Content: "ok"},
	})
	want := "user: ok"
	if got != want {
		t.Fatalf("unexpected rendered messages\nwant=%q\ngot=%q", want, got)
	}
}
