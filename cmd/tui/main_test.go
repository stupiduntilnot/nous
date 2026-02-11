package main

import (
	"bytes"
	"io"
	"os"
	"strings"
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
		{in: "steer focus", wantCmd: "steer"},
		{in: "follow_up next", wantCmd: "follow_up"},
		{in: "abort", wantCmd: "abort"},
		{in: "new", wantCmd: "new_session"},
		{in: "switch sess-1", wantCmd: "switch_session"},
		{in: "branch sess-1", wantCmd: "branch_session"},
		{in: "set_active_tools", wantCmd: "set_active_tools"},
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

func TestRenderResultRendersStatusWarningErrorEvents(t *testing.T) {
	payload := map[string]any{
		"output": "ok",
		"events": []any{
			map[string]any{"type": "status", "message": "await_next: continue_with_tool_results"},
			map[string]any{"type": "warning", "code": "tool_not_found", "message": "tool_not_found: missing.tool"},
			map[string]any{"type": "error", "code": "provider_error", "message": "provider stream returned error", "cause": "upstream_down"},
		},
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	os.Stdout = w
	renderResult(payload)
	_ = w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout failed: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"assistant: ok",
		"status: await_next: continue_with_tool_results",
		"warning: tool_not_found tool_not_found: missing.tool",
		"error: provider_error provider stream returned error cause=upstream_down",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("render output missing %q in:\n%s", want, out)
		}
	}
}

func TestParseInputBranchUsesSessionID(t *testing.T) {
	_, payload, _, err := parseInput("branch sess-1")
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

func TestExtractSessionID(t *testing.T) {
	if got := extractSessionID(nil); got != "" {
		t.Fatalf("expected empty id from nil payload, got: %q", got)
	}
	if got := extractSessionID(map[string]any{"x": "1"}); got != "" {
		t.Fatalf("expected empty id from payload without session_id, got: %q", got)
	}
	if got := extractSessionID(map[string]any{"session_id": "sess-123"}); got != "sess-123" {
		t.Fatalf("unexpected session id: %q", got)
	}
}
