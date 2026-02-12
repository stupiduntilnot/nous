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
	"strings"
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
	activeParentID  string
	eventSeq        uint64
	subSeq          uint64
	subMu           sync.Mutex
	subscribers     map[uint64]chan protocol.Envelope
	compactor       core.Compactor
	compactionMu    sync.Mutex
	retriedOverflow map[string]bool

	dispatchOverride func(protocol.Envelope) protocol.ResponseEnvelope
}

func NewServer(socketPath string) *Server {
	return &Server{
		socketPath:      socketPath,
		eventSocketPath: socketPath + ".events",
		timeout:         30 * time.Second,
		logWriter:       os.Stderr,
		subscribers:     make(map[uint64]chan protocol.Envelope),
		compactor:       core.NewDeterministicCompactor(core.DefaultCompactionSettings),
		retriedOverflow: make(map[string]bool),
	}
}

func (s *Server) SetEngine(engine *core.Engine, loop *core.CommandLoop) {
	s.engine = engine
	s.loop = loop
}

func (s *Server) SetSessionManager(mgr *session.Manager) {
	s.sessions = mgr
}

func (s *Server) SetCompactor(compactor core.Compactor) {
	if compactor == nil {
		s.compactor = core.NewDeterministicCompactor(core.DefaultCompactionSettings)
		return
	}
	s.compactor = compactor
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
			if ev.Type == core.EventAgentEnd && ev.RunID != "" {
				s.clearOverflowRetry(ev.RunID)
			}
		})
	}
	s.loop.SetOnTurnEnd(func(r core.TurnResult) {
		sessionID, parentID := s.runContextFor(r.RunID)
		if r.Err != nil {
			if isContextOverflowError(r.Err) && s.markOverflowRetry(r.RunID) && sessionID != "" {
				if _, _, err := s.compactSession(sessionID, "", "overflow"); err == nil {
					if r.Kind == core.TurnFollowUp {
						_ = s.loop.FollowUp(r.Input)
					} else {
						_ = s.loop.Steer(r.Input)
					}
				}
			}
			return
		}
		nextParentID, err := s.appendTurnRecord(sessionID, r.RunID, string(r.Kind), r.Input, r.Output, parentID)
		if err != nil {
			return
		}
		if nextParentID != "" {
			s.setRunParent(r.RunID, nextParentID)
		}
		if sessionID != "" {
			_, _, _ = s.compactSessionIfThreshold(sessionID)
		}
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
		leafID := ""
		if rawLeaf, exists := env.Payload["leaf_id"]; exists {
			parsed, ok := rawLeaf.(string)
			if !ok {
				return responseErr(env.ID, "invalid_payload", "leaf_id must be a string")
			}
			leafID = parsed
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
			return s.promptAsync(env.ID, text, leafID)
		}
		return s.promptSync(env.ID, text, leafID)
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
	case protocol.CmdGetState:
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "state",
			Payload: s.statePayload(),
		})
	case protocol.CmdGetMessages:
		sessionID, _ := env.Payload["session_id"].(string)
		if sessionID == "" && s.sessions != nil {
			sessionID = s.sessions.ActiveSession()
		}
		leafID, _ := env.Payload["leaf_id"].(string)
		if sessionID == "" {
			payload := map[string]any{
				"session_id": "",
				"messages":   []session.MessageEntry{},
			}
			if leafID != "" {
				payload["leaf_id"] = leafID
			}
			return responseOK(protocol.Envelope{
				V:       protocol.Version,
				ID:      env.ID,
				Type:    "messages",
				Payload: payload,
			})
		}
		if s.sessions == nil {
			return responseErr(env.ID, "session_error", "session_manager_not_ready")
		}
		var (
			entries []session.MessageEntry
			err     error
		)
		resolvedLeafID, err := s.resolveLeafID(sessionID, leafID)
		if err != nil {
			if os.IsNotExist(err) {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			return responseErr(env.ID, "session_error", err.Error())
		}
		if resolvedLeafID != "" {
			entries, err = s.sessions.BuildMessageContextFromLeaf(sessionID, resolvedLeafID)
		} else {
			entries, err = s.sessions.BuildMessageContext(sessionID)
		}
		if err != nil {
			if os.IsNotExist(err) {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			return responseErr(env.ID, "session_error", err.Error())
		}
		payload := map[string]any{
			"session_id": sessionID,
			"messages":   entries,
		}
		if resolvedLeafID != "" {
			payload["leaf_id"] = resolvedLeafID
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "messages",
			Payload: payload,
		})
	case protocol.CmdSetLeaf:
		if s.sessions == nil {
			return responseErr(env.ID, "session_error", "session_manager_not_ready")
		}
		sessionID, _ := env.Payload["session_id"].(string)
		if sessionID == "" {
			sessionID = s.sessions.ActiveSession()
		}
		if sessionID == "" {
			return responseErr(env.ID, "invalid_payload", "session_id is required")
		}
		leafID, _ := env.Payload["leaf_id"].(string)
		leafID = strings.TrimSpace(leafID)
		if leafID == "" {
			return responseErr(env.ID, "invalid_payload", "leaf_id is required")
		}
		if err := s.sessions.SetActiveLeaf(sessionID, leafID); err != nil {
			if os.IsNotExist(err) {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			if strings.Contains(err.Error(), "leaf_not_found") {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseErr(env.ID, "session_error", err.Error())
		}
		return responseOK(protocol.Envelope{
			V:    protocol.Version,
			ID:   env.ID,
			Type: "leaf",
			Payload: map[string]any{
				"session_id": sessionID,
				"leaf_id":    leafID,
			},
		})
	case protocol.CmdGetTree:
		if s.sessions == nil {
			return responseErr(env.ID, "session_error", "session_manager_not_ready")
		}
		sessionID, _ := env.Payload["session_id"].(string)
		if sessionID == "" {
			sessionID = s.sessions.ActiveSession()
		}
		if sessionID == "" {
			return responseOK(protocol.Envelope{
				V:       protocol.Version,
				ID:      env.ID,
				Type:    "tree",
				Payload: map[string]any{"session_id": "", "nodes": []map[string]any{}},
			})
		}
		entries, err := s.sessions.BuildMessageTree(sessionID)
		if err != nil {
			if os.IsNotExist(err) {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			return responseErr(env.ID, "session_error", err.Error())
		}
		nodes := make([]map[string]any, 0, len(entries))
		for _, rec := range entries {
			nodes = append(nodes, map[string]any{
				"id":        rec.ID,
				"parent_id": rec.ParentID,
				"role":      rec.Role,
				"snippet":   snippet(rec.Text, 80),
			})
		}
		payload := map[string]any{
			"session_id": sessionID,
			"nodes":      nodes,
		}
		if activeLeaf, err := s.sessions.ActiveLeaf(sessionID); err == nil && activeLeaf != "" {
			payload["leaf_id"] = activeLeaf
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "tree",
			Payload: payload,
		})
	case protocol.CmdCompactSession:
		if s.sessions == nil {
			return responseErr(env.ID, "session_error", "session_manager_not_ready")
		}
		sessionID, _ := env.Payload["session_id"].(string)
		if sessionID == "" {
			sessionID = s.sessions.ActiveSession()
		}
		if sessionID == "" {
			return responseErr(env.ID, "invalid_payload", "session_id is required")
		}
		instruction := ""
		if raw, ok := env.Payload["instruction"]; ok {
			v, ok := raw.(string)
			if !ok {
				return responseErr(env.ID, "invalid_payload", "instruction must be a string")
			}
			instruction = v
		}
		_, result, err := s.compactSession(sessionID, instruction, "manual")
		if err != nil {
			if os.IsNotExist(err) {
				return responseErr(env.ID, "session_not_found", err.Error())
			}
			if strings.Contains(err.Error(), "nothing_to_compact") {
				return responseErr(env.ID, "command_rejected", err.Error())
			}
			return responseErr(env.ID, "session_error", err.Error())
		}
		payload := map[string]any{
			"session_id":          sessionID,
			"summary":             result.Summary,
			"first_kept_entry_id": result.FirstKeptEntryID,
			"tokens_before":       result.TokensBefore,
			"trigger":             "manual",
		}
		if strings.TrimSpace(instruction) != "" {
			payload["instruction"] = strings.TrimSpace(instruction)
		}
		return responseOK(protocol.Envelope{
			V:       protocol.Version,
			ID:      env.ID,
			Type:    "compaction",
			Payload: payload,
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

func (s *Server) promptSync(reqID, text, leafID string) protocol.ResponseEnvelope {
	sessionID, err := s.ensureActiveSession()
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "session operation failed", err)
	}
	resolvedLeafID, err := s.resolveLeafID(sessionID, leafID)
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to resolve session leaf", err)
	}
	promptWithContext, err := s.promptWithSessionContext(sessionID, text, resolvedLeafID)
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
		if isContextOverflowError(err) {
			if _, _, compactErr := s.compactSession(sessionID, "", "overflow"); compactErr == nil {
				retryPrompt, ctxErr := s.promptWithSessionContext(sessionID, text, resolvedLeafID)
				if ctxErr != nil {
					return responseErrWithCause(reqID, "session_error", "failed to build session context", ctxErr)
				}
				out, err = s.engine.Prompt(context.Background(), runID+"-retry", retryPrompt)
			}
		}
		if err != nil {
			return responseErrWithCause(reqID, "provider_error", "provider request failed", err)
		}
	}
	if _, err := s.appendTurnRecord(sessionID, runID, string(core.TurnPrompt), text, out, resolvedLeafID); err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to persist session records", err)
	}
	_, _, _ = s.compactSessionIfThreshold(sessionID)
	payload := map[string]any{
		"output":     out,
		"events":     events,
		"session_id": sessionID,
	}
	if resolvedLeafID != "" {
		payload["leaf_id"] = resolvedLeafID
	}
	return responseOK(protocol.Envelope{
		V:       protocol.Version,
		ID:      reqID,
		Type:    "result",
		Payload: payload,
	})
}

