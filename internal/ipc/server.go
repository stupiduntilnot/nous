package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"nous/internal/core"
	"nous/internal/extension"
	"nous/internal/protocol"
	"nous/internal/provider"
	"nous/internal/session"
)

type Server struct {
	socketPath      string
	eventSocketPath string
	listener        net.Listener
	eventListener   net.Listener
	wg              sync.WaitGroup
	sessions        *session.Manager
	engine          *core.Engine
	loop            *core.CommandLoop
	timeout         time.Duration
	logWriter       io.Writer
	logMu           sync.Mutex
	unsub           func()
	runMu           sync.Mutex
	activeRunID     string
	activeSessionID string
	eventSeq        uint64
	subSeq          uint64
	subMu           sync.Mutex
	subscribers     map[uint64]chan protocol.Envelope

	dispatchOverride func(protocol.Envelope) protocol.ResponseEnvelope
}

func NewServer(socketPath string) *Server {
	return &Server{
		socketPath:      socketPath,
		eventSocketPath: socketPath + ".events",
		timeout:         3 * time.Second,
		logWriter:       os.Stderr,
		subscribers:     make(map[uint64]chan protocol.Envelope),
	}
}

func (s *Server) SetEngine(engine *core.Engine, loop *core.CommandLoop) {
	s.engine = engine
	s.loop = loop
}

func (s *Server) SetSessionManager(mgr *session.Manager) {
	s.sessions = mgr
}

func (s *Server) SetCommandTimeout(d time.Duration) error {
	if d <= 0 {
		return fmt.Errorf("invalid_timeout")
	}
	s.timeout = d
	return nil
}

func (s *Server) SetLogWriter(w io.Writer) {
	if w == nil {
		s.logWriter = io.Discard
		return
	}
	s.logWriter = w
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
		s.engine.SetExtensionManager(extension.NewManager())
	}
	if s.loop == nil {
		s.loop = core.NewCommandLoop(s.engine)
	}
	if s.unsub == nil {
		s.unsub = s.engine.Subscribe(func(ev core.Event) {
			s.logRuntimeEvent(ev)
			s.publishRuntimeEvent(ev)
		})
	}
	s.loop.SetOnTurnEnd(func(r core.TurnResult) {
		sessionID := s.sessionIDForRun(r.RunID)
		_ = s.appendTurnRecord(sessionID, r.RunID, string(r.Kind), r.Input, r.Output)
	})
	_ = os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("listen uds: %w", err)
	}
	s.listener = ln

	_ = os.Remove(s.eventSocketPath)
	eventLn, err := net.Listen("unix", s.eventSocketPath)
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("listen event uds: %w", err)
	}
	s.eventListener = eventLn

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	go s.serveEvents(ctx, eventLn)

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
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.eventListener != nil {
		_ = s.eventListener.Close()
	}
	s.closeSubscribers()
	if s.socketPath != "" {
		_ = os.Remove(s.socketPath)
	}
	if s.eventSocketPath != "" {
		_ = os.Remove(s.eventSocketPath)
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
						Error:    &protocol.ErrorBody{Code: "internal_error", Message: "internal panic", Cause: fmt.Sprintf("%v", r)},
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

func (s *Server) serveEvents(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return
			}
			continue
		}
		s.wg.Add(1)
		go s.handleEventConn(conn)
	}
}

