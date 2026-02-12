package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"nous/internal/ipc"
	"nous/internal/protocol"
)

func main() {
	socket := "/tmp/nous-core.sock"
	if len(os.Args) > 1 {
		socket = os.Args[1]
	}
	activeSession := ""
	queue := &runQueueState{}
	var runDone <-chan struct{}

	fmt.Println("nous tui mvp")
	fmt.Printf("socket: %s\n", socket)
	printStatus(socket, activeSession, queue)
	printHelp()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("tui> ")
		if !scanner.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		cmd, payload, quit, err := parseInput(line)
		if err != nil {
			fmt.Printf("error: %v\n", err)
			continue
		}
		if quit {
			if runDone != nil {
				select {
				case <-runDone:
				case <-time.After(3 * time.Second):
				}
			}
			return
		}
		if cmd == "" {
			if line == "help" {
				printHelp()
				continue
			}
			if line == "status" {
				printStatus(socket, activeSession, queue)
				continue
			}
			fmt.Printf("unknown command: %s\n", line)
			continue
		}
		if cmd == string(protocol.CmdPrompt) {
			if runDone != nil && channelClosed(runDone) {
				runDone = nil
			}
			if runDone == nil && queue.Snapshot().ActiveRunID == "" {
				done := make(chan struct{})
				runDone = done
				go func() {
					defer close(done)
					if err := streamRunEvents(socket, "", func(env protocol.Envelope) {
						evRunID, _ := env.Payload["run_id"].(string)
						ev := make(map[string]any, len(env.Payload)+1)
						ev["type"] = env.Type
						for k, v := range env.Payload {
							ev[k] = v
						}
						renderEvent(ev)
						if snap, changed := queue.MarkEvent(env.Type, evRunID); changed {
							printQueueSnapshot(snap)
						}
					}); err != nil {
						fmt.Printf("event stream error: %v\n", err)
					}
				}()
			}
		}

		resp, sendErr := ipc.SendCommand(socket, protocol.Envelope{
			ID:      fmt.Sprintf("tui-%s-%d", cmd, time.Now().UnixNano()),
			Type:    cmd,
			Payload: payload,
		})
		if sendErr != nil {
			fmt.Printf("transport error: %v\n", sendErr)
			continue
		}
		if resp.OK {
			if sid := extractSessionID(resp.Payload); sid != "" {
				activeSession = sid
			}
			if resp.Type == "accepted" {
				if cmdName, _ := resp.Payload["command"].(string); cmdName == "prompt" {
					runID, _ := resp.Payload["run_id"].(string)
					if runID == "" {
						fmt.Printf("ok: type=%s payload=%v\n", resp.Type, resp.Payload)
						continue
					}
					queue.Activate(runID)
					printQueueSnapshot(queue.Snapshot())
					continue
				}
				if cmdName, _ := resp.Payload["command"].(string); cmdName == "steer" || cmdName == "follow_up" {
					runID, _ := resp.Payload["run_id"].(string)
					if snap, changed := queue.MarkAccepted(cmdName, runID); changed {
						printQueueSnapshot(snap)
					}
				}
			}
			if resp.Type == "result" {
				renderResult(resp.Payload)
				if activeSession != "" {
					fmt.Printf("session: %s\n", activeSession)
				}
				continue
			}
			fmt.Printf("ok: type=%s payload=%v\n", resp.Type, resp.Payload)
			if activeSession != "" && (resp.Type == "session" || resp.Type == "result") {
				fmt.Printf("session: %s\n", activeSession)
			}
			continue
		}
		if resp.Error != nil {
			fmt.Printf("error: %s: %s\n", resp.Error.Code, resp.Error.Message)
			continue
		}
		fmt.Println("error: unknown")
	}
}

