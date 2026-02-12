package ipc

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"nous/internal/core"
	"nous/internal/protocol"
	"nous/internal/provider"
)

func TestCaptureRunTraceAndReplayValidation(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}
	if err := waitForSocket(socket+".events", 2*time.Second); err != nil {
		t.Fatalf("event socket not ready: %v", err)
	}

	traceCh := make(chan []protocol.Envelope, 1)
	traceErrCh := make(chan error, 1)
	go func() {
		trace, err := CaptureRunTrace(socket, "run-1", 4*time.Second)
		if err != nil {
			traceErrCh <- err
			return
		}
		traceCh <- trace
	}()

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "trace-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "trace me", "wait": false},
	})
	if err != nil {
		t.Fatalf("prompt wait=false failed: %v", err)
	}
	if !resp.OK || resp.Type != "accepted" {
		t.Fatalf("unexpected prompt response: %+v", resp)
	}
	if got, _ := resp.Payload["run_id"].(string); got != "run-1" {
		t.Fatalf("unexpected run id: got=%q payload=%+v", got, resp.Payload)
	}

	var trace []protocol.Envelope
	select {
	case trace = <-traceCh:
	case err := <-traceErrCh:
		t.Fatalf("capture trace failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for trace capture")
	}
	if len(trace) == 0 {
		t.Fatalf("expected non-empty trace")
	}
	if err := ValidateRunTrace(trace, "run-1"); err != nil {
		t.Fatalf("trace validation failed: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteTraceNDJSON(&buf, trace); err != nil {
		t.Fatalf("write trace failed: %v", err)
	}
	decoded, err := ReadTraceNDJSON(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read trace failed: %v", err)
	}
	if len(decoded) != len(trace) {
		t.Fatalf("trace roundtrip count mismatch: got=%d want=%d", len(decoded), len(trace))
	}
	if err := ValidateRunTrace(decoded, "run-1"); err != nil {
		t.Fatalf("roundtrip trace validation failed: %v", err)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func TestValidateRunTraceRejectsMismatchedRun(t *testing.T) {
	err := ValidateRunTrace([]protocol.Envelope{
		{Type: string(protocol.EvAgentStart), Payload: map[string]any{"run_id": "run-1"}},
		{Type: string(protocol.EvAgentEnd), Payload: map[string]any{"run_id": "run-1"}},
	}, "run-2")
	if err == nil {
		t.Fatalf("expected mismatched run validation error")
	}
}

type traceBurstProvider struct {
	total int
}

func (p traceBurstProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 8)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventStart}
		for i := 0; i < p.total; i++ {
			out <- provider.Event{Type: provider.EventTextDelta, Delta: fmt.Sprintf("trace-%03d", i)}
		}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestCaptureRunTraceWithBurstMessageUpdates(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	engine := core.NewEngine(core.NewRuntime(), traceBurstProvider{total: 120})
	srv.SetEngine(engine, core.NewCommandLoop(engine))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ctx) }()
	if err := waitForSocket(socket, 2*time.Second); err != nil {
		t.Fatalf("server not ready: %v", err)
	}
	if err := waitForSocket(socket+".events", 2*time.Second); err != nil {
		t.Fatalf("event socket not ready: %v", err)
	}

	traceCh := make(chan []protocol.Envelope, 1)
	traceErrCh := make(chan error, 1)
	go func() {
		trace, err := CaptureRunTrace(socket, "run-1", 6*time.Second)
		if err != nil {
			traceErrCh <- err
			return
		}
		traceCh <- trace
	}()

	resp, err := SendCommand(socket, protocol.Envelope{
		ID:      "trace-burst",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "trace burst", "wait": false},
	})
	if err != nil || !resp.OK {
		t.Fatalf("prompt wait=false failed: resp=%+v err=%v", resp, err)
	}

	var trace []protocol.Envelope
	select {
	case trace = <-traceCh:
	case err := <-traceErrCh:
		t.Fatalf("capture trace failed: %v", err)
	case <-time.After(7 * time.Second):
		t.Fatalf("timed out waiting for burst trace capture")
	}
	if err := ValidateRunTrace(trace, "run-1"); err != nil {
		t.Fatalf("trace validation failed: %v", err)
	}
	updates := 0
	for _, ev := range trace {
		if ev.Type == string(protocol.EvMessageUpdate) {
			updates++
		}
	}
	if updates != 120 {
		t.Fatalf("expected 120 message_update events in trace, got %d", updates)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}