func (s *Server) handleEventConn(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	subID, ch := s.addSubscriber()
	defer s.removeSubscriber(subID)

	for env := range ch {
		if err := writeEvent(conn, env); err != nil {
			return
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
		if ext := s.extensionManager(); ext != nil {
			out, err := ext.RunSessionBeforeSwitchHooks(s.sessions.ActiveSession(), "", "new")
			if err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			if out.Cancel {
				return responseErr(env.ID, "command_rejected", out.Reason)
			}
		}
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
		if ext := s.extensionManager(); ext != nil {
			out, err := ext.RunSessionBeforeSwitchHooks(s.sessions.ActiveSession(), rawID, "switch")
			if err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			if out.Cancel {
				return responseErr(env.ID, "command_rejected", out.Reason)
			}
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
		rawParentID, _ := env.Payload["session_id"].(string)
		if rawParentID == "" {
			rawParentID, _ = env.Payload["parent_id"].(string) // backward-compatible alias
		}
		if rawParentID == "" {
			return responseErr(env.ID, "invalid_payload", "session_id is required")
		}
		if ext := s.extensionManager(); ext != nil {
			out, err := ext.RunSessionBeforeForkHooks(rawParentID)
			if err != nil {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			if out.Cancel {
				return responseErr(env.ID, "command_rejected", out.Reason)
			}
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
		wait := true
		if rawWait, exists := env.Payload["wait"]; exists {
			b, ok := rawWait.(bool)
			if !ok {
				return responseErr(env.ID, "invalid_payload", "wait must be a boolean")
			}
			wait = b
		}
		if !wait {
			return s.promptAsync(env.ID, text)
		}
		return s.promptSync(env.ID, text)
	case protocol.CmdSteer:
		text, ok := env.Payload["text"].(string)
		if !ok || text == "" {
			return responseErr(env.ID, "invalid_payload", "text is required")
		}
		if err := s.loop.Steer(text); err != nil {
			return responseErr(env.ID, "command_rejected", err.Error())
		}
		payload := map[string]any{"command": "steer"}
		if runID := s.loop.CurrentRunID(); runID != "" {
			payload["run_id"] = runID
		}
		return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: payload})
	case protocol.CmdFollowUp:
		text, ok := env.Payload["text"].(string)
		if !ok || text == "" {
			return responseErr(env.ID, "invalid_payload", "text is required")
		}
		if err := s.loop.FollowUp(text); err != nil {
			return responseErr(env.ID, "command_rejected", err.Error())
		}
		payload := map[string]any{"command": "follow_up"}
		if runID := s.loop.CurrentRunID(); runID != "" {
			payload["run_id"] = runID
		}
		return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: payload})
	case protocol.CmdAbort:
		if err := s.loop.Abort(); err != nil {
			return responseErr(env.ID, "command_rejected", err.Error())
		}
		payload := map[string]any{"command": "abort"}
		if runID := s.loop.CurrentRunID(); runID != "" {
			payload["run_id"] = runID
		}
		return responseOK(protocol.Envelope{V: protocol.Version, ID: env.ID, Type: "accepted", Payload: payload})
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
	case protocol.CmdSetSteeringMode:
		mode, ok := env.Payload["mode"].(string)
		if !ok || mode == "" {
			return responseErr(env.ID, "invalid_payload", "mode is required")
		}
		if err := s.loop.SetSteeringMode(core.QueueMode(mode)); err != nil {
			return responseErr(env.ID, "invalid_payload", "mode must be one-at-a-time or all")
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "accepted",
			Payload: map[string]any{"command": "set_steering_mode", "mode": mode},
		})
	case protocol.CmdSetFollowUpMode:
		mode, ok := env.Payload["mode"].(string)
		if !ok || mode == "" {
			return responseErr(env.ID, "invalid_payload", "mode is required")
		}
		if err := s.loop.SetFollowUpMode(core.QueueMode(mode)); err != nil {
			return responseErr(env.ID, "invalid_payload", "mode must be one-at-a-time or all")
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "accepted",
			Payload: map[string]any{"command": "set_follow_up_mode", "mode": mode},
		})
	case protocol.CmdExtensionCmd:
		name, ok := env.Payload["name"].(string)
		if !ok || name == "" {
			return responseErr(env.ID, "invalid_payload", "name is required")
		}
		rawPayload, _ := env.Payload["payload"].(map[string]any)
		if rawPayload == nil {
			rawPayload = map[string]any{}
		}
		out, err := s.engine.ExecuteExtensionCommand(name, rawPayload)
		if err != nil {
			return responseErr(env.ID, "command_rejected", err.Error())
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "extension_result",
			Payload: out,
		})
	default:
		return responseErr(env.ID, "not_implemented", "command not implemented yet")
	}
}

func (s *Server) promptSync(reqID, text string) protocol.ResponseEnvelope {
	sessionID, err := s.ensureActiveSession()
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "session operation failed", err)
	}
	promptWithContext, err := s.promptWithSessionContext(sessionID, text)
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to build session context", err)
	}

	events := make([]core.Event, 0, 16)
	unsub := s.engine.Subscribe(func(ev core.Event) {
		events = append(events, ev)
	})
	defer unsub()

	runID := fmt.Sprintf("sync-%d", time.Now().UnixNano())
	out, err := s.engine.Prompt(context.Background(), runID, promptWithContext)
	if err != nil {
		return responseErrWithCause(reqID, "provider_error", "provider request failed", err)
	}
	if err := s.appendTurnRecord(sessionID, runID, string(core.TurnPrompt), text, out); err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to persist session records", err)
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

