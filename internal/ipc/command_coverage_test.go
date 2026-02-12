package ipc

import (
	"path/filepath"
	"testing"

	"nous/internal/core"
	"nous/internal/extension"
	"nous/internal/provider"
	"nous/internal/protocol"
	"nous/internal/session"
)

func TestDispatchDoesNotReturnNotImplementedForKnownCommands(t *testing.T) {
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

	parentID, err := mgr.NewSession()
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	cases := []protocol.Envelope{
		{ID: "c-ping", Type: string(protocol.CmdPing), Payload: map[string]any{}},
		{ID: "c-prompt", Type: string(protocol.CmdPrompt), Payload: map[string]any{"text": "hello", "wait": true}},
		{ID: "c-steer", Type: string(protocol.CmdSteer), Payload: map[string]any{"text": "focus"}},
		{ID: "c-follow", Type: string(protocol.CmdFollowUp), Payload: map[string]any{"text": "next"}},
		{ID: "c-abort", Type: string(protocol.CmdAbort), Payload: map[string]any{}},
		{ID: "c-tools", Type: string(protocol.CmdSetActiveTools), Payload: map[string]any{"tools": []any{}}},
		{ID: "c-new", Type: string(protocol.CmdNewSession), Payload: map[string]any{}},
		{ID: "c-switch", Type: string(protocol.CmdSwitchSession), Payload: map[string]any{"session_id": parentID}},
		{ID: "c-branch", Type: string(protocol.CmdBranchSession), Payload: map[string]any{"session_id": parentID}},
		{ID: "c-ext", Type: string(protocol.CmdExtensionCmd), Payload: map[string]any{"name": "missing", "payload": map[string]any{}}},
	}

	for _, tc := range cases {
		resp := srv.dispatch(tc)
		if resp.ID != tc.ID {
			t.Fatalf("response id mismatch for %s: got=%q want=%q", tc.Type, resp.ID, tc.ID)
		}
		if resp.Error != nil && resp.Error.Code == "not_implemented" {
			t.Fatalf("command unexpectedly not implemented: %s", tc.Type)
		}
	}
}

func TestDispatchPromptWithWaitFalseIsAccepted(t *testing.T) {
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

	resp := srv.dispatch(protocol.Envelope{
		ID:      "prompt-wait-false",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello", "wait": false},
	})
	if !resp.OK || resp.Type != "accepted" || resp.Error != nil {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got, _ := resp.Payload["command"].(string); got != "prompt" {
		t.Fatalf("unexpected accepted command payload: %+v", resp.Payload)
	}
	if runID, _ := resp.Payload["run_id"].(string); runID == "" {
		t.Fatalf("missing run_id in accepted payload: %+v", resp.Payload)
	}
}
