package protocol

import (
	"encoding/json"
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

	components := doc["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	cmd := schemas["CommandEnvelope"].(map[string]any)
	allOf := cmd["allOf"].([]any)
	props := allOf[1].(map[string]any)["properties"].(map[string]any)
	types := props["type"].(map[string]any)["enum"].([]any)

	want := []string{"ping", "prompt", "steer", "follow_up", "abort", "set_active_tools", "new_session", "switch_session"}
	got := make([]string, 0, len(types))
	for _, v := range types {
		got = append(got, v.(string))
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Fatalf("command enum missing %q", w)
		}
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
