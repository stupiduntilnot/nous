package main

import (
	"bufio"
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

	fmt.Println("oh-my-agent minimal tui")
	fmt.Printf("socket: %s\n", socket)

	if checkConnected(socket) {
		fmt.Println("status: connected")
	} else {
		fmt.Println("status: disconnected")
	}

	fmt.Println("commands: ping, quit")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("tui> ")
		if !scanner.Scan() {
			fmt.Println()
			return
		}
		line := strings.TrimSpace(scanner.Text())
		switch line {
		case "":
			continue
		case "quit", "exit":
			return
		case "ping":
			resp, err := ipc.SendCommand(socket, protocol.Envelope{
				ID:      fmt.Sprintf("tui-ping-%d", time.Now().UnixNano()),
				Type:    string(protocol.CmdPing),
				Payload: map[string]any{},
			})
			if err != nil {
				fmt.Printf("error: %v\n", err)
				continue
			}
			if resp.OK {
				fmt.Println("pong")
			} else if resp.Error != nil {
				fmt.Printf("error: %s: %s\n", resp.Error.Code, resp.Error.Message)
			} else {
				fmt.Println("error: unknown")
			}
		default:
			fmt.Printf("unknown command: %s\n", line)
		}
	}
}

func checkConnected(socket string) bool {
	conn, err := net.DialTimeout("unix", socket, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
