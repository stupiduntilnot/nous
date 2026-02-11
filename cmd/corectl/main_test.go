package main

import (
	"testing"

	"oh-my-agent/internal/protocol"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		wantCmd string
		wantErr bool
	}{
		{args: []string{"ping"}, wantCmd: "ping"},
		{args: []string{"prompt", "hello"}, wantCmd: "prompt"},
		{args: []string{"prompt_async", "hello"}, wantCmd: "prompt"},
		{args: []string{"steer", "focus"}, wantCmd: "steer"},
		{args: []string{"follow_up", "next"}, wantCmd: "follow_up"},
		{args: []string{"abort"}, wantCmd: "abort"},
		{args: []string{"new"}, wantCmd: "new_session"},
		{args: []string{"switch", "sess-1"}, wantCmd: "switch_session"},
		{args: []string{"branch", "sess-1"}, wantCmd: "branch_session"},
		{args: []string{"set_active_tools"}, wantCmd: "set_active_tools"},
		{args: []string{"set_active_tools", "a", "b"}, wantCmd: "set_active_tools"},
		{args: []string{"ext", "hello"}, wantCmd: "extension_command"},
		{args: []string{"ext", "hello", "{\"x\":1}"}, wantCmd: "extension_command"},
		{args: []string{"prompt"}, wantErr: true},
	}

	for _, tc := range tests {
		cmd, _, err := parseArgs(tc.args)
		if tc.wantErr && err == nil {
			t.Fatalf("expected error for args=%v", tc.args)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("unexpected error for args=%v: %v", tc.args, err)
		}
		if cmd != tc.wantCmd {
			t.Fatalf("unexpected cmd for args=%v: got=%q want=%q", tc.args, cmd, tc.wantCmd)
		}
	}
}

func TestParseArgsPromptAsyncPayload(t *testing.T) {
	_, payload, err := parseArgs([]string{"prompt_async", "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wait, _ := payload["wait"].(bool)
	if wait {
		t.Fatalf("expected wait=false for prompt_async")
	}
}

func TestFormatError(t *testing.T) {
	got := formatError(&protocol.ErrorBody{Code: "provider_error", Message: "request failed", Cause: "dial timeout"})
	if got != "provider_error: request failed (dial timeout)" {
		t.Fatalf("unexpected formatted error: %q", got)
	}

	got = formatError(&protocol.ErrorBody{Code: "invalid_payload", Message: "text is required"})
	if got != "invalid_payload: text is required" {
		t.Fatalf("unexpected formatted error without cause: %q", got)
	}
}

func TestParseArgsSetActiveToolsClear(t *testing.T) {
	_, payload, err := parseArgs([]string{"set_active_tools"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tools, ok := payload["tools"].([]any)
	if !ok {
		t.Fatalf("tools payload missing: %+v", payload)
	}
	if len(tools) != 0 {
		t.Fatalf("expected empty tools payload, got: %+v", tools)
	}
}
