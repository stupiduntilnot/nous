package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"nous/internal/protocol"
)

func TestPromptWaitFalseStreamsEventsOverEventSocket(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server command socket not ready: %v", err)
	}
	if err := waitForSocket(socket+".events", 2*time.Second); err != nil {
		t.Fatalf("server event socket not ready: %v", err)
	}

	eventConn, err := net.Dial("unix", socket+".events")
	if err != nil {
		t.Fatalf("event stream dial failed: %v", err)
	}
	defer eventConn.Close()

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "async-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello stream", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt wait=false failed: %v", err)
	}
	if !resp.OK || resp.Type != "accepted" {
		t.Fatalf("unexpected async prompt response: %+v", resp)
	}
	runID, _ := resp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("accepted prompt missing run_id: %+v", resp.Payload)
	}

	reader := bufio.NewReader(eventConn)
	seenStart := false
	seenEnd := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (!seenStart || !seenEnd) {
		_ = eventConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			t.Fatalf("read event failed: %v", err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			t.Fatalf("decode event failed: %v", err)
		}
		evRunID, _ := env.Payload["run_id"].(string)
		if evRunID != runID {
			continue
		}
		if env.Type == string(protocol.EvAgentStart) {
			seenStart = true
		}
		if env.Type == string(protocol.EvAgentEnd) {
			seenEnd = true
		}
	}
	if !seenStart || !seenEnd {
		t.Fatalf("expected agent start/end on event stream for run %q", runID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}
