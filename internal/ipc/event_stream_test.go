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

type twoToolCallAdapter struct{}

func (a twoToolCallAdapter) Stream(_ context.Context, req provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 8)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventStart}
		if hasToolResult(req.Messages) {
			out <- provider.Event{Type: provider.EventTextDelta, Delta: "done"}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "tc-1", Name: "first", Arguments: map[string]any{}}}
		out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "tc-2", Name: "second", Arguments: map[string]any{}}}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func hasToolResult(messages []provider.Message) bool {
	for _, msg := range messages {
		if msg.Role == "tool_result" {
			return true
		}
	}
	return false
}

func TestSteerDuringFirstToolSkipsRemainingToolCalls(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	firstToolStarted := make(chan struct{}, 1)
	releaseFirstTool := make(chan struct{}, 1)

	engine := core.NewEngine(core.NewRuntime(), twoToolCallAdapter{})
	engine.SetExtensionManager(extension.NewManager())
	engine.SetTools([]core.Tool{
		core.ToolFunc{ToolName: "first", Run: func(ctx context.Context, _ map[string]any) (string, error) {
			select {
			case firstToolStarted <- struct{}{}:
			default:
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-releaseFirstTool:
				return "first-ok", nil
			}
		}},
		core.ToolFunc{ToolName: "second", Run: func(_ context.Context, _ map[string]any) (string, error) {
			return "second-ok", nil
		}},
	})
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
		ID:      "a2-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "run tools", "wait": false},
	})
	if err != nil || !promptResp.OK {
		t.Fatalf("async prompt failed: resp=%+v err=%v", promptResp, err)
	}
	runID, _ := promptResp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id in prompt response: %+v", promptResp.Payload)
	}

	select {
	case <-firstToolStarted:
	case <-time.After(time.Second):
		t.Fatalf("first tool did not start")
	}

	steerResp, err := SendCommand(socket, protocol.Envelope{
		ID:      "a2-steer",
		Type:    string(protocol.CmdSteer),
		Payload: map[string]any{"text": "interrupt"},
	})
	if err != nil || !steerResp.OK {
		t.Fatalf("steer failed: resp=%+v err=%v", steerResp, err)
	}
	releaseFirstTool <- struct{}{}

	reader := bufio.NewReader(eventConn)
	foundSkipped := false
	secondToolRan := false

	deadline := time.Now().Add(5 * time.Second)
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
		toolName, _ := env.Payload["tool_name"].(string)
		switch env.Type {
		case string(protocol.EvToolExecutionUpdate):
			delta, _ := env.Payload["delta"].(string)
			if toolName == "second" && delta == "second-ok" {
				secondToolRan = true
			}
			if toolName == "second" && containsSkipped(delta) {
				foundSkipped = true
			}
		case string(protocol.EvAgentEnd):
			if foundSkipped || secondToolRan {
				goto done
			}
		}
	}

done:
	if secondToolRan {
		t.Fatalf("expected second tool call to be skipped when steer was queued")
	}
	if !foundSkipped {
		t.Fatalf("expected skipped tool update event for second tool")
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

type retryThenSuccessProvider struct{}

func (retryThenSuccessProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	out := make(chan provider.Event, 5)
	go func() {
		defer close(out)
		out <- provider.Event{Type: provider.EventStart}
		out <- provider.Event{Type: provider.EventWarning, Code: "provider_retry", Message: "openai retry attempt 1/3"}
		out <- provider.Event{Type: provider.EventTextDelta, Delta: "ok-after-retry"}
		out <- provider.Event{Type: provider.EventDone}
	}()
	return out
}

func TestEventStreamPreservesOrderUnderRetryThenSuccess(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	engine := core.NewEngine(core.NewRuntime(), retryThenSuccessProvider{})
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
		ID:      "retry-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "hello", "wait": false},
	})
	if err != nil || !promptResp.OK {
		t.Fatalf("async prompt failed: resp=%+v err=%v", promptResp, err)
	}
	runID, _ := promptResp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id in prompt response: %+v", promptResp.Payload)
	}

	reader := bufio.NewReader(eventConn)
	retryWarningIdx := -1
	messageUpdateIdx := -1
	eventIndex := 0
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
		if env.Type == string(protocol.EvWarning) {
			if code, _ := env.Payload["code"].(string); code == "provider_retry" && retryWarningIdx == -1 {
				retryWarningIdx = eventIndex
			}
		}
		if env.Type == string(protocol.EvMessageUpdate) && messageUpdateIdx == -1 {
			messageUpdateIdx = eventIndex
		}
		if env.Type == string(protocol.EvAgentEnd) {
			break
		}
		eventIndex++
	}

	if retryWarningIdx == -1 {
		t.Fatalf("expected provider_retry warning event on stream")
	}
	if messageUpdateIdx == -1 {
		t.Fatalf("expected message_update event on stream")
	}
	if retryWarningIdx > messageUpdateIdx {
		t.Fatalf("expected retry warning before message update, got warning_idx=%d message_update_idx=%d", retryWarningIdx, messageUpdateIdx)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

type progressEventsProvider struct {
	calls int
}

func (p *progressEventsProvider) Stream(_ context.Context, _ provider.Request) <-chan provider.Event {
	p.calls++
	out := make(chan provider.Event, 4)
	go func(call int) {
		defer close(out)
		if call == 1 {
			out <- provider.Event{Type: provider.EventToolCall, ToolCall: provider.ToolCall{ID: "tc-progress", Name: "progressive", Arguments: map[string]any{}}}
			out <- provider.Event{Type: provider.EventDone}
			return
		}
		out <- provider.Event{Type: provider.EventDone}
	}(p.calls)
	return out
}

func TestToolExecutionProgressUpdatesAreStreamed(t *testing.T) {
	socket := testSocketPath(t)
	srv := NewServer(socket)

	engine := core.NewEngine(core.NewRuntime(), &progressEventsProvider{})
	engine.SetExtensionManager(extension.NewManager())
	engine.SetTools([]core.Tool{
		core.ProgressiveToolFunc{
			ToolName: "progressive",
			Run: func(_ context.Context, _ map[string]any, progress core.ToolProgressFunc) (string, error) {
				progress("10%")
				progress("50%")
				return "complete", nil
			},
		},
	})
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
		ID:      "progress-prompt",
		Type:    string(protocol.CmdPrompt),
		Payload: map[string]any{"text": "run tool", "wait": false},
	})
	if err != nil || !promptResp.OK {
		t.Fatalf("async prompt failed: resp=%+v err=%v", promptResp, err)
	}
	runID, _ := promptResp.Payload["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id in prompt response: %+v", promptResp.Payload)
	}

	reader := bufio.NewReader(eventConn)
	updates := make([]string, 0, 4)
	deadline := time.Now().Add(5 * time.Second)
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
		if env.Type == string(protocol.EvToolExecutionUpdate) {
			delta, _ := env.Payload["delta"].(string)
			updates = append(updates, delta)
		}
		if env.Type == string(protocol.EvAgentEnd) {
			break
		}
	}

	if len(updates) < 3 {
		t.Fatalf("expected multiple progress updates, got: %v", updates)
	}
	if updates[0] != "10%" || updates[1] != "50%" || updates[len(updates)-1] != "complete" {
		t.Fatalf("unexpected streamed progress updates: %v", updates)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}

func containsSkipped(delta string) bool {
	return delta == "tool_error: Skipped due to queued user message."
}
