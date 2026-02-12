package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"nous/internal/core"
	"nous/internal/extension"
	"nous/internal/protocol"
	"nous/internal/provider"
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

type firstTurnBlockingAdapter struct {
	mu sync.Mutex
	n  int

	started      chan struct{}
	releaseFirst chan struct{}
}

func (a *firstTurnBlockingAdapter) Stream(ctx context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 4)
	go func() {
		defer close(out)
		a.mu.Lock()
		a.n++
		call := a.n
		a.mu.Unlock()

		out <- provider.Event{Type: provider.EventStart}
		if call == 1 {
			select {
			case a.started <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				out <- provider.Event{Type: provider.EventError, Err: ctx.Err()}
				return
			case <-a.releaseFirst:
			}
		}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: fmt.Sprintf("turn-%d:%s", call, provider.RenderMessages(req.Messages))}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestQueuedFollowUpKeepsSingleRunLifecycleEvents(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	adapter := &firstTurnBlockingAdapter{
		started:      make(chan struct{}, 1),
		releaseFirst: make(chan struct{}, 1),
	}
	engine := core.NewEngine(core.NewRuntime(), adapter)
	engine.SetExtensionManager(extension.NewManager())
	srv.SetEngine(engine, core.NewCommandLoop(engine))

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

	promptResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "a1-run-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "first", "wait": false},
	})
	if err != nil || !promptResp.OK {
		t.Fatalf("async prompt failed: resp=%+v err=%v", promptResp, err)
	}
	runID, _ := promptResp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id in prompt response: %+v", promptResp.Payload)
	}

	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatalf("first turn did not start")
	}

	followResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "a1-run-follow",
		Type:    string(protocol.CmdFollowUp),
		Payload: map[string]any{"text": "second"},
	})
	if err != nil || !followResp.OK {
		t.Fatalf("follow_up failed: resp=%+v err=%v", followResp, err)
	}
	adapter.releaseFirst <- struct{}{}

	reader := bufio.NewReader(eventConn)
	agentStart := 0
	agentEnd := 0
	turnStart := 0
	turnEnd := 0

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		_ = eventConn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
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
		if gotRunID, _ := env.Payload["run_id"].(string); gotRunID != runID {
			continue
		}
		switch env.Type {
		case string(protocol.EvAgentStart):
			agentStart++
		case string(protocol.EvAgentEnd):
			agentEnd++
		case string(protocol.EvTurnStart):
			turnStart++
		case string(protocol.EvTurnEnd):
			turnEnd++
		}
		if agentEnd == 1 {
			break
		}
	}

	if agentStart != 1 || agentEnd != 1 {
		t.Fatalf("expected single run lifecycle events, got agent_start=%d agent_end=%d", agentStart, agentEnd)
	}
	if turnStart < 2 || turnEnd < 2 {
		t.Fatalf("expected queued follow_up to produce second turn, got turn_start=%d turn_end=%d", turnStart, turnEnd)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}