func (s *Server) promptAsync(reqID, text, leafID string) protocol.ResponseEnvelope {
	sessionID, err := s.ensureActiveSession()
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "session operation failed", err)
	}
	resolvedLeafID, err := s.resolveLeafID(sessionID, leafID)
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to resolve session leaf", err)
	}
	promptWithContext, err := s.promptWithSessionContext(sessionID, text, resolvedLeafID)
	if err != nil {
		return responseErrWithCause(reqID, "session_error", "failed to build session context", err)
	}

	runID, err := s.loop.PromptWithExecutionText(text, promptWithContext)
	if err != nil {
		return responseErr(reqID, "command_rejected", err.Error())
	}
	s.setActiveRunContext(runID, sessionID, resolvedLeafID)
	payload := map[string]any{
		"command":    "prompt",
		"run_id":     runID,
		"session_id": sessionID,
	}
	if resolvedLeafID != "" {
		payload["leaf_id"] = resolvedLeafID
	}
	return responseOK(protocol.Envelope{
		V:       protocol.Version,
		ID:      reqID,
		Type:    "accepted",
		Payload: payload,
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

func (s *Server) appendTurnRecord(sessionID, runID, kind, input, output, parentID string) (string, error) {
	if sessionID == "" {
		var err error
		sessionID, err = s.ensureActiveSession()
		if err != nil {
			return "", err
		}
	}
	user := session.NewMessageEntry("user", input, runID, kind)
	if parentID != "" {
		user.ParentID = parentID
	}
	user, err := s.sessions.AppendMessageToResolved(sessionID, user)
	if err != nil {
		return "", err
	}
	assistant := session.NewMessageEntry("assistant", output, runID, kind)
	assistant.ParentID = user.ID
	assistant, err = s.sessions.AppendMessageToResolved(sessionID, assistant)
	if err != nil {
		return "", err
	}
	_ = s.sessions.SetActiveLeaf(sessionID, assistant.ID)
	return assistant.ID, nil
}

func (s *Server) promptWithSessionContext(sessionID, prompt, leafID string) (string, error) {
	var (
		records []session.MessageEntry
		err     error
	)
	if leafID != "" {
		records, err = s.sessions.BuildMessageContextFromLeaf(sessionID, leafID)
	} else {
		records, err = s.sessions.BuildMessageContext(sessionID)
	}
	if err != nil {
		return "", err
	}
	return session.BuildPromptContext(records, prompt, 20), nil
}

func (s *Server) resolveLeafID(sessionID, explicitLeafID string) (string, error) {
	explicitLeafID = strings.TrimSpace(explicitLeafID)
	if explicitLeafID != "" {
		return explicitLeafID, nil
	}
	if s.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return "", nil
	}
	leafID, err := s.sessions.ActiveLeaf(sessionID)
	if err != nil {
		if os.IsNotExist(err) {
			return "", err
		}
		return "", nil
	}
	return strings.TrimSpace(leafID), nil
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

func (s *Server) statePayload() map[string]any {
	runState := string(core.StateIdle)
	runID := ""
	steeringMode := string(core.QueueModeOneAtATime)
	followUpMode := string(core.QueueModeOneAtATime)
	pendingSteers := 0
	pendingFollowUps := 0

	if s.loop != nil {
		runState = string(s.loop.State())
		runID = s.loop.CurrentRunID()
		steeringMode = string(s.loop.SteeringMode())
		followUpMode = string(s.loop.FollowUpMode())
		pendingSteers, pendingFollowUps = s.loop.PendingCounts()
	}

	sessionID := ""
	if s.sessions != nil {
		sessionID = s.sessions.ActiveSession()
	}

	return map[string]any{
		"run_state":      runState,
		"run_id":         runID,
		"session_id":     sessionID,
		"steering_mode":  steeringMode,
		"follow_up_mode": followUpMode,
		"pending_counts": map[string]any{
			"steer":     pendingSteers,
			"follow_up": pendingFollowUps,
		},
	}
}

func (s *Server) setActiveRunContext(runID, sessionID, parentID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.activeRunID = runID
	s.activeSessionID = sessionID
	s.activeParentID = parentID
}

func (s *Server) runContextFor(runID string) (sessionID, parentID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if runID != "" && runID == s.activeRunID {
		return s.activeSessionID, s.activeParentID
	}
	return "", ""
}

func (s *Server) setRunParent(runID, parentID string) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if runID != "" && runID == s.activeRunID {
		s.activeParentID = parentID
	}
}

func (s *Server) compactSession(sessionID, instruction, trigger string) (session.CompactionEntry, core.CompactionResult, error) {
	if s.sessions == nil {
		return session.CompactionEntry{}, core.CompactionResult{}, fmt.Errorf("session_manager_not_ready")
	}
	if strings.TrimSpace(sessionID) == "" {
		return session.CompactionEntry{}, core.CompactionResult{}, fmt.Errorf("empty_session_id")
	}

	messages, err := s.sessions.BuildMessageContext(sessionID)
	if err != nil {
		return session.CompactionEntry{}, core.CompactionResult{}, err
	}
	compactMessages := make([]core.CompactionMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.ID == "" || strings.TrimSpace(msg.Text) == "" || strings.TrimSpace(msg.Role) == "" {
			continue
		}
		compactMessages = append(compactMessages, core.CompactionMessage{
			ID:   msg.ID,
			Role: msg.Role,
			Text: msg.Text,
		})
	}

	s.compactionMu.Lock()
	compactor := s.compactor
	s.compactionMu.Unlock()
	if compactor == nil {
		compactor = core.NewDeterministicCompactor(core.DefaultCompactionSettings)
	}

	result, err := compactor.Compact(compactMessages, instruction)
	if err != nil {
		return session.CompactionEntry{}, core.CompactionResult{}, err
	}
	entry := session.NewCompactionEntry(result.Summary, result.FirstKeptEntryID, instruction, result.TokensBefore, trigger)
	entry, err = s.sessions.AppendCompactionToResolved(sessionID, entry)
	if err != nil {
		return session.CompactionEntry{}, core.CompactionResult{}, err
	}
	return entry, result, nil
}

func (s *Server) compactSessionIfThreshold(sessionID string) (session.CompactionEntry, core.CompactionResult, error) {
	if s.sessions == nil || strings.TrimSpace(sessionID) == "" {
		return session.CompactionEntry{}, core.CompactionResult{}, fmt.Errorf("empty_session_id")
	}
	messages, err := s.sessions.BuildMessageContext(sessionID)
	if err != nil {
		return session.CompactionEntry{}, core.CompactionResult{}, err
	}
	compactMessages := make([]core.CompactionMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.ID == "" || strings.TrimSpace(msg.Text) == "" || strings.TrimSpace(msg.Role) == "" {
			continue
		}
		compactMessages = append(compactMessages, core.CompactionMessage{
			ID:   msg.ID,
			Role: msg.Role,
			Text: msg.Text,
		})
	}

	s.compactionMu.Lock()
	compactor := s.compactor
	s.compactionMu.Unlock()
	if compactor == nil {
		compactor = core.NewDeterministicCompactor(core.DefaultCompactionSettings)
	}
	if !compactor.ShouldCompact(compactor.EstimateTokens(compactMessages)) {
		return session.CompactionEntry{}, core.CompactionResult{}, fmt.Errorf("nothing_to_compact")
	}
	return s.compactSession(sessionID, "", "threshold")
}

func (s *Server) markOverflowRetry(runID string) bool {
	if strings.TrimSpace(runID) == "" {
		return false
	}
	s.compactionMu.Lock()
	defer s.compactionMu.Unlock()
	if s.retriedOverflow == nil {
		s.retriedOverflow = make(map[string]bool)
	}
	if s.retriedOverflow[runID] {
		return false
	}
	s.retriedOverflow[runID] = true
	return true
}

func (s *Server) clearOverflowRetry(runID string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	s.compactionMu.Lock()
	defer s.compactionMu.Unlock()
	delete(s.retriedOverflow, runID)
}

func snippet(text string, max int) string {
	value := strings.TrimSpace(text)
	if max <= 0 || len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	patterns := []string{
		"context_length_exceeded",
		"maximum context length",
		"too many tokens",
		"token limit",
		"prompt is too long",
		"max tokens",
	}
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
