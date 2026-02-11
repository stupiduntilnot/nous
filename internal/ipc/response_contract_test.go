package ipc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/extension"
	"oh-my-agent/internal/provider"
	"oh-my-agent/internal/protocol"
	"oh-my-agent/internal/session"
)

func TestDispatchResponsesSatisfySpecPayloadRequirements(t *testing.T) {
	respReqs := loadResponsePayloadRequirements(t)

	base := testWorkDir(t)
	srv := NewServer(filepath.Join(base, "core.sock"))

	mgr, err := session.NewManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("new session manager failed: %v", err)
	}
	srv.SetSessionManager(mgr)

	ext := extension.NewManager()
	if err := ext.RegisterCommand("echo", func(payload map[string]any) (map[string]any, error) {
		return payload, nil
	}); err != nil {
		t.Fatalf("register extension command failed: %v", err)
	}
	e := core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	e.SetExtensionManager(ext)
	srv.SetEngine(e, core.NewCommandLoop(e))

	cases := []struct {
		name    string
		env     protocol.Envelope
		typeKey string
	}{
		{
			name: "pong",
			env: protocol.Envelope{ID: "r-ping", Type: string(protocol.CmdPing), Payload: map[string]any{}},
			typeKey: "pong",
		},
		{
			name: "session",
			env: protocol.Envelope{ID: "r-new", Type: string(protocol.CmdNewSession), Payload: map[string]any{}},
			typeKey: "session",
		},
		{
			name: "accepted_prompt",
			env: protocol.Envelope{ID: "r-async", Type: string(protocol.CmdPrompt), Payload: map[string]any{"text": "hello", "wait": false}},
			typeKey: "accepted:prompt",
		},
		{
			name: "result",
			env: protocol.Envelope{ID: "r-sync", Type: string(protocol.CmdPrompt), Payload: map[string]any{"text": "hello", "wait": true}},
			typeKey: "result",
		},
		{
			name: "extension_result",
			env: protocol.Envelope{ID: "r-ext", Type: string(protocol.CmdExtensionCmd), Payload: map[string]any{"name": "echo", "payload": map[string]any{"text": "ok"}}},
			typeKey: "extension_result",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := srv.dispatch(tc.env)
			if !resp.OK {
				t.Fatalf("dispatch failed: %+v", resp)
			}
			if tc.typeKey == "accepted:prompt" {
				if cmd, _ := resp.Payload["command"].(string); cmd != "prompt" {
					t.Fatalf("accepted response command mismatch: %+v", resp.Payload)
				}
			}
			fields, ok := respReqs[tc.typeKey]
			if !ok {
				t.Fatalf("missing response requirements for key %q", tc.typeKey)
			}
			for _, f := range fields {
				if _, ok := resp.Payload[f]; !ok {
					t.Fatalf("response type %q missing required payload field %q: %+v", tc.typeKey, f, resp.Payload)
				}
			}
		})
	}
}

func loadResponsePayloadRequirements(t *testing.T) map[string][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.FromSlash("../../docs/protocol/openapi-like.json"))
	if err != nil {
		t.Fatalf("read protocol spec failed: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode protocol spec failed: %v", err)
	}
	rawReqs, ok := doc["x-response-payload-requirements"].(map[string]any)
	if !ok {
		t.Fatalf("x-response-payload-requirements missing")
	}
	out := make(map[string][]string, len(rawReqs))
	for k, v := range rawReqs {
		rawFields, ok := v.([]any)
		if !ok {
			t.Fatalf("requirements for %q must be array, got %#v", k, v)
		}
		fields := make([]string, 0, len(rawFields))
		for _, f := range rawFields {
			s, ok := f.(string)
			if !ok {
				t.Fatalf("requirements field for %q must be string, got %#v", k, f)
			}
			fields = append(fields, s)
		}
		out[k] = fields
	}
	return out
}
