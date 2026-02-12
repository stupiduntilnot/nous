package main

import (
	"errors"
	"strings"
	"testing"

	"nous/internal/protocol"
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
		{args: []string{"prompt_stream", "hello"}, wantCmd: "__prompt_stream__"},
		{args: []string{"trace", "run-1"}, wantCmd: "__trace__"},
		{args: []string{"steer", "focus"}, wantCmd: "steer"},
		{args: []string{"follow_up", "next"}, wantCmd: "follow_up"},
		{args: []string{"abort"}, wantCmd: "abort"},
		{args: []string{"new"}, wantCmd: "new_session"},
		{args: []string{"switch", "sess-1"}, wantCmd: "switch_session"},
		{args: []string{"branch", "sess-1"}, wantCmd: "branch_session"},
		{args: []string{"set_active_tools"}, wantCmd: "set_active_tools"},
		{args: []string{"set_active_tools", "a", "b"}, wantCmd: "set_active_tools"},
		{args: []string{"set_steering_mode", "all"}, wantCmd: "set_steering_mode"},
		{args: []string{"set_follow_up_mode", "one-at-a-time"}, wantCmd: "set_follow_up_mode"},
		{args: []string{"get_state"}, wantCmd: "get_state"},
		{args: []string{"get_messages"}, wantCmd: "get_messages"},
		{args: []string{"get_messages", "sess-1"}, wantCmd: "get_messages"},
		{args: []string{"ext", "hello"}, wantCmd: "extension_command"},
		{args: []string{"ext", "hello", "{\"x\":1}"}, wantCmd: "extension_command"},
		{args: []string{"prompt"}, wantErr: true},
		{args: []string{"prompt_async"}, wantErr: true},
		{args: []string{"prompt_stream"}, wantErr: true},
		{args: []string{"trace"}, wantErr: true},
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

func TestParseArgsPromptAsyncSetsWaitFalse(t *testing.T) {
	_, payload, err := parseArgs([]string{"prompt_async", "hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := payload["text"].(string); got != "hello world" {
		t.Fatalf("unexpected prompt_async text payload: %+v", payload)
	}
	wait, ok := payload["wait"].(bool)
	if !ok || wait {
		t.Fatalf("prompt_async should set wait=false payload: %+v", payload)
	}
}

func TestParseArgsPromptStreamUsesSpecialCommand(t *testing.T) {
	cmd, payload, err := parseArgs([]string{"prompt_stream", "hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "__prompt_stream__" {
		t.Fatalf("unexpected command: %q", cmd)
	}
	if got, _ := payload["text"].(string); got != "hello world" {
		t.Fatalf("unexpected prompt_stream text payload: %+v", payload)
	}
}

func TestParseArgsTraceRequiresRunID(t *testing.T) {
	cmd, payload, err := parseArgs([]string{"trace", "run-42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd != "__trace__" {
		t.Fatalf("unexpected command for trace: %q", cmd)
	}
	if got, _ := payload["run_id"].(string); got != "run-42" {
		t.Fatalf("trace payload missing run_id: %+v", payload)
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

func TestDescribeRequestError(t *testing.T) {
	got := describeRequestError("prompt", errors.New("read response: read unix ->/tmp/nous-core.sock: i/o timeout"))
	if got == "" || got == "request failed" {
		t.Fatalf("unexpected described timeout error: %q", got)
	}
	if want := "prompt_async"; !strings.Contains(got, want) {
		t.Fatalf("expected timeout hint to mention %q, got: %q", want, got)
	}

	got = describeRequestError("ping", errors.New("dial uds: no such file"))
	if got != "dial uds: no such file" {
		t.Fatalf("unexpected generic described error: %q", got)
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

func TestParseArgsBranchUsesSessionID(t *testing.T) {
	_, payload, err := parseArgs([]string{"branch", "sess-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := payload["session_id"].(string); got != "sess-1" {
		t.Fatalf("expected session_id payload, got: %+v", payload)
	}
	if _, ok := payload["parent_id"]; ok {
		t.Fatalf("branch payload should not contain parent_id: %+v", payload)
	}
}

func TestParseArgsGetMessagesOptionalSessionID(t *testing.T) {
	_, payload, err := parseArgs([]string{"get_messages", "sess-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, _ := payload["session_id"].(string); got != "sess-1" {
		t.Fatalf("expected session_id payload, got: %+v", payload)
	}
}
