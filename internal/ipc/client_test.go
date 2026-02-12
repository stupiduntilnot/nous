package ipc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nous/internal/core"
	"nous/internal/extension"
	"nous/internal/protocol"
	"nous/internal/provider"
	"nous/internal/session"
)

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (b *blockingExecutor) Prompt(ctx context.Context, _ string, _ string) (string, error) {
	select {
	case b.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-b.release:
		return "ok", nil
	}
}

type orderedExecutor struct {
	started chan string
	release chan struct{}

	mu    sync.Mutex
	calls []string
}

func newOrderedExecutor() *orderedExecutor {
	return &orderedExecutor{
		started: make(chan string, 32),
		release: make(chan struct{}, 32),
	}
}

func (o *orderedExecutor) Prompt(ctx context.Context, _ string, prompt string) (string, error) {
	o.mu.Lock()
	o.calls = append(o.calls, prompt)
	o.mu.Unlock()

	select {
	case o.started <- prompt:
	default:
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-o.release:
		return "ok:" + prompt, nil
	}
}

func (o *orderedExecutor) Calls() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]string, len(o.calls))
	copy(out, o.calls)
	return out
}

type echoPromptAdapter struct{}

func (e *echoPromptAdapter) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 3)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			out <- provider.Event{Type: provider.EventError, Err: ctx.Err()}
			return
		default:
		}
		out <- provider.Event{Type: provider.EventStart}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: provider.RenderMessages(req.Messages)}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

type toolLogProvider struct{}

func (p toolLogProvider) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event)
	go func() {
		defer close(out)
		if hasToolResultMessage(req.Messages) {
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventStart}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "tc-1", Name: "echo", Arguments: map[string]any{"text": "x"}}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func hasToolResultMessage(messages []provider.Message) bool {
	for _, msg := range messages {
		if msg.Role == "tool_result" {
			return true
		}
	}
	return false
}

func TestCorectlPing(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "ping-test",
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("send ping failed: %v", err)
	}
	if !resp.OK || resp.Type != "pong" {
		t.Fatalf("unexpected response: %+v", resp)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestServerLogsRuntimeContextFields(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	var logs bytes.Buffer
	srv.SetLogWriter(&logs)

	e := core.NewEngine(core.NewRuntime(), toolLogProvider{})
	e.SetTools([]core.Tool{
		core.ToolFunc{ToolName: "echo", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "ok", nil
		}},
	})
	srv.engine = e
	srv.loop = core.NewCommandLoop(e)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "log-1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "log me", "wait": true},
	})
	if err != nil || !resp.OK {
		t.Fatalf("prompt wait failed: resp=%+v err=%v", resp, err)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}

	seenRun := false
	seenTurn := false
	seenMessage := false
	seenTool := false
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("invalid log json: %v (%q)", err, line)
		}
		if _, ok := decoded["run_id"]; ok {
			seenRun = true
		}
		if _, ok := decoded["turn_id"]; ok {
			seenTurn = true
		}
		if _, ok := decoded["message_id"]; ok {
			seenMessage = true
		}
		if _, ok := decoded["tool_call_id"]; ok {
			seenTool = true
		}
	}
	if !seenRun || !seenTurn || !seenMessage || !seenTool {
		t.Fatalf("missing expected log context fields: run=%v turn=%v message=%v tool=%v logs=%q", seenRun, seenTurn, seenMessage, seenTool, logs.String())
	}
}