func (s *Server) promptAsync(reqID, text string) protocol.ResponseEnvelope {
	sessionID, err := s.ensureActiveSession()
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "session operation failed", err)
	}
	promptWithContext, err := s.promptWithSessionContext(sessionID, text)
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to build session context", err)
	}

	runID, err := s.loop.PromptWithExecutionText(text, promptWithContext)
	if err != nil {
		return responseErr(reqID, "command_rejected", err.Error())
	}
	s.setActiveRunSession(runID, sessionID)
	return responseOK(protocol.Envelope{
		V:    protocol.Version,
		ID:   reqID,
		Type: "accepted",
		Payload: map[string]any{
			"command":    "prompt",
			"run_id":     runID,
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

func (s *Server) appendTurnRecord(sessionID, runID, kind, input, output string) error {
	if sessionID == "" {
		var err error
		sessionID, err = s.ensureActiveSession()
		if err != nil {
			return err
		}
	}
	if err := s.sessions.AppendMessageTo(sessionID, session.NewMessageEntry("user", input, runID, kind)); err != nil {
		return err
	}
	return s.sessions.AppendMessageTo(sessionID, session.NewMessageEntry("assistant", output, runID, kind))
}

func (s *Server) promptWithSessionContext(sessionID, prompt string) (string, error) {
	records, err := s.sessions.BuildMessageContext(sessionID)
	if err != nil {
		return "", err
	}
	return session.BuildPromptContext(records, prompt, 20), nil
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

func responseErrWithCause(id, code, message string, cause error) protocol.ResponseEnvelope {
	resp := responseErr(id, code, message)
	if cause != nil && resp.Error != nil {
		resp.Error.Cause = cause.Error()
	}
	return resp
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

func writeEvent(conn net.Conn, env protocol.Envelope) error {
	b, err := json.Marshal(env)
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

func (s *Server) logRuntimeEvent(ev core.Event) {
	logEvent := core.NewLogEvent("info", string(ev.Type))
	logEvent.RunID = ev.RunID
	if ev.Turn > 0 {
		logEvent.TurnID = fmt.Sprintf("turn-%d", ev.Turn)
	}
	logEvent.MessageID = ev.MessageID
	logEvent.ToolCallID = ev.ToolCallID
	s.writeLog(logEvent)
}

func (s *Server) publishRuntimeEvent(ev core.Event) {
	env := protocol.Envelope{
		V:       protocol.Version,
		ID:      fmt.Sprintf("evt-%d", atomic.AddUint64(&s.eventSeq, 1)),
		Type:    string(ev.Type),
		Payload: runtimeEventPayload(ev),
		TS:      ev.Timestamp,
	}

	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- env:
		default:
		}
	}
}

func runtimeEventPayload(ev core.Event) map[string]any {
	payload := map[string]any{}
	if ev.RunID != "" {
		payload["run_id"] = ev.RunID
	}
	if ev.Turn > 0 {
		payload["turn_id"] = strconv.Itoa(ev.Turn)
	}
	if ev.MessageID != "" {
		payload["message_id"] = ev.MessageID
	}
	if ev.Role != "" {
		payload["role"] = ev.Role
	}
	if ev.Delta != "" {
		payload["delta"] = ev.Delta
	}
	if ev.ToolCallID != "" {
		payload["tool_call_id"] = ev.ToolCallID
	}
	if ev.ToolName != "" {
		payload["tool_name"] = ev.ToolName
	}
	if ev.Message != "" {
		payload["message"] = ev.Message
	}
	if ev.Code != "" {
		payload["code"] = ev.Code
	}
	if ev.Cause != "" {
		payload["cause"] = ev.Cause
	}
	return payload
}

func (s *Server) addSubscriber() (uint64, chan protocol.Envelope) {
	ch := make(chan protocol.Envelope, 256)
	id := atomic.AddUint64(&s.subSeq, 1)
	s.subMu.Lock()
	s.subscribers[id] = ch
	s.subMu.Unlock()
	return id, ch
}

func (s *Server) removeSubscriber(id uint64) {
	s.subMu.Lock()
	ch, ok := s.subscribers[id]
	if ok {
		delete(s.subscribers, id)
	}
	s.subMu.Unlock()
	if ok {
		close(ch)
	}
}

func (s *Server) closeSubscribers() {
	s.subMu.Lock()
	subs := s.subscribers
	s.subscribers = make(map[uint64]chan protocol.Envelope)
	s.subMu.Unlock()
	for _, ch := range subs {
		close(ch)
	}
}

func (s *Server) writeLog(ev core.LogEvent) {
	if s.logWriter == nil {
		return
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	_ = core.WriteLogEvent(s.logWriter, ev)
}

func (s *Server) extensionManager() *extension.Manager {
	if s.engine == nil {
		return nil
	}
	return s.engine.ExtensionManager()
}

func (s *Server) setActiveRunSession(runID, sessionID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.activeRunID = runID
	s.activeSessionID = sessionID
}

func (s *Server) sessionIDForRun(runID string) string {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if runID != "" && runID == s.activeRunID {
		return s.activeSessionID
	}
	return ""
}
