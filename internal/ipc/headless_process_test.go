package ipc

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"oh-my-agent/internal/protocol"
)

func TestHeadlessCoreProcess(t *testing.T) {
	socket := testSocketPath(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "../../cmd/core", "-socket", socket)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start headless core: %v", err)
	}

	if err := waitForSocket(socket, 4*time.Second); err != nil {
		t.Fatalf("headless core not ready: %v", err)
	}

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "headless-ping",
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	})
	if err != nil {
		t.Fatalf("ping failed against headless core: %v", err)
	}
	if !resp.OK || resp.Type != "pong" {
		t.Fatalf("unexpected headless response: %+v", resp)
	}

	cancel()
	waitErr := cmd.Wait()
	if waitErr != nil && ctx.Err() == nil {
		t.Fatalf("headless core exit failed: %v", waitErr)
	}
}
