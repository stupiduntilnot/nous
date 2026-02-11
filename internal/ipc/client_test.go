package ipc

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"oh-my-agent/internal/protocol"
)

func TestCorectlPing(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "core.sock")
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
	socket := filepath.Join(t.TempDir(), "core.sock")
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
	socket := filepath.Join(t.TempDir(), "core.sock")
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
