package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/provider"
	"oh-my-agent/internal/protocol"
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

func TestPromptSteerFollowUpAbortCommands(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)
	exec := &blockingExecutor{
		started: make(chan struct{}, 1),
		release: make(chan struct{}, 4),
	}
	srv.loop = core.NewCommandLoop(exec)
	srv.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())

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
	if !promptResp.OK || promptResp.Type != "accepted" || promptResp.ID != "r1" {
		t.Fatalf("unexpected prompt response: %+v", promptResp)
	}
	select {
	case <-exec.started:
	case <-time.After(time.Second):
		t.Fatalf("prompt did not start execution")
	}

	steerResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "r2",
		Type:    string(protocol.CmdSteer),
		Payload: map[string]any{"text": "focus"},
	})
	if err != nil {
		t.Fatalf("steer command failed: %v", err)
	}
	if !steerResp.OK || steerResp.ID != "r2" {
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
	if !followResp.OK || followResp.ID != "r3" {
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
	if !abortResp.OK || abortResp.ID != "r4" {
		t.Fatalf("unexpected abort response: %+v", abortResp)
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
	rawEvents, ok := resp.Payload["events"].([]any)
	if !ok || len(rawEvents) == 0 {
		t.Fatalf("expected non-empty events payload, got: %#v", resp.Payload["events"])
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

func testSocketPath(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/oma-%d.sock", time.Now().UnixNano())
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}
