package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"oh-my-agent/internal/ipc"
	"oh-my-agent/internal/protocol"
)

func main() {
	socket := flag.String("socket", "/tmp/pi-core.sock", "uds socket path")
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

	resp, err := ipc.SendCommand(*socket, protocol.Envelope{
		ID:      fmt.Sprintf("corectl-%d", time.Now().UnixNano()),
		Type:    cmd,
		Payload: payload,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "request failed: %v\n", err)
		os.Exit(1)
	}
	if !resp.OK {
		if resp.Error != nil {
			fmt.Fprintf(os.Stderr, "%s: %s\n", resp.Error.Code, resp.Error.Message)
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
	case "set_active_tools":
		if len(args) < 2 {
			return "", nil, fmt.Errorf("set_active_tools requires at least one tool name")
		}
		tools := make([]any, 0, len(args)-1)
		for _, t := range args[1:] {
			tools = append(tools, t)
		}
		return string(protocol.CmdSetActiveTools), map[string]any{"tools": tools}, nil
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
	fmt.Fprintln(os.Stderr, "  steer <text>")
	fmt.Fprintln(os.Stderr, "  follow_up <text>")
	fmt.Fprintln(os.Stderr, "  abort")
	fmt.Fprintln(os.Stderr, "  new")
	fmt.Fprintln(os.Stderr, "  switch <session_id>")
	fmt.Fprintln(os.Stderr, "  set_active_tools <tool...>")
}
