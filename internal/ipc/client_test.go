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
