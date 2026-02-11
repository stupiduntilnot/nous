package ipc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"oh-my-agent/internal/protocol"
)

func SendCommand(socketPath string, cmd protocol.Envelope) (protocol.ResponseEnvelope, error) {
	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		return protocol.ResponseEnvelope{}, fmt.Errorf("dial uds: %w", err)
	}
	defer conn.Close()

	if cmd.V == "" {
		cmd.V = protocol.Version
	}
	if cmd.Payload == nil {
		cmd.Payload = map[string]any{}
	}

	b, err := json.Marshal(cmd)
	if err != nil {
		return protocol.ResponseEnvelope{}, fmt.Errorf("marshal command: %w", err)
	}
	b = append(b, '\n')
	if _, err := conn.Write(b); err != nil {
		return protocol.ResponseEnvelope{}, fmt.Errorf("write command: %w", err)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return protocol.ResponseEnvelope{}, fmt.Errorf("read response: %w", err)
	}

	var resp protocol.ResponseEnvelope
	if err := json.Unmarshal(line, &resp); err != nil {
		return protocol.ResponseEnvelope{}, fmt.Errorf("decode response: %w", err)
	}
	return resp, nil
}
