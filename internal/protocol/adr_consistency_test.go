package protocol

import (
	"os"
	"strings"
	"testing"
)

func TestProtocolADRExistsAndContainsDecisions(t *testing.T) {
	b, err := os.ReadFile("../../docs/adr/0001-transport-and-wire-protocol.md")
	if err != nil {
		t.Fatalf("failed to read ADR: %v", err)
	}

	content := string(b)
	checks := []string{
		"Transport is Unix Domain Socket (UDS)",
		"Wire protocol framing is NDJSON",
		"stdio transport is explicitly out of scope",
	}

	for _, c := range checks {
		if !strings.Contains(content, c) {
			t.Fatalf("ADR missing expected text: %q", c)
		}
	}
}
