package main

import (
	"testing"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		in      string
		wantCmd string
		wantErr bool
		wantQ   bool
	}{
		{in: "ping", wantCmd: "ping"},
		{in: "prompt hello", wantCmd: "prompt"},
		{in: "prompt_async hello", wantCmd: "prompt"},
		{in: "steer focus", wantCmd: "steer"},
		{in: "follow_up next", wantCmd: "follow_up"},
		{in: "abort", wantCmd: "abort"},
		{in: "new", wantCmd: "new_session"},
		{in: "switch sess-1", wantCmd: "switch_session"},
		{in: "branch sess-1", wantCmd: "branch_session"},
		{in: "set_active_tools ", wantCmd: "set_active_tools"},
		{in: "set_active_tools tool_a tool_b", wantCmd: "set_active_tools"},
		{in: "ext hello", wantCmd: "extension_command"},
		{in: "ext hello {\"x\":1}", wantCmd: "extension_command"},
		{in: "quit", wantQ: true},
		{in: "prompt ", wantErr: true},
	}

	for _, tc := range tests {
		cmd, _, quit, err := parseInput(tc.in)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for %q", tc.in)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for %q: %v", tc.in, err)
		}
		if quit != tc.wantQ {
			t.Fatalf("unexpected quit flag for %q: got=%v want=%v", tc.in, quit, tc.wantQ)
		}
		if cmd != tc.wantCmd {
			t.Fatalf("unexpected cmd for %q: got=%q want=%q", tc.in, cmd, tc.wantCmd)
		}
	}
}

func TestParseInputExtPayload(t *testing.T) {
	cmd, payload, _, err := parseInput("ext hello {\"x\":1}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "extension_command" {
		t.Fatalf("unexpected cmd: %q", cmd)
	}
	raw, _ := payload["payload"].(map[string]any)
	if got, _ := raw["x"].(float64); got != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestParseInputExtInvalidPayload(t *testing.T) {
	if _, _, _, err := parseInput("ext hello {oops"); err == nil {
		t.Fatalf("expected invalid json payload error")
	}
}

func TestParseInputPromptAsyncPayload(t *testing.T) {
	_, payload, _, err := parseInput("prompt_async hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wait, _ := payload["wait"].(bool)
	if wait {
		t.Fatalf("expected wait=false for prompt_async")
	}
}

func TestParseInputSetActiveToolsClear(t *testing.T) {
	_, payload, _, err := parseInput("set_active_tools ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("missing tools payload: %+v", payload)
	}
	if len(tools) != 0 {
		t.Fatalf("expected empty tool list, got: %+v", tools)
	}
}
