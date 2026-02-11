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
	"strings"
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

type sessionMessageRecord struct {
	Type      string `json:"type"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	RunID     string `json:"run_id,omitempty"`
	TurnKind  string `json:"turn_kind,omitempty"`
	CreatedAt string `json:"created_at"`
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
	s.loop.SetOnTurnEnd(func(r core.TurnResult) {
		_ = s.appendTurnRecord(r.RunID, string(r.Kind), r.Input, r.Output)
	})
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
		case protocol.CmdBranchSession:
			rawParentID, ok := env.Payload["parent_id"].(string)
			if !ok || rawParentID == "" {
				return responseErr(env.ID, "invalid_payload", "parent_id is required")
			}
			id, err := s.sessions.BranchFrom(rawParentID)
			if err != nil {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			return responseOK(protocol.Envelope{
				V:    protocol.Version,
				ID:   env.ID,
				Type: "session",
				Payload: map[string]any{
					"session_id": id,
					"parent_id":  rawParentID,
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
	sessionID, err := s.ensureActiveSession()
	if err != nil {
		return responseErr(reqID, "session_error", err.Error())
	}
	promptWithContext, err := s.promptWithSessionContext(sessionID, text)
	if err != nil {
		return responseErr(reqID, "session_error", err.Error())
	}

	events := make([]core.Event, 0, 16)
	unsub := s.engine.Subscribe(func(ev core.Event) {
		events = append(events, ev)
	})
	defer unsub()

	runID := fmt.Sprintf("sync-%d", time.Now().UnixNano())
	out, err := s.engine.Prompt(context.Background(), runID, promptWithContext)
	if err != nil {
		return responseErr(reqID, "provider_error", err.Error())
	}
	if err := s.appendTurnRecord(runID, string(core.TurnPrompt), text, out); err != nil {
		return responseErr(reqID, "session_error", err.Error())
	}
	return responseOK(protocol.Envelope{
		V:    protocol.Version,
		ID:   reqID,
		Type: "result",
		Payload: map[string]any{
			"output":     out,
			"events":     events,
			"session_id": sessionID,
		},
	})
}

func (s *Server) ensureActiveSession() (string, error) {
	if s.sessions == nil {
		return "", fmt.Errorf("session_manager_not_ready")
	}
	if id := s.sessions.ActiveSession(); id != "" {
		return id, nil
	}
	return s.sessions.NewSession()
}

func (s *Server) appendTurnRecord(runID, kind, input, output string) error {
	_, err := s.ensureActiveSession()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.sessions.Append(sessionMessageRecord{
		Type:      "message",
		Role:      "user",
		Text:      input,
		RunID:     runID,
		TurnKind:  kind,
		CreatedAt: now,
	}); err != nil {
		return err
	}
	return s.sessions.Append(sessionMessageRecord{
		Type:      "message",
		Role:      "assistant",
		Text:      output,
		RunID:     runID,
		TurnKind:  kind,
		CreatedAt: now,
	})
}

func (s *Server) promptWithSessionContext(sessionID, prompt string) (string, error) {
	records, err := s.sessions.BuildContext(sessionID)
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return prompt, nil
	}

	lines := make([]string, 0, len(records))
	for _, raw := range records {
		var rec sessionMessageRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		if rec.Type != "message" || rec.Role == "" || strings.TrimSpace(rec.Text) == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", rec.Role, rec.Text))
	}
	if len(lines) == 0 {
		return prompt, nil
	}
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}

	var b strings.Builder
	b.WriteString("Conversation so far:\n")
	for _, line := range lines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("user: ")
	b.WriteString(prompt)
	return b.String(), nil
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