func parseInput(line string) (cmd string, payload map[string]any, quit bool, err error) {
	switch {
	case line == "quit" || line == "exit":
		return "", nil, true, nil
	case line == "help" || line == "status":
		return "", nil, false, nil
	case line == "ping":
		return string(protocol.CmdPing), map[string]any{}, false, nil
	case line == "abort":
		return string(protocol.CmdAbort), map[string]any{}, false, nil
	case line == "new":
		return string(protocol.CmdNewSession), map[string]any{}, false, nil
	case line == "set_active_tools":
		return string(protocol.CmdSetActiveTools), map[string]any{"tools": []any{}}, false, nil
	case strings.HasPrefix(line, "prompt "):
		text := strings.TrimSpace(strings.TrimPrefix(line, "prompt "))
		if text == "" {
			return "", nil, false, fmt.Errorf("prompt text is required")
		}
		return string(protocol.CmdPrompt), map[string]any{"text": text, "wait": false}, false, nil
	case strings.HasPrefix(line, "steer "):
		text := strings.TrimSpace(strings.TrimPrefix(line, "steer "))
		if text == "" {
			return "", nil, false, fmt.Errorf("steer text is required")
		}
		return string(protocol.CmdSteer), map[string]any{"text": text}, false, nil
	case strings.HasPrefix(line, "follow_up "):
		text := strings.TrimSpace(strings.TrimPrefix(line, "follow_up "))
		if text == "" {
			return "", nil, false, fmt.Errorf("follow_up text is required")
		}
		return string(protocol.CmdFollowUp), map[string]any{"text": text}, false, nil
	case strings.HasPrefix(line, "switch "):
		id := strings.TrimSpace(strings.TrimPrefix(line, "switch "))
		if id == "" {
			return "", nil, false, fmt.Errorf("session id is required")
		}
		return string(protocol.CmdSwitchSession), map[string]any{"session_id": id}, false, nil
	case strings.HasPrefix(line, "branch "):
		id := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
		if id == "" {
			return "", nil, false, fmt.Errorf("session id is required")
		}
		return string(protocol.CmdBranchSession), map[string]any{"session_id": id}, false, nil
	case strings.HasPrefix(line, "set_active_tools "):
		rest := strings.TrimSpace(strings.TrimPrefix(line, "set_active_tools "))
		if rest == "" {
			return string(protocol.CmdSetActiveTools), map[string]any{"tools": []any{}}, false, nil
		}
		parts := strings.Fields(rest)
		tools := make([]any, 0, len(parts))
		for _, p := range parts {
			tools = append(tools, p)
		}
		return string(protocol.CmdSetActiveTools), map[string]any{"tools": tools}, false, nil
	case strings.HasPrefix(line, "ext "):
		rest := strings.TrimSpace(strings.TrimPrefix(line, "ext "))
		if rest == "" {
			return "", nil, false, fmt.Errorf("extension command name is required")
		}
		name := rest
		payload := map[string]any{}
		if idx := strings.IndexByte(rest, ' '); idx >= 0 {
			name = strings.TrimSpace(rest[:idx])
			raw := strings.TrimSpace(rest[idx+1:])
			if raw != "" {
				if err := json.Unmarshal([]byte(raw), &payload); err != nil {
					return "", nil, false, fmt.Errorf("extension payload must be JSON object: %w", err)
				}
			}
		}
		if name == "" {
			return "", nil, false, fmt.Errorf("extension command name is required")
		}
		return string(protocol.CmdExtensionCmd), map[string]any{"name": name, "payload": payload}, false, nil
	default:
		return "", nil, false, nil
	}
}

func printHelp() {
	fmt.Println("commands:")
	fmt.Println("  ping")
	fmt.Println("  prompt <text>")
	fmt.Println("  steer <text>")
	fmt.Println("  follow_up <text>")
	fmt.Println("  abort")
	fmt.Println("  new")
	fmt.Println("  switch <session_id>")
	fmt.Println("  branch <session_id>")
	fmt.Println("  set_active_tools [tool...]   (no args = clear all)")
	fmt.Println("  ext <name> [json_payload]")
	fmt.Println("  status")
	fmt.Println("  help")
	fmt.Println("  quit")
}

func renderResult(payload map[string]any) {
	if out, ok := payload["output"].(string); ok {
		fmt.Printf("assistant: %s\n", out)
	}
	rawEvents, _ := payload["events"].([]any)
	for _, item := range rawEvents {
		ev, ok := item.(map[string]any)
		if !ok {
			continue
		}
		renderEvent(ev)
	}
}

func renderEvent(ev map[string]any) {
	tp, _ := ev["type"].(string)
	runID, _ := ev["run_id"].(string)
	turnID, _ := ev["turn_id"].(string)
	toolName, _ := ev["tool_name"].(string)
	switch tp {
	case "message_update":
		if delta, ok := ev["delta"].(string); ok && delta != "" {
			fmt.Printf("assistant: %s\n", delta)
		}
	case "tool_execution_start", "tool_execution_update", "tool_execution_end":
		delta, _ := ev["delta"].(string)
		if delta != "" {
			fmt.Printf("tool: %s name=%s run=%s turn=%s delta=%s\n", tp, toolName, runID, turnID, delta)
			break
		}
		fmt.Printf("tool: %s name=%s run=%s turn=%s\n", tp, toolName, runID, turnID)
	case "agent_start", "agent_end", "turn_start", "turn_end":
		if runID != "" || turnID != "" {
			fmt.Printf("status: %s run=%s turn=%s\n", tp, runID, turnID)
			break
		}
		fmt.Printf("status: %s\n", tp)
	case "status":
		if msg, ok := ev["message"].(string); ok && msg != "" {
			fmt.Printf("status: %s\n", msg)
		}
	case "warning":
		code, _ := ev["code"].(string)
		msg, _ := ev["message"].(string)
		if code != "" || msg != "" {
			fmt.Printf("warning: %s %s\n", code, msg)
		}
	case "error":
		code, _ := ev["code"].(string)
		msg, _ := ev["message"].(string)
		cause, _ := ev["cause"].(string)
		if cause != "" {
			fmt.Printf("error: %s %s cause=%s\n", code, msg, cause)
			break
		}
		if code != "" || msg != "" {
			fmt.Printf("error: %s %s\n", code, msg)
		}
	}
}

