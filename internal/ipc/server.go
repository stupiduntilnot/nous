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
	"time"

	"oh-my-agent/internal/core"
	"oh-my-agent/internal/provider"
	"oh-my-agent/internal/protocol"
	"oh-my-agent/internal/session"
)

type Server struct {
	socketPath string
	listener   net.Listener
	wg         sync.WaitGroup
	sessions   *session.Manager
	engine     *core.Engine
	loop       *core.CommandLoop
	timeout    time.Duration

	dispatchOverride func(protocol.Envelope) protocol.ResponseEnvelope
}

func NewServer(socketPath string) *Server {
	return &Server{socketPath: socketPath, timeout: 3 * time.Second}
}

func (s *Server) SetEngine(engine *core.Engine, loop *core.CommandLoop) {
	s.engine = engine
	s.loop = loop
}

func (s *Server) SetSessionManager(mgr *session.Manager) {
	s.sessions = mgr
}

func (s *Server) Serve(ctx context.Context) error {
	if err := ensureSocketDir(s.socketPath); err != nil {
		return err
	}
	if s.sessions == nil {
		sessDir := filepath.Join(filepath.Dir(s.socketPath), "sessions")
		mgr, err := session.NewManager(sessDir)
		if err != nil {
			return fmt.Errorf("init session manager: %w", err)
		}
		s.sessions = mgr
	}
	if s.engine == nil {
		s.engine = core.NewEngine(core.NewRuntime(), provider.NewMockAdapter())
	}
	if s.loop == nil {
		s.loop = core.NewCommandLoop(s.engine)
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
			_ = writeError(conn, "", "invalid_command", decErr.Error())
			continue
		}

		respCh := make(chan protocol.ResponseEnvelope, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					respCh <- protocol.ResponseEnvelope{
						Envelope: protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "error", Payload: map[string]any{}},
						OK:       false,
						Error:    &protocol.ErrorBody{Code: "internal_error", Message: fmt.Sprintf("panic: %v", r)},
					}
				}
			}()
			respCh <- s.dispatch(env)
		}()

		select {
		case resp := <-respCh:
			_ = writeResponse(conn, resp)
		case <-time.After(s.timeout):
			_ = writeError(conn, env.ID, "timeout", "command timed out")
		}
	}
}

func (s *Server) dispatch(env protocol.Envelope) protocol.ResponseEnvelope {
	if s.dispatchOverride != nil {
		return s.dispatchOverride(env)
	}

	switch protocol.CommandType(env.Type) {
		case protocol.CmdPing:
			return responseOK(protocol.Envelope{
				V:       protocol.Version,
				ID:      env.ID,
				Type:    "pong",
				Payload: map[string]any{"message": "pong"},
			})
		case protocol.CmdNewSession:
			id, err := s.sessions.NewSession()
			if err != nil {
				return responseErr(env.ID, "session_error", err.Error())
			}
			return responseOK(protocol.Envelope{
				V:    protocol.Version,
				ID:   env.ID,
				Type: "session",
				Payload: map[string]any{
					"session_id": id,
					"active":     true,
				},
			})
		case protocol.CmdSwitchSession:
			rawID, ok := env.Payload["session_id"].(string)
			if !ok || rawID == "" {
				return responseErr(env.ID, "invalid_payload", "session_id is required")
			}
			if err := s.sessions.SwitchSession(rawID); err != nil {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			return responseOK(protocol.Envelope{
				V:    protocol.Version,
				ID:   env.ID,
				Type: "session",
				Payload: map[string]any{
					"session_id": rawID,
					"active":     true,
				},
			})
		case protocol.CmdPrompt:
			text, ok := env.Payload["text"].(string)
			if !ok || text == "" {
				return responseErr(env.ID, "invalid_payload", "text is required")
			}
			wait, _ := env.Payload["wait"].(bool)
			if wait {
				return s.promptSync(env.ID, text)
			}
			if err := s.loop.Prompt(text); err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseOK(protocol.Envelope{
				V:    protocol.Version,
				ID:   env.ID,
				Type: "accepted",
				Payload: map[string]any{
					"command": "prompt",
				},
			})
		case protocol.CmdSteer:
			text, ok := env.Payload["text"].(string)
			if !ok || text == "" {
				return responseErr(env.ID, "invalid_payload", "text is required")
			}
			if err := s.loop.Steer(text); err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: map[string]any{"command": "steer"}})
		case protocol.CmdFollowUp:
			text, ok := env.Payload["text"].(string)
			if !ok || text == "" {
				return responseErr(env.ID, "invalid_payload", "text is required")
			}
			if err := s.loop.FollowUp(text); err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: map[string]any{"command": "follow_up"}})
		case protocol.CmdAbort:
			if err := s.loop.Abort(); err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: map[string]any{"command": "abort"}})
		case protocol.CmdSetActiveTools:
			raw, ok := env.Payload["tools"].([]any)
			if !ok {
				return responseErr(env.ID, "invalid_payload", "tools must be an array")
			}
			tools := make([]string, 0, len(raw))
			valid := true
			for _, item := range raw {
				name, ok := item.(string)
				if !ok || name == "" {
					valid = false
					break
				}
				tools = append(tools, name)
			}
			if !valid {
				return responseErr(env.ID, "invalid_payload", "tools must be string array")
			}
			if err := s.engine.SetActiveTools(tools); err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseOK(protocol.Envelope{
				V:       protocol.Version,
				ID:      env.ID,
				Type:    "accepted",
				Payload: map[string]any{"command": "set_active_tools", "count": len(tools)},
			})
		default:
			return responseErr(env.ID, "not_implemented", "command not implemented yet")
		}
}

func (s *Server) promptSync(reqID, text string) protocol.ResponseEnvelope {
	events := make([]core.Event, 0, 16)
	unsub := s.engine.Subscribe(func(ev core.Event) {
		events = append(events, ev)
	})
	defer unsub()

	runID := fmt.Sprintf("sync-%d", time.Now().UnixNano())
	out, err := s.engine.Prompt(context.Background(), runID, text)
	if err != nil {
		return responseErr(reqID, "provider_error", err.Error())
	}
	return responseOK(protocol.Envelope{
		V:    protocol.Version,
		ID:   reqID,
		Type: "result",
		Payload: map[string]any{
			"output": out,
			"events": events,
		},
	})
}

func responseOK(env protocol.Envelope) protocol.ResponseEnvelope {
	return protocol.ResponseEnvelope{Envelope: env, OK: true}
}

func writeError(conn net.Conn, id, code, message string) error {
	return writeResponse(conn, responseErr(id, code, message))
}

func responseErr(id, code, message string) protocol.ResponseEnvelope {
	return protocol.ResponseEnvelope{
		Envelope: protocol.Envelope{V: protocol.Version, ID: id, Type: "error", Payload: map[string]any{}},
		OK:       false,
		Error:    &protocol.ErrorBody{Code: code, Message: message},
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
