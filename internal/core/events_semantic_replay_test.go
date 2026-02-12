package core

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRuntimeEventSequenceMatchesProtocolExample(t *testing.T) {
	want := readEventTypesFromNDJSON(t, filepath.FromSlash("../../docs/example-protocol-events-runtime-tool-sequence.ndjson"))

	e := NewEngine(NewRuntime(), scriptedProvider{})
	e.SetTools([]Tool{
		ToolFunc{ToolName: "first", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "first-ok", nil
		}},
		ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "second-ok", nil
		}},
	})

	got := make([]string, 0, len(want))
	unsub := e.Subscribe(func(ev Event) {
		got = append(got, string(ev.Type))
	})
	defer unsub()

	if _, err := e.Prompt(context.Background(), "run-semantic-replay", "go"); err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	if !slices.Equal(got, want) {
		t.Fatalf("event sequence drift\nwant=%v\ngot=%v", want, got)
	}
}

func readEventTypesFromNDJSON(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open ndjson failed: %v", err)
	}
	defer f.Close()

	out := make([]string, 0, 16)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var env struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &env); err != nil {
			t.Fatalf("invalid ndjson line: %v", err)
		}
		if env.Type == "" {
			t.Fatalf("missing event type in ndjson line")
		}
		out = append(out, env.Type)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan ndjson failed: %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("ndjson event example is empty")
	}
	return out
}
