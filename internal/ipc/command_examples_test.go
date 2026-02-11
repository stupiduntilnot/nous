package ipc

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/extension"
	"oh-my-agent/internal/provider"
	"oh-my-agent/internal/protocol"
	"oh-my-agent/internal/session"
)

func TestDispatchAcceptsProtocolCommandExamplePayloadShapes(t *testing.T) {
	base := testWorkDir(t)
	srv := NewServer(filepath.Join(base, "core.sock"))

	mgr, err := session.NewManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("new session manager failed: %v", err)
	}
	srv.SetSessionManager(mgr)

	e := core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	e.SetExtensionManager(extension.NewManager())
	srv.SetEngine(e, core.NewCommandLoop(e))

	sessionID, err := mgr.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	f, err := os.Open(filepath.FromSlash("../../docs/protocol/examples/commands.ndjson"))
	if err != nil {
		t.Fatalf("open command examples failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		env, err := protocol.DecodeCommand(scanner.Bytes())
		if err != nil {
			t.Fatalf("invalid command fixture line %d: %v", line, err)
		}

		switch env.Type {
		case string(protocol.CmdSwitchSession), string(protocol.CmdBranchSession):
			env.Payload["session_id"] = sessionID
		}

		resp := srv.dispatch(env)
		if resp.ID != env.ID {
			t.Fatalf("line %d response id mismatch: got=%q want=%q", line, resp.ID, env.ID)
		}
		if resp.Error != nil {
			if resp.Error.Code == "invalid_payload" || resp.Error.Code == "not_implemented" {
				t.Fatalf("line %d command %s unexpectedly rejected for payload shape: %+v", line, env.Type, resp.Error)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan command fixtures failed: %v", err)
	}
}
