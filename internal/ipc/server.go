package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"

	"oh-my-agent/internal/protocol"
)

type Server struct {
	socketPath string
	listener   net.Listener
	wg         sync.WaitGroup
}

func NewServer(socketPath string) *Server {
	return &Server{socketPath: socketPath}
}

func (s *Server) Serve(ctx context.Context) error {
	if err := ensureSocketDir(s.socketPath); err != nil {
		return err
	}
	_ = os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen uds: %w", err)
	}
	s.listener = ln

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				break
			}
			continue
		}
		s.wg.Add(1)
		go s.handleConn(conn)
	}

	s.wg.Wait()
	return nil
}

func (s *Server) Close() error {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}

		env, decErr := protocol.DecodeCommand(line)
		if decErr != nil {
			_ = writeResponse(conn, protocol.ResponseEnvelope{
				Envelope: protocol.Envelope{V: protocol.Version, ID: "", Type: "error", Payload: map[string]any{}},
				OK:       false,
				Error:    &protocol.ErrorBody{Code: "invalid_command", Message: decErr.Error()},
			})
			continue
		}

		switch protocol.CommandType(env.Type) {
		case protocol.CmdPing:
			_ = writeResponse(conn, protocol.ResponseEnvelope{
				Envelope: protocol.Envelope{
					V:       protocol.Version,
					ID:      env.ID,
					Type:    "pong",
					Payload: map[string]any{"message": "pong"},
				},
				OK: true,
			})
		default:
			_ = writeResponse(conn, protocol.ResponseEnvelope{
				Envelope: protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "error", Payload: map[string]any{}},
				OK:       false,
				Error:    &protocol.ErrorBody{Code: "not_implemented", Message: "command not implemented yet"},
			})
		}
	}
}

func writeResponse(conn net.Conn, resp protocol.ResponseEnvelope) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = conn.Write(b)
	return err
}

func ensureSocketDir(socketPath string) error {
	dir := filepath.Dir(socketPath)
	return os.MkdirAll(dir, 0o755)
}
