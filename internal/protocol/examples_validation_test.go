package protocol

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProtocolExamplesCommandsNDJSON(t *testing.T) {
	path := filepath.FromSlash("../../docs/protocol/examples/commands.ndjson")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open commands example failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		env, err := DecodeCommand(line)
		if err != nil {
			t.Fatalf("invalid command example line %d: %v", count+1, err)
		}
		if env.ID == "" || env.Type == "" {
			t.Fatalf("missing required fields on line %d", count+1)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan commands example failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("commands example should not be empty")
	}
}

func TestProtocolExamplesResponsesNDJSON(t *testing.T) {
	path := filepath.FromSlash("../../docs/protocol/examples/responses.ndjson")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open responses example failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		var resp ResponseEnvelope
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("invalid response example line %d: %v", count+1, err)
		}
		if resp.V == "" || resp.ID == "" || resp.Type == "" {
			t.Fatalf("response line %d missing envelope fields", count+1)
		}
		if !resp.OK {
			if resp.Error == nil || resp.Error.Code == "" || resp.Error.Message == "" {
				t.Fatalf("response line %d invalid error body", count+1)
			}
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses example failed: %v", err)
	}
	if count == 0 {
		t.Fatalf("responses example should not be empty")
	}
}

func TestProtocolExamplesEventsNDJSON(t *testing.T) {
	for _, path := range []string{
		"../../docs/protocol/examples/events_prompt_tool.ndjson",
		"../../docs/protocol/examples/events_runtime_tool_sequence.ndjson",
	} {
		f, err := os.Open(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("open events example failed: %v", err)
		}
		scanner := bufio.NewScanner(f)
		count := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			var env Envelope
			if err := json.Unmarshal(line, &env); err != nil {
				_ = f.Close()
				t.Fatalf("invalid event example line %d in %s: %v", count+1, path, err)
			}
			if env.V == "" || env.ID == "" || env.Type == "" {
				_ = f.Close()
				t.Fatalf("event line %d missing envelope fields in %s", count+1, path)
			}
			if err := ValidateEventType(EventType(env.Type)); err != nil {
				_ = f.Close()
				t.Fatalf("event line %d has invalid type %q in %s: %v", count+1, env.Type, path, err)
			}
			count++
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			t.Fatalf("scan events example failed for %s: %v", path, err)
		}
		_ = f.Close()
		if count == 0 {
			t.Fatalf("events example should not be empty: %s", path)
		}
	}
}
