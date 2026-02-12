package protocol

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProtocolExamplesCommandsNDJSON(t *testing.T) {
	for _, path := range []string{
		"../../docs/example-protocol-commands.ndjson",
		"../../docs/example-protocol-commands-live-run-control.ndjson",
	} {
		f, err := os.Open(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("open commands example failed: %v", err)
		}
		scanner := bufio.NewScanner(f)
		count := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			env, err := DecodeCommand(line)
			if err != nil {
				_ = f.Close()
				t.Fatalf("invalid command example line %d in %s: %v", count+1, path, err)
			}
			if env.ID == "" || env.Type == "" {
				_ = f.Close()
				t.Fatalf("missing required fields on line %d in %s", count+1, path)
			}
			assertCommandPayloadSemantics(t, env, count+1)
			count++
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			t.Fatalf("scan commands example failed for %s: %v", path, err)
		}
		_ = f.Close()
		if count == 0 {
			t.Fatalf("commands example should not be empty: %s", path)
		}
	}
}

func assertCommandPayloadSemantics(t *testing.T, env Envelope, line int) {
	t.Helper()
	switch CommandType(env.Type) {
	case CmdPing, CmdAbort, CmdNewSession, CmdGetState, CmdGetMessages, CmdCompactSession:
		return
	case CmdPrompt, CmdSteer, CmdFollowUp:
		if text, _ := env.Payload["text"].(string); text == "" {
			t.Fatalf("command line %d (%s) requires non-empty payload.text", line, env.Type)
		}
	case CmdSetActiveTools:
		if _, ok := env.Payload["tools"].([]any); !ok {
			t.Fatalf("command line %d (%s) requires payload.tools array", line, env.Type)
		}
	case CmdSetSteeringMode, CmdSetFollowUpMode:
		mode, _ := env.Payload["mode"].(string)
		if mode != "one-at-a-time" && mode != "all" {
			t.Fatalf("command line %d (%s) requires payload.mode one-at-a-time|all", line, env.Type)
		}
	case CmdSwitchSession, CmdBranchSession:
		if sid, _ := env.Payload["session_id"].(string); sid == "" {
			t.Fatalf("command line %d (%s) requires payload.session_id", line, env.Type)
		}
	case CmdExtensionCmd:
		if name, _ := env.Payload["name"].(string); name == "" {
			t.Fatalf("command line %d (%s) requires payload.name", line, env.Type)
		}
	default:
		t.Fatalf("unsupported command in examples: %s", env.Type)
	}
}

func TestProtocolExamplesResponsesNDJSON(t *testing.T) {
	for _, path := range []string{
		"../../docs/example-protocol-responses.ndjson",
		"../../docs/example-protocol-responses-live-run-control.ndjson",
	} {
		f, err := os.Open(filepath.FromSlash(path))
		if err != nil {
			t.Fatalf("open responses example failed: %v", err)
		}
		scanner := bufio.NewScanner(f)
		count := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			var resp ResponseEnvelope
			if err := json.Unmarshal(line, &resp); err != nil {
				_ = f.Close()
				t.Fatalf("invalid response example line %d in %s: %v", count+1, path, err)
			}
			if resp.V == "" || resp.ID == "" || resp.Type == "" {
				_ = f.Close()
				t.Fatalf("response line %d missing envelope fields in %s", count+1, path)
			}
			if !resp.OK {
				if resp.Error == nil || resp.Error.Code == "" || resp.Error.Message == "" {
					_ = f.Close()
					t.Fatalf("response line %d invalid error body in %s", count+1, path)
				}
			}
			assertResponsePayloadSemantics(t, resp, count+1)
			count++
		}
		if err := scanner.Err(); err != nil {
			_ = f.Close()
			t.Fatalf("scan responses example failed for %s: %v", path, err)
		}
		_ = f.Close()
		if count == 0 {
			t.Fatalf("responses example should not be empty: %s", path)
		}
	}
}

func assertResponsePayloadSemantics(t *testing.T, resp ResponseEnvelope, line int) {
	t.Helper()
	if !resp.OK {
		return
	}
	switch resp.Type {
	case "pong":
		return
	case "accepted":
		cmd, _ := resp.Payload["command"].(string)
		if cmd == "" {
			t.Fatalf("response line %d accepted payload requires command", line)
		}
		switch cmd {
		case "prompt", "steer", "follow_up", "abort":
			if runID, _ := resp.Payload["run_id"].(string); runID == "" {
				t.Fatalf("response line %d accepted %s payload requires run_id", line, cmd)
			}
		case "set_steering_mode", "set_follow_up_mode":
			if mode, _ := resp.Payload["mode"].(string); mode == "" {
				t.Fatalf("response line %d accepted %s payload requires mode", line, cmd)
			}
		}
	case "result":
		if _, ok := resp.Payload["output"].(string); !ok {
			t.Fatalf("response line %d result payload requires output", line)
		}
		if _, ok := resp.Payload["events"].([]any); !ok {
			t.Fatalf("response line %d result payload requires events array", line)
		}
		if sid, _ := resp.Payload["session_id"].(string); sid == "" {
			t.Fatalf("response line %d result payload requires session_id", line)
		}
	case "session":
		if sid, _ := resp.Payload["session_id"].(string); sid == "" {
			t.Fatalf("response line %d session payload requires session_id", line)
		}
		if _, ok := resp.Payload["active"].(bool); !ok {
			t.Fatalf("response line %d session payload requires active boolean", line)
		}
	case "state":
		if _, ok := resp.Payload["run_state"].(string); !ok {
			t.Fatalf("response line %d state payload requires run_state", line)
		}
		if _, ok := resp.Payload["run_id"].(string); !ok {
			t.Fatalf("response line %d state payload requires run_id", line)
		}
		if _, ok := resp.Payload["session_id"].(string); !ok {
			t.Fatalf("response line %d state payload requires session_id", line)
		}
		if _, ok := resp.Payload["steering_mode"].(string); !ok {
			t.Fatalf("response line %d state payload requires steering_mode", line)
		}
		if _, ok := resp.Payload["follow_up_mode"].(string); !ok {
			t.Fatalf("response line %d state payload requires follow_up_mode", line)
		}
		if _, ok := resp.Payload["pending_counts"].(map[string]any); !ok {
			t.Fatalf("response line %d state payload requires pending_counts object", line)
		}
	case "messages":
		if _, ok := resp.Payload["session_id"].(string); !ok {
			t.Fatalf("response line %d messages payload requires session_id", line)
		}
		if _, ok := resp.Payload["messages"].([]any); !ok {
			t.Fatalf("response line %d messages payload requires messages array", line)
		}
	case "compaction":
		if sid, _ := resp.Payload["session_id"].(string); sid == "" {
			t.Fatalf("response line %d compaction payload requires session_id", line)
		}
		if summary, _ := resp.Payload["summary"].(string); summary == "" {
			t.Fatalf("response line %d compaction payload requires summary", line)
		}
		if first, _ := resp.Payload["first_kept_entry_id"].(string); first == "" {
			t.Fatalf("response line %d compaction payload requires first_kept_entry_id", line)
		}
	default:
		// keep examples permissive for other success envelopes (session/extension_result)
	}
}

func TestProtocolExamplesEventsNDJSON(t *testing.T) {
	for _, path := range []string{
		"../../docs/example-protocol-events-prompt-tool.ndjson",
		"../../docs/example-protocol-events-runtime-tool-sequence.ndjson",
		"../../docs/example-protocol-events-live-run-control.ndjson",
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
