package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"oh-my-agent/internal/ipc"
	"oh-my-agent/internal/protocol"
)

func main() {
	socket := "/tmp/pi-core.sock"
	if len(os.Args) > 1 {
		socket = os.Args[1]
	}

	fmt.Println("oh-my-agent tui mvp")
	fmt.Printf("socket: %s\n", socket)
	printStatus(socket)
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
			return
		}
		if cmd == "" {
			if line == "help" {
				printHelp()
				continue
			}
			if line == "status" {
				printStatus(socket)
				continue
			}
			fmt.Printf("unknown command: %s\n", line)
			continue
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
			if resp.Type == "result" {
				renderResult(resp.Payload)
				continue
			}
			fmt.Printf("ok: type=%s payload=%v\n", resp.Type, resp.Payload)
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
		return string(protocol.CmdPrompt), map[string]any{"text": text, "wait": true}, false, nil
	case strings.HasPrefix(line, "prompt_async "):
		text := strings.TrimSpace(strings.TrimPrefix(line, "prompt_async "))
		if text == "" {
			return "", nil, false, fmt.Errorf("prompt_async text is required")
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
			return "", nil, false, fmt.Errorf("parent session id is required")
		}
		return string(protocol.CmdBranchSession), map[string]any{"parent_id": id}, false, nil
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
	fmt.Println("  prompt_async <text>")
	fmt.Println("  steer <text>")
	fmt.Println("  follow_up <text>")
	fmt.Println("  abort")
	fmt.Println("  new")
	fmt.Println("  switch <session_id>")
	fmt.Println("  branch <parent_session_id>")
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
		tp, _ := ev["type"].(string)
		switch tp {
		case "message_update":
			if delta, ok := ev["delta"].(string); ok && delta != "" {
				fmt.Printf("message: %s\n", delta)
			}
		case "tool_execution_start", "tool_execution_update", "tool_execution_end":
			fmt.Printf("tool: %s %v\n", tp, ev)
		case "agent_start", "agent_end", "turn_start", "turn_end":
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
}

func printStatus(socket string) {
	if checkConnected(socket) {
		fmt.Println("status: connected")
		return
	}
	fmt.Println("status: disconnected")
}

func checkConnected(socket string) bool {
	conn, err := net.DialTimeout("unix", socket, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
