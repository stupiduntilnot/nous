package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nous/internal/provider"
)

type textOnlyProvider struct{}

func (textOnlyProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventStart}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: "hello"}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestEventSequenceGoldenPromptBasic(t *testing.T) {
	e := NewEngine(NewRuntime(), textOnlyProvider{})
	types := make([]string, 0, 8)
	unsub := e.Subscribe(func(ev Event) {
		types = append(types, string(ev.Type))
	})
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-golden-basic", "hi"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	assertEventGolden(t, "prompt_basic.events", strings.Join(types, "\n")+"\n")
}

func TestEventSequenceGoldenToolCall(t *testing.T) {
	e := NewEngine(NewRuntime(), scriptedProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "second-ok", nil
		}},
	})
	types := make([]string, 0, 16)
	unsub := e.Subscribe(func(ev Event) {
		types = append(types, string(ev.Type))
	})
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-golden-tools", "go"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	assertEventGolden(t, "prompt_tool.events", strings.Join(types, "\n")+"\n")
}

func assertEventGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s failed: %v", name, err)
	}
	want := string(wantBytes)
	if got != want {
		t.Fatalf("golden mismatch for %s\nwant:\n%s\ngot:\n%s", name, want, got)
	}
}
