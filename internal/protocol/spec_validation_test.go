package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestProtocolSchemaValidation(t *testing.T) {
	b, err := os.ReadFile("../../docs/protocol/openapi-like.json")
	if err != nil {
		t.Fatalf("failed to read openapi-like spec: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("invalid json spec: %v", err)
	}

	if doc["openapi"] != "3.1.0" {
		t.Fatalf("unexpected openapi version: %#v", doc["openapi"])
	}

	xTransport, ok := doc["x-transport"].(map[string]any)
	if !ok {
		t.Fatalf("x-transport is missing or invalid")
	}
	if xTransport["type"] != "uds" || xTransport["framing"] != "ndjson" {
		t.Fatalf("x-transport must declare uds + ndjson, got: %#v", xTransport)
	}
	reqs, ok := doc["x-command-payload-requirements"].(map[string]any)
	if !ok {
		t.Fatalf("x-command-payload-requirements is missing or invalid")
	}
	assertRequiredField(t, reqs, "prompt", "text")
	assertRequiredField(t, reqs, "steer", "text")
	assertRequiredField(t, reqs, "follow_up", "text")
	assertRequiredField(t, reqs, "set_active_tools", "tools")
	assertRequiredField(t, reqs, "switch_session", "session_id")
	assertRequiredField(t, reqs, "branch_session", "session_id")
	assertRequiredField(t, reqs, "extension_command", "name")
	assertNotRequiredField(t, reqs, "branch_session", "parent_id")
	respReqs, ok := doc["x-response-payload-requirements"].(map[string]any)
	if !ok {
		t.Fatalf("x-response-payload-requirements is missing or invalid")
	}
	assertRequiredField(t, respReqs, "pong", "message")
	assertRequiredField(t, respReqs, "accepted:prompt", "command")
	assertRequiredField(t, respReqs, "accepted:prompt", "session_id")
	assertRequiredField(t, respReqs, "result", "output")
	assertRequiredField(t, respReqs, "result", "events")
	assertRequiredField(t, respReqs, "result", "session_id")
	assertRequiredField(t, respReqs, "session", "session_id")

	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	cmd := schemas["CommandEnvelope"].(map[string]any)
	allOf := cmd["allOf"].([]any)
	props := allOf[1].(map[string]any)["properties"].(map[string]any)
	types := props["type"].(map[string]any)["enum"].([]any)

	got := make([]string, 0, len(types))
	for _, v := range types {
		got = append(got, v.(string))
	}

	want := expectedCommands()
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Fatalf("command enum missing %q", w)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			t.Fatalf("command enum has unknown command %q", g)
		}
	}

	ev := schemas["EventEnvelope"].(map[string]any)
	evAllOf := ev["allOf"].([]any)
	evProps := evAllOf[1].(map[string]any)["properties"].(map[string]any)
	evTypes := evProps["type"].(map[string]any)["enum"].([]any)
	gotEvents := make([]string, 0, len(evTypes))
	for _, v := range evTypes {
		gotEvents = append(gotEvents, v.(string))
	}
	wantEvents := expectedEvents()
	for _, w := range wantEvents {
		if !slices.Contains(gotEvents, w) {
			t.Fatalf("event enum missing %q", w)
		}
	}
	for _, g := range gotEvents {
		if !slices.Contains(wantEvents, g) {
			t.Fatalf("event enum has unknown event %q", g)
		}
	}

	resp := schemas["ResponseEnvelope"].(map[string]any)
	respAllOf := resp["allOf"].([]any)
	respProps := respAllOf[1].(map[string]any)["properties"].(map[string]any)
	errObj := respProps["error"].(map[string]any)
	errProps := errObj["properties"].(map[string]any)
	if _, ok := errProps["cause"]; !ok {
		t.Fatalf("response error schema missing optional cause field")
	}
}

func TestPiMonoSemanticCompatibility(t *testing.T) {
	b, err := os.ReadFile("../../docs/protocol/pi-mono-semantic-matrix.md")
	if err != nil {
		t.Fatalf("failed to read semantic matrix: %v", err)
	}

	content := string(b)
	checks := []string{
		"`prompt`",
		"`steer`",
		"`follow_up`",
		"`abort`",
		"`branch_session`",
		"agent_start/end",
		"turn_start/end",
		"message_start/update/end",
		"tool_execution_start/update/end",
	}
	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Fatalf("semantic matrix missing %q", c)
		}
	}
}

func expectedCommands() []string {
	out := make([]string, 0, len(validCommands))
	for cmd := range validCommands {
		out = append(out, fmt.Sprintf("%s", cmd))
	}
	slices.Sort(out)
	return out
}

func expectedEvents() []string {
	out := make([]string, 0, len(validEvents))
	for ev := range validEvents {
		out = append(out, fmt.Sprintf("%s", ev))
	}
	slices.Sort(out)
	return out
}

func assertRequiredField(t *testing.T, reqs map[string]any, cmd, field string) {
	t.Helper()
	raw, ok := reqs[cmd].([]any)
	if !ok {
		t.Fatalf("payload requirements missing command %q", cmd)
	}
	fields := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		fields = append(fields, s)
	}
	if !slices.Contains(fields, field) {
		t.Fatalf("payload requirements for %q missing field %q", cmd, field)
	}
}

func assertNotRequiredField(t *testing.T, reqs map[string]any, cmd, field string) {
	t.Helper()
	raw, ok := reqs[cmd].([]any)
	if !ok {
		t.Fatalf("payload requirements missing command %q", cmd)
	}
	fields := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		fields = append(fields, s)
	}
	if slices.Contains(fields, field) {
		t.Fatalf("payload requirements for %q should not contain %q", cmd, field)
	}
}
