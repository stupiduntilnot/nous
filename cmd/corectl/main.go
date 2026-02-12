package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"nous/internal/ipc"
	"nous/internal/protocol"
)

func main() {
	socket := flag.String("socket", "/tmp/nous-core.sock", "uds socket path")
	requestTimeout := flag.Duration("request-timeout", 30*time.Second, "request timeout (e.g. 30s, 500ms)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	cmd, payload, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid args: %v\n", err)
		usage()
		os.Exit(2)
	}
	if cmd == "__trace__" {
		runID, _ := payload["run_id"].(string)
		events, err := ipc.CaptureRunTrace(*socket, runID, *requestTimeout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "trace failed: %v\n", err)
			os.Exit(1)
		}
		var buf bytes.Buffer
		if err := ipc.WriteTraceNDJSON(&buf, events); err != nil {
			fmt.Fprintf(os.Stderr, "trace encode failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(buf.String())
		return
	}

	resp, err := ipc.SendCommandWithTimeout(*socket, protocol.Envelope{
		ID:      fmt.Sprintf("corectl-%d", time.Now().UnixNano()),
		Type:    cmd,
		Payload: payload,
	}, *requestTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	if !resp.OK {
		if resp.Error != nil {
			fmt.Fprintln(os.Stderr, formatError(resp.Error))
		} else {
			fmt.Fprintln(os.Stderr, "request failed")
		}
		os.Exit(1)
	}
	printResponse(resp)
}

func parseArgs(args []string) (cmd string, payload map[string]any, err error) {
	switch args[0] {
	case "ping":
		return string(protocol.CmdPing), map[string]any{}, nil
	case "prompt":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("prompt requires text")
		}
		return string(protocol.CmdPrompt), map[string]any{"text": strings.Join(args[1:], " "), "wait": true}, nil
	case "prompt_async":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("prompt_async requires text")
		}
		return string(protocol.CmdPrompt), map[string]any{"text": strings.Join(args[1:], " "), "wait": false}, nil
	case "trace":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("trace requires run_id")
		}
		return "__trace__", map[string]any{"run_id": args[1]}, nil
	case "steer":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("steer requires text")
		}
		return string(protocol.CmdSteer), map[string]any{"text": strings.Join(args[1:], " ")}, nil
	case "follow_up":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("follow_up requires text")
		}
		return string(protocol.CmdFollowUp), map[string]any{"text": strings.Join(args[1:], " ")}, nil
	case "abort":
		return string(protocol.CmdAbort), map[string]any{}, nil
	case "new":
		return string(protocol.CmdNewSession), map[string]any{}, nil
	case "switch":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("switch requires session id")
		}
		return string(protocol.CmdSwitchSession), map[string]any{"session_id": args[1]}, nil
	case "branch":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("branch requires parent session id")
		}
		return string(protocol.CmdBranchSession), map[string]any{"session_id": args[1]}, nil
	case "set_active_tools":
		if len(args) == 1 {
			return string(protocol.CmdSetActiveTools), map[string]any{"tools": []any{}}, nil
		}
		tools := make([]any, 0, len(args)-1)
		for _, t := range args[1:] {
			tools = append(tools, t)
		}
		return string(protocol.CmdSetActiveTools), map[string]any{"tools": tools}, nil
	case "set_steering_mode":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("set_steering_mode requires mode (one-at-a-time|all)")
		}
		return string(protocol.CmdSetSteeringMode), map[string]any{"mode": args[1]}, nil
	case "set_follow_up_mode":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("set_follow_up_mode requires mode (one-at-a-time|all)")
		}
		return string(protocol.CmdSetFollowUpMode), map[string]any{"mode": args[1]}, nil
	case "get_state":
		if len(args) != 1 {
			return "", nil, fmt.Errorf("get_state does not take arguments")
		}
		return string(protocol.CmdGetState), map[string]any{}, nil
	case "get_messages":
		if len(args) > 2 {
			return "", nil, fmt.Errorf("get_messages takes at most one optional session id")
		}
		payload := map[string]any{}
		if len(args) == 2 {
			payload["session_id"] = args[1]
		}
		return string(protocol.CmdGetMessages), payload, nil
	case "ext":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("ext requires command name")
		}
		name := args[1]
		payload := map[string]any{}
		if len(args) >= 3 {
			if err := json.Unmarshal([]byte(args[2]), &payload); err != nil {
				return "", nil, fmt.Errorf("ext payload must be JSON object: %w", err)
			}
		}
		return string(protocol.CmdExtensionCmd), map[string]any{"name": name, "payload": payload}, nil
	default:
		return "", nil, fmt.Errorf("unknown command: %s", args[0])
	}
}

func printResponse(resp protocol.ResponseEnvelope) {
	if resp.Type == "pong" {
		fmt.Println("pong")
		return
	}
	b, err := json.MarshalIndent(resp.Payload, "", "  ")
	if err != nil {
		fmt.Printf("ok: %v\n", resp.Payload)
		return
	}
	fmt.Println(string(b))
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: corectl [--socket path] <command>")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  ping")
	fmt.Fprintln(os.Stderr, "  prompt <text>")
	fmt.Fprintln(os.Stderr, "  prompt_async <text>")
	fmt.Fprintln(os.Stderr, "  trace <run_id>")
	fmt.Fprintln(os.Stderr, "  steer <text>")
	fmt.Fprintln(os.Stderr, "  follow_up <text>")
	fmt.Fprintln(os.Stderr, "  abort")
	fmt.Fprintln(os.Stderr, "  new")
	fmt.Fprintln(os.Stderr, "  switch <session_id>")
	fmt.Fprintln(os.Stderr, "  branch <session_id>")
	fmt.Fprintln(os.Stderr, "  set_active_tools [tool...]   (no args = clear all)")
	fmt.Fprintln(os.Stderr, "  set_steering_mode <one-at-a-time|all>")
	fmt.Fprintln(os.Stderr, "  set_follow_up_mode <one-at-a-time|all>")
	fmt.Fprintln(os.Stderr, "  get_state")
	fmt.Fprintln(os.Stderr, "  get_messages [session_id]")
	fmt.Fprintln(os.Stderr, "  ext <name> [json_payload]")
}

func formatError(err *protocol.ErrorBody) string {
	if err == nil {
		return "request failed"
	}
	if err.Cause == "" {
		return fmt.Sprintf("%s: %s", err.Code, err.Message)
	}
	return fmt.Sprintf("%s: %s (%s)", err.Code, err.Message, err.Cause)
}
