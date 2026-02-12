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

func TestParseInputPromptIsNonBlocking(t *testing.T) {
	_, payload, _, err := parseInput("prompt hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wait, ok := payload["wait"].(bool)
	if !ok {
		t.Fatalf("missing wait flag in payload: %+v", payload)
	}
	if wait {
		t.Fatalf("prompt should default to non-blocking wait=false payload: %+v", payload)
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

func TestRenderResultRendersMessageUpdatesAsAssistant(t *testing.T) {
	payload := map[string]any{
		"events": []any{
			map[string]any{"type": "message_update", "delta": "hello chunk"},
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
	if !strings.Contains(out, "assistant: hello chunk") {
		t.Fatalf("expected streamed assistant chunk in output, got:\n%s", out)
	}
}

func TestRenderResultRendersProgressFieldsForRunTurnTool(t *testing.T) {
	payload := map[string]any{
		"events": []any{
			map[string]any{"type": "agent_start", "run_id": "run-1"},
			map[string]any{"type": "turn_start", "run_id": "run-1", "turn_id": "1"},
			map[string]any{"type": "tool_execution_start", "run_id": "run-1", "turn_id": "1", "tool_name": "bash"},
			map[string]any{"type": "tool_execution_update", "run_id": "run-1", "turn_id": "1", "tool_name": "bash", "delta": "running"},
			map[string]any{"type": "tool_execution_end", "run_id": "run-1", "turn_id": "1", "tool_name": "bash"},
			map[string]any{"type": "turn_end", "run_id": "run-1", "turn_id": "1"},
			map[string]any{"type": "agent_end", "run_id": "run-1"},
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
		"status: agent_start run=run-1",
		"status: turn_start run=run-1 turn=1",
		"tool: tool_execution_start name=bash run=run-1 turn=1",
		"tool: tool_execution_update name=bash run=run-1 turn=1 delta=running",
		"tool: tool_execution_end name=bash run=run-1 turn=1",
		"status: agent_end run=run-1",
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

func TestRunQueueStateLifecycle(t *testing.T) {
	q := &runQueueState{}
	q.Activate("run-1")

	if snap, changed := q.MarkAccepted("steer", "run-1"); !changed || snap.PendingSteer != 1 || snap.PendingFollowUp != 0 {
		t.Fatalf("unexpected steer queue state: changed=%v snap=%+v", changed, snap)
	}
	if snap, changed := q.MarkAccepted("follow_up", "run-1"); !changed || snap.PendingSteer != 1 || snap.PendingFollowUp != 1 {
		t.Fatalf("unexpected follow_up queue state: changed=%v snap=%+v", changed, snap)
	}

	if snap, changed := q.MarkEvent("turn_start", "run-1"); !changed || snap.PendingSteer != 0 || snap.PendingFollowUp != 1 {
		t.Fatalf("unexpected queue consumption after first turn: changed=%v snap=%+v", changed, snap)
	}
	if snap, changed := q.MarkEvent("turn_start", "run-1"); !changed || snap.PendingSteer != 0 || snap.PendingFollowUp != 0 {
		t.Fatalf("unexpected queue consumption after second turn: changed=%v snap=%+v", changed, snap)
	}
	if snap, changed := q.MarkEvent("agent_end", "run-1"); !changed || snap.ActiveRunID != "" {
		t.Fatalf("queue should reset at agent_end: changed=%v snap=%+v", changed, snap)
	}
}

func TestRunQueueStateIgnoresMismatchedRun(t *testing.T) {
	q := &runQueueState{}
	q.Activate("run-1")

	if _, changed := q.MarkAccepted("steer", "run-other"); changed {
		t.Fatalf("expected mismatched run accepted command to be ignored")
	}
	if _, changed := q.MarkEvent("turn_start", "run-other"); changed {
		t.Fatalf("expected mismatched run event to be ignored")
	}
}
