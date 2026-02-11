package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"oh-my-agent/internal/ipc"
	"oh-my-agent/internal/protocol"
)

func main() {
	socket := flag.String("socket", "/tmp/pi-core.sock", "uds socket path")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: corectl [--socket path] ping")
		os.Exit(2)
	}

	switch args[0] {
	case "ping":
		runPing(*socket)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(2)
	}
}

func runPing(socketPath string) {
	resp, err := ipc.SendCommand(socketPath, protocol.Envelope{
		ID:      fmt.Sprintf("ping-%d", time.Now().UnixNano()),
		Type:    string(protocol.CmdPing),
		Payload: map[string]any{},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ping failed: %v\n", err)
		os.Exit(1)
	}
	if !resp.OK {
		if resp.Error != nil {
			fmt.Fprintf(os.Stderr, "ping error: %s: %s\n", resp.Error.Code, resp.Error.Message)
		} else {
			fmt.Fprintln(os.Stderr, "ping error: unknown")
		}
		os.Exit(1)
	}
	fmt.Println("pong")
}