func TestSessionNewAndSwitch(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	newResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "new-session",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("new_session failed: %v", err)
	}
	if !newResp.OK || newResp.Type != "session" {
		t.Fatalf("unexpected new_session response: %+v", newResp)
	}
	id, _ := newResp.Payload["session_id"].(string)
	if id == "" {
		t.Fatalf("expected non-empty session_id")
	}

	switchResp, err := SendCommand(socket, protocol.Envelope{
		ID:   "switch-session",
		Type: string(protocol.CmdSwitchSession),
		Payload: map[string]any{
			"session_id": id,
		},
	})
	if err != nil {
		t.Fatalf("switch_session failed: %v", err)
	}
	if !switchResp.OK || switchResp.Type != "session" {
		t.Fatalf("unexpected switch_session response: %+v", switchResp)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestSwitchSessionNotFound(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:   "switch-missing",
		Type: string(protocol.CmdSwitchSession),
		Payload: map[string]any{
			"session_id": "missing",
		},
	})
	if err != nil {
		t.Fatalf("switch_session failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected switch_session to fail for missing session: %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "session_not_found" {
		t.Fatalf("unexpected error body: %+v", resp.Error)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestBranchSession(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	newResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "new-parent",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil || !newResp.OK {
		t.Fatalf("new_session failed: resp=%+v err=%v", newResp, err)
	}
	parentID, _ := newResp.Payload["session_id"].(string)
	if parentID == "" {
		t.Fatalf("expected parent session id")
	}

	branchResp, err := SendCommand(socket, protocol.Envelope{
		ID:   "branch-1",
		Type: string(protocol.CmdBranchSession),
		Payload: map[string]any{
			"session_id": parentID,
		},
	})
	if err != nil {
		t.Fatalf("branch_session failed: %v", err)
	}
	if !branchResp.OK || branchResp.Type != "session" {
		t.Fatalf("unexpected branch_session response: %+v", branchResp)
	}
	branchID, _ := branchResp.Payload["session_id"].(string)
	if branchID == "" || branchID == parentID {
		t.Fatalf("invalid branch session id: %q", branchID)
	}
	if gotParent, _ := branchResp.Payload["parent_id"].(string); gotParent != parentID {
		t.Fatalf("unexpected parent_id: got=%q want=%q", gotParent, parentID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestBranchSessionBackwardCompatibleParentID(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()

	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	newResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "new-parent-compat",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil || !newResp.OK {
		t.Fatalf("new_session failed: resp=%+v err=%v", newResp, err)
	}
	parentID, _ := newResp.Payload["session_id"].(string)
	if parentID == "" {
		t.Fatalf("expected parent session id")
	}

	branchResp, err := SendCommand(socket, protocol.Envelope{
		ID:   "branch-parent-id",
		Type: string(protocol.CmdBranchSession),
		Payload: map[string]any{
			"parent_id": parentID,
		},
	})
	if err != nil {
		t.Fatalf("branch_session failed: %v", err)
	}
	if !branchResp.OK || branchResp.Type != "session" {
		t.Fatalf("unexpected branch_session response: %+v", branchResp)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestExtensionCommandDispatch(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	m := extension.NewManager()
	if err := m.RegisterCommand("hello", func(payload map[string]any) (map[string]any, error) {
		name, _ := payload["name"].(string)
		return map[string]any{"msg": "hi " + name}, nil
	}); err != nil {
		t.Fatalf("register extension command failed: %v", err)
	}
	e := core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	e.SetExtensionManager(m)
	srv.engine = e
	srv.loop = core.NewCommandLoop(e)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:   "ext-1",
		Type: string(protocol.CmdExtensionCmd),
		Payload: map[string]any{
			"name":    "hello",
			"payload": map[string]any{"name": "dev"},
		},
	})
	if err != nil {
		t.Fatalf("extension command failed: %v", err)
	}
	if !resp.OK || resp.Type != "extension_result" {
		t.Fatalf("unexpected extension command response: %+v", resp)
	}
	if got, _ := resp.Payload["msg"].(string); got != "hi dev" {
		t.Fatalf("unexpected extension command payload: %+v", resp.Payload)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestExtensionCommandNotFound(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:   "ext-missing",
		Type: string(protocol.CmdExtensionCmd),
		Payload: map[string]any{
			"name":    "missing",
			"payload": map[string]any{},
		},
	})
	if err != nil {
		t.Fatalf("extension command failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected extension command to fail for missing command: %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "command_rejected" {
		t.Fatalf("unexpected error body: %+v", resp.Error)
	}
	if !strings.Contains(resp.Error.Message, "extension_command_not_found") {
		t.Fatalf("unexpected error message: %+v", resp.Error)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestSetActiveToolsCommand(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	e := core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	e.SetTools([]core.Tool{
		core.ToolFunc{ToolName: "tool_a", Run: func(_ context.Context, _ map[string]any) (string, error) { return "ok", nil }},
		core.ToolFunc{ToolName: "tool_b", Run: func(_ context.Context, _ map[string]any) (string, error) { return "ok", nil }},
	})
	srv.engine = e
	srv.loop = core.NewCommandLoop(e)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	okResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "tools-ok",
		Type:    string(protocol.CmdSetActiveTools),
		Payload: map[string]any{"tools": []any{"tool_a"}},
	})
	if err != nil {
		t.Fatalf("set_active_tools failed: %v", err)
	}
	if !okResp.OK {
		t.Fatalf("expected set_active_tools success, got: %+v", okResp)
	}

	badPayloadResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "tools-bad-payload",
		Type:    string(protocol.CmdSetActiveTools),
		Payload: map[string]any{"tools": "tool_a"},
	})
	if err != nil {
		t.Fatalf("set_active_tools bad payload failed: %v", err)
	}
	if badPayloadResp.OK || badPayloadResp.Error == nil || badPayloadResp.Error.Code != "invalid_payload" {
		t.Fatalf("expected invalid_payload error, got: %+v", badPayloadResp)
	}

	missingToolResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "tools-missing",
		Type:    string(protocol.CmdSetActiveTools),
		Payload: map[string]any{"tools": []any{"unknown_tool"}},
	})
	if err != nil {
		t.Fatalf("set_active_tools missing tool failed: %v", err)
	}
	if missingToolResp.OK || missingToolResp.Error == nil || missingToolResp.Error.Code != "command_rejected" {
		t.Fatalf("expected command_rejected error, got: %+v", missingToolResp)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptSteerFollowUpAbortCommands(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	srv.loop = core.NewCommandLoop(srv.engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	promptResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "r1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello"},
	})
	if err != nil {
		t.Fatalf("prompt command failed: %v", err)
	}
	if !promptResp.OK || promptResp.Type != "result" || promptResp.ID != "r1" {
		t.Fatalf("unexpected prompt response: %+v", promptResp)
	}

	steerResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "r2",
		Type:    string(protocol.CmdSteer),
		Payload: map[string]any{"text": "focus"},
	})
	if err != nil {
		t.Fatalf("steer command failed: %v", err)
	}
	if steerResp.OK || steerResp.Error == nil || steerResp.Error.Code != "command_rejected" {
		t.Fatalf("unexpected steer response: %+v", steerResp)
	}

	followResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "r3",
		Type:    string(protocol.CmdFollowUp),
		Payload: map[string]any{"text": "and then"},
	})
	if err != nil {
		t.Fatalf("follow_up command failed: %v", err)
	}
	if followResp.OK || followResp.Error == nil || followResp.Error.Code != "command_rejected" {
		t.Fatalf("unexpected follow_up response: %+v", followResp)
	}

	abortResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "r4",
		Type:    string(protocol.CmdAbort),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("abort command failed: %v", err)
	}
	if abortResp.OK || abortResp.Error == nil || abortResp.Error.Code != "command_rejected" {
		t.Fatalf("unexpected abort response: %+v", abortResp)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptCommandWithWaitFalseAcceptedOverIPC(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	srv.loop = core.NewCommandLoop(srv.engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "p-wait-false",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "p0", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt command failed: %v", err)
	}
	if !resp.OK || resp.Type != "accepted" || resp.Error != nil {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got, _ := resp.Payload["command"].(string); got != "prompt" {
		t.Fatalf("unexpected accepted command payload: %+v", resp.Payload)
	}
	if runID, _ := resp.Payload["run_id"].(string); runID == "" {
		t.Fatalf("expected run_id in accepted payload: %+v", resp.Payload)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptValidationErrorHasRequestID(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "bad-text",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("prompt command failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected prompt with empty payload to fail")
	}
	if resp.ID != "bad-text" {
		t.Fatalf("expected request id echo, got %q", resp.ID)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_payload" {
		t.Fatalf("unexpected error body: %+v", resp.Error)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestAbortWithoutActiveRunReturnsRejected(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "abort-empty",
		Type:    string(protocol.CmdAbort),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("abort command failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected abort to fail without active run")
	}
	if resp.Error == nil || resp.Error.Code != "command_rejected" {
		t.Fatalf("expected command_rejected error code, got %+v", resp.Error)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptWaitReturnsEvents(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "wait-1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello", "wait": true},
	})
	if err != nil {
		t.Fatalf("prompt wait failed: %v", err)
	}
	if !resp.OK || resp.Type != "result" {
		t.Fatalf("unexpected prompt wait response: %+v", resp)
	}
	if _, ok := resp.Payload["output"].(string); !ok {
		t.Fatalf("expected output in payload")
	}
	if sid, _ := resp.Payload["session_id"].(string); sid == "" {
		t.Fatalf("expected session_id in payload")
	}
	rawEvents, ok := resp.Payload["events"].([]any)
	if !ok || len(rawEvents) == 0 {
		t.Fatalf("expected non-empty events payload, got: %#v", resp.Payload["events"])
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptWaitPersistsSessionRecords(t *testing.T) {
	base := testWorkDir(t)
	socket := filepath.Join(base, "core.sock")
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "persist-1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello session", "wait": true},
	})
	if err != nil {
		t.Fatalf("prompt wait failed: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected successful response: %+v", resp)
	}
	sessionID, _ := resp.Payload["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("missing session_id: %+v", resp.Payload)
	}

	mgr, err := session.NewManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	records, skipped, err := mgr.Recover(sessionID)
	if err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	if skipped != 0 {
		t.Fatalf("expected no skipped records, got %d", skipped)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 message records, got %d", len(records))
	}

	var first map[string]any
	if err := json.Unmarshal(records[0], &first); err != nil {
		t.Fatalf("decode first record failed: %v", err)
	}
	if first["role"] != "user" {
		t.Fatalf("expected first record role=user, got %v", first["role"])
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptWaitIncludesPriorSessionContext(t *testing.T) {
	base := testWorkDir(t)
	socket := filepath.Join(base, "core.sock")
	srv := NewServer(socket)
	srv.engine = core.NewEngine(core.NewRuntime(), &echoPromptAdapter{})
	srv.loop = core.NewCommandLoop(srv.engine)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	first, err := SendCommand(socket, protocol.Envelope{
		ID:      "ctx-1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "first question", "wait": true},
	})
	if err != nil || !first.OK {
		t.Fatalf("first prompt failed: resp=%+v err=%v", first, err)
	}

	second, err := SendCommand(socket, protocol.Envelope{
		ID:      "ctx-2",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "second question", "wait": true},
	})
	if err != nil || !second.OK {
		t.Fatalf("second prompt failed: resp=%+v err=%v", second, err)
	}
	out, _ := second.Payload["output"].(string)
	if !strings.Contains(out, "user: first question") {
		t.Fatalf("expected output prompt to include prior context, got: %s", out)
	}
	if !strings.Contains(out, "assistant:") {
		t.Fatalf("expected output prompt to include assistant context, got: %s", out)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestPromptPersistsSessionRecords(t *testing.T) {
	base := testWorkDir(t)
	socket := filepath.Join(base, "core.sock")
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	newResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "new-async",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil || !newResp.OK {
		t.Fatalf("new_session failed: resp=%+v err=%v", newResp, err)
	}
	sessionID, _ := newResp.Payload["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("missing session id: %+v", newResp.Payload)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "prompt-1",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "prompt test", "wait": true},
	})
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if !resp.OK || resp.Type != "result" {
		t.Fatalf("unexpected prompt response: %+v", resp)
	}
	if gotSID, _ := resp.Payload["session_id"].(string); gotSID != sessionID {
		t.Fatalf("result session_id mismatch: got=%q want=%q payload=%+v", gotSID, sessionID, resp.Payload)
	}

	mgr, err := session.NewManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		records, _, err := mgr.Recover(sessionID)
		if err == nil && len(records) >= 2 {
			var first map[string]any
			if err := json.Unmarshal(records[0], &first); err != nil {
				t.Fatalf("decode first record failed: %v", err)
			}
			if first["role"] != "user" {
				t.Fatalf("expected first role=user, got %v", first["role"])
			}
			cancel()
			if err := <-errCh; err != nil {
				t.Fatalf("server returned error: %v", err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("session records not persisted in time")
}

func TestPromptWaitFalseIsAccepted(t *testing.T) {
	base := testWorkDir(t)
	socket := filepath.Join(base, "core.sock")
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "prompt-wait-false",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "rejected prompt", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt request failed: %v", err)
	}
	if !resp.OK || resp.Type != "accepted" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got, _ := resp.Payload["command"].(string); got != "prompt" {
		t.Fatalf("unexpected accepted command payload: %+v", resp.Payload)
	}
	if runID, _ := resp.Payload["run_id"].(string); runID == "" {
		t.Fatalf("expected run_id in accepted payload: %+v", resp.Payload)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestCommandTimeoutReturnsStandardError(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	srv.timeout = 20 * time.Millisecond
	srv.dispatchOverride = func(env protocol.Envelope) protocol.ResponseEnvelope {
		time.Sleep(120 * time.Millisecond)
		return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "late", Payload: map[string]any{}})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "slow-1",
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("send command failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected timeout response to fail")
	}
	if resp.Error == nil || resp.Error.Code != "timeout" {
		t.Fatalf("unexpected timeout error: %+v", resp.Error)
	}
	if resp.ID != "slow-1" {
		t.Fatalf("expected id echo for timeout, got %q", resp.ID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestAsyncPromptRunControlAcceptsSteerFollowUpAbort(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	exec := newOrderedExecutor()
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	srv.loop = core.NewCommandLoop(exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	promptResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-ctrl-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "p0", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt wait=false failed: %v", err)
	}
	if !promptResp.OK || promptResp.Type != "accepted" {
		t.Fatalf("unexpected prompt response: %+v", promptResp)
	}
	if got, _ := promptResp.Payload["command"].(string); got != "prompt" {
		t.Fatalf("unexpected prompt accepted payload: %+v", promptResp.Payload)
	}
	runID, _ := promptResp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id in prompt accepted payload: %+v", promptResp.Payload)
	}

	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("prompt turn did not start")
	}

	steerResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-ctrl-steer",
		Type:    string(protocol.CmdSteer),
		Payload: map[string]any{"text": "s1"},
	})
	if err != nil {
		t.Fatalf("steer failed: %v", err)
	}
	if !steerResp.OK || steerResp.Type != "accepted" {
		t.Fatalf("unexpected steer response: %+v", steerResp)
	}
	if got, _ := steerResp.Payload["run_id"].(string); got != runID {
		t.Fatalf("steer run_id mismatch: got=%q want=%q payload=%+v", got, runID, steerResp.Payload)
	}

	followResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-ctrl-follow",
		Type:    string(protocol.CmdFollowUp),
		Payload: map[string]any{"text": "f1"},
	})
	if err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}
	if !followResp.OK || followResp.Type != "accepted" {
		t.Fatalf("unexpected follow_up response: %+v", followResp)
	}
	if got, _ := followResp.Payload["run_id"].(string); got != runID {
		t.Fatalf("follow_up run_id mismatch: got=%q want=%q payload=%+v", got, runID, followResp.Payload)
	}

	abortResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-ctrl-abort",
		Type:    string(protocol.CmdAbort),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("abort failed: %v", err)
	}
	if !abortResp.OK || abortResp.Type != "accepted" {
		t.Fatalf("unexpected abort response: %+v", abortResp)
	}
	if got, _ := abortResp.Payload["run_id"].(string); got != runID {
		t.Fatalf("abort run_id mismatch: got=%q want=%q payload=%+v", got, runID, abortResp.Payload)
	}

	waitForLoopState(t, srv.loop, core.StateIdle, 2*time.Second)
	if got := exec.Calls(); len(got) != 1 || got[0] != "p0" {
		t.Fatalf("abort should prevent queued turns from executing: %v", got)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestAsyncPromptRunControlSteerPreemptsFollowUpOverIPC(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	exec := newOrderedExecutor()
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	srv.loop = core.NewCommandLoop(exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	promptResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-order-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "p0", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt wait=false failed: %v", err)
	}
	if !promptResp.OK || promptResp.Type != "accepted" {
		t.Fatalf("unexpected prompt response: %+v", promptResp)
	}

	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("prompt turn did not start")
	}

	followResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-order-follow",
		Type:    string(protocol.CmdFollowUp),
		Payload: map[string]any{"text": "f1"},
	})
	if err != nil {
		t.Fatalf("follow_up failed: %v", err)
	}
	if !followResp.OK {
		t.Fatalf("expected follow_up accepted, got: %+v", followResp)
	}

	steerResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-order-steer",
		Type:    string(protocol.CmdSteer),
		Payload: map[string]any{"text": "s1"},
	})
	if err != nil {
		t.Fatalf("steer failed: %v", err)
	}
	if !steerResp.OK {
		t.Fatalf("expected steer accepted, got: %+v", steerResp)
	}

	for range 8 {
		exec.release <- struct{}{}
	}
	waitForLoopState(t, srv.loop, core.StateIdle, 2*time.Second)

	got := exec.Calls()
	want := []string{"p0", "s1", "f1"}
	if len(got) != len(want) {
		t.Fatalf("unexpected run control call count: got=%d want=%d calls=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected run control order at %d: got=%q want=%q all=%v", i, got[i], want[i], got)
		}
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestAsyncPromptPersistsToOriginalSessionWhenSwitchingMidRun(t *testing.T) {
	base := testWorkDir(t)
	socket := filepath.Join(base, "core.sock")
	srv := NewServer(socket)

	exec := newOrderedExecutor()
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	srv.loop = core.NewCommandLoop(exec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	firstSessionResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "sess-1",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil || !firstSessionResp.OK {
		t.Fatalf("new session failed: resp=%+v err=%v", firstSessionResp, err)
	}
	origSessionID, _ := firstSessionResp.Payload["session_id"].(string)
	if origSessionID == "" {
		t.Fatalf("missing first session id: %+v", firstSessionResp.Payload)
	}

	promptResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "switch-mid-run-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "p0", "wait": false},
	})
	if err != nil || !promptResp.OK {
		t.Fatalf("prompt wait=false failed: resp=%+v err=%v", promptResp, err)
	}

	select {
	case <-exec.started:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("prompt turn did not start")
	}

	secondSessionResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "sess-2",
		Type:    string(protocol.CmdNewSession),
		Payload: map[string]any{},
	})
	if err != nil || !secondSessionResp.OK {
		t.Fatalf("second new session failed: resp=%+v err=%v", secondSessionResp, err)
	}
	newSessionID, _ := secondSessionResp.Payload["session_id"].(string)
	if newSessionID == "" || newSessionID == origSessionID {
		t.Fatalf("unexpected second session id: orig=%q new=%q", origSessionID, newSessionID)
	}

	exec.release <- struct{}{}
	waitForLoopState(t, srv.loop, core.StateIdle, 2*time.Second)

	mgr, err := session.NewManager(filepath.Join(base, "sessions"))
	if err != nil {
		t.Fatalf("new manager failed: %v", err)
	}
	origRecords, _, err := mgr.Recover(origSessionID)
	if err != nil {
		t.Fatalf("recover original session failed: %v", err)
	}
	if len(origRecords) < 2 {
		t.Fatalf("expected original session to contain run records, got %d", len(origRecords))
	}

	newRecords, _, err := mgr.Recover(newSessionID)
	if err != nil {
		t.Fatalf("recover new session failed: %v", err)
	}
	if len(newRecords) != 0 {
		t.Fatalf("expected switched session to have no run records, got %d", len(newRecords))
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestSendCommandWithTimeoutDeadline(t *testing.T) {
	socket := testSocketPath(t)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(200 * time.Millisecond)
	}()

	_, err = SendCommandWithTimeout(socket, protocol.Envelope{
		ID:      "deadline-1",
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	}, 50*time.Millisecond)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !strings.Contains(err.Error(), "i/o timeout") {
		t.Fatalf("expected i/o timeout error, got: %v", err)
	}
	<-done
}

func TestCommandPanicReturnsInternalError(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	srv.dispatchOverride = func(protocol.Envelope) protocol.ResponseEnvelope {
		panic("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "panic-1",
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("send command failed: %v", err)
	}
	if resp.OK {
		t.Fatalf("expected panic response to fail")
	}
	if resp.Error == nil || resp.Error.Code != "internal_error" {
		t.Fatalf("unexpected panic error: %+v", resp.Error)
	}
	if resp.ID != "panic-1" {
		t.Fatalf("expected id echo for panic, got %q", resp.ID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func waitForSocket(socket string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

func waitForLoopState(t *testing.T, loop *core.CommandLoop, want core.RunState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if loop.State() == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("loop did not reach state %q", want)
}

func testSocketPath(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/oma-%d.sock", time.Now().UnixNano())
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func testWorkDir(t *testing.T) string {
	t.Helper()
	dir := fmt.Sprintf("/tmp/oma-%d", time.Now().UnixNano())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir workdir failed: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