func streamRunEvents(socket, runID string, onEvent func(protocol.Envelope)) error {
	conn, err := net.DialTimeout("unix", socket+".events", 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("connect event stream: %w", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	targetRunID := runID
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			if err == io.EOF {
				return fmt.Errorf("event stream closed before agent_end")
			}
			return fmt.Errorf("read event stream: %w", err)
		}
		var env protocol.Envelope
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		evRunID, _ := env.Payload["run_id"].(string)
		if evRunID == "" {
			continue
		}
		if targetRunID == "" {
			targetRunID = evRunID
		}
		if evRunID != targetRunID {
			continue
		}
		onEvent(env)
		if env.Type == string(protocol.EvAgentEnd) {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for run end: %s", targetRunID)
}

func printStatus(socket, activeSession string, queue *runQueueState) {
	if checkConnected(socket) {
		fmt.Println("status: connected")
	} else {
		fmt.Println("status: disconnected")
	}
	if activeSession == "" {
		fmt.Println("session: (none)")
		printQueueSnapshot(queue.Snapshot())
		return
	}
	fmt.Printf("session: %s\n", activeSession)
	printQueueSnapshot(queue.Snapshot())
}

func checkConnected(socket string) bool {
	conn, err := net.DialTimeout("unix", socket, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func extractSessionID(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	sid, _ := payload["session_id"].(string)
	return sid
}

type runQueueSnapshot struct {
	ActiveRunID     string
	PendingSteer    int
	PendingFollowUp int
}

type runQueueState struct {
	mu              sync.Mutex
	activeRunID     string
	pendingSteer    int
	pendingFollowUp int
}

func (q *runQueueState) Activate(runID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.activeRunID = runID
	q.pendingSteer = 0
	q.pendingFollowUp = 0
}

func (q *runQueueState) Snapshot() runQueueSnapshot {
	q.mu.Lock()
	defer q.mu.Unlock()
	return runQueueSnapshot{
		ActiveRunID:     q.activeRunID,
		PendingSteer:    q.pendingSteer,
		PendingFollowUp: q.pendingFollowUp,
	}
}

func (q *runQueueState) MarkAccepted(command, runID string) (runQueueSnapshot, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if runID == "" || runID != q.activeRunID {
		return runQueueSnapshot{}, false
	}
	switch command {
	case "steer":
		q.pendingSteer++
	case "follow_up":
		q.pendingFollowUp++
	default:
		return runQueueSnapshot{}, false
	}
	return runQueueSnapshot{
		ActiveRunID:     q.activeRunID,
		PendingSteer:    q.pendingSteer,
		PendingFollowUp: q.pendingFollowUp,
	}, true
}

func (q *runQueueState) MarkEvent(eventType, runID string) (runQueueSnapshot, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if runID == "" {
		return runQueueSnapshot{}, false
	}
	changed := false
	if q.activeRunID == "" && eventType == string(protocol.EvAgentStart) {
		q.activeRunID = runID
		changed = true
	}
	if runID != q.activeRunID {
		return runQueueSnapshot{}, false
	}
	switch eventType {
	case string(protocol.EvTurnStart):
		if q.pendingSteer > 0 {
			q.pendingSteer--
			changed = true
		} else if q.pendingFollowUp > 0 {
			q.pendingFollowUp--
			changed = true
		}
	case string(protocol.EvAgentEnd):
		q.activeRunID = ""
		q.pendingSteer = 0
		q.pendingFollowUp = 0
		changed = true
	}
	if !changed {
		return runQueueSnapshot{}, false
	}
	return runQueueSnapshot{
		ActiveRunID:     q.activeRunID,
		PendingSteer:    q.pendingSteer,
		PendingFollowUp: q.pendingFollowUp,
	}, true
}

func printQueueSnapshot(s runQueueSnapshot) {
	if s.ActiveRunID == "" {
		fmt.Println("queue: idle")
		return
	}
	fmt.Printf("queue: run=%s steer=%d follow_up=%d\n", s.ActiveRunID, s.PendingSteer, s.PendingFollowUp)
}

func channelClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
