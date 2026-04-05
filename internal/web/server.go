package web

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

const (
	defaultSessionsLimit = 30
	maxPendingBTW        = 5
)

//go:embed static/index.html static/styles.css static/app.js
var staticFiles embed.FS

type Options struct {
	Runner    *agent.Runner
	Store     *session.Store
	Session   *session.Session
	Config    config.Config
	Workspace string
	Addr      string
}

type Server struct {
	runner    *agent.Runner
	store     *session.Store
	cfg       config.Config
	workspace string

	mu              sync.Mutex
	activeRun       *runState
	pendingApproval *approvalState

	subsMu         sync.Mutex
	subscribers    map[int]*subscriber
	nextSubscriber int
	defaultSession *session.Session
}

type runState struct {
	ID            string
	SessionID     string
	Mode          planpkg.AgentMode
	Phase         string
	StartedAt     time.Time
	Cancel        context.CancelFunc
	Interrupting  bool
	InterruptSafe bool
	PendingBTW    []string
}

type approvalState struct {
	ID        string
	RunID     string
	SessionID string
	Command   string
	Reason    string
	Reply     chan bool
}

type subscriber struct {
	ID        int
	SessionID string
	Ch        chan streamEvent
}

type streamEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"session_id,omitempty"`
	RunID     string      `json:"run_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

type apiMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	ToolCalls  []llm.ToolCall `json:"tool_calls,omitempty"`
}

type sessionPayload struct {
	ID          string               `json:"id"`
	Workspace   string               `json:"workspace"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	Mode        planpkg.AgentMode    `json:"mode"`
	Plan        planpkg.State        `json:"plan"`
	ActiveSkill *session.ActiveSkill `json:"active_skill,omitempty"`
	Messages    []apiMessage         `json:"messages"`
}

func Run(opts Options) error {
	if opts.Runner == nil {
		return errors.New("runner is required")
	}
	if opts.Store == nil {
		return errors.New("store is required")
	}
	if strings.TrimSpace(opts.Workspace) == "" {
		return errors.New("workspace is required")
	}
	addr := strings.TrimSpace(opts.Addr)
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	s := &Server{
		runner:         opts.Runner,
		store:          opts.Store,
		cfg:            opts.Config,
		workspace:      opts.Workspace,
		subscribers:    make(map[int]*subscriber),
		defaultSession: opts.Session,
	}
	s.runner.SetObserver(agent.ObserverFunc(s.handleRunnerEvent))
	s.runner.SetApprovalHandler(s.handleApprovalRequest)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/sessions/", s.handleSessionByID)
	mux.HandleFunc("/api/mode", s.handleMode)
	mux.HandleFunc("/api/skills", s.handleSkills)
	mux.HandleFunc("/api/skills/clear", s.handleSkillClear)
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/runs/", s.handleRunByID)
	mux.HandleFunc("/api/events/stream", s.handleEventStream)
	mux.HandleFunc("/", s.handleStatic)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	err := httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"workspace": s.workspace,
		"time":      time.Now().UTC(),
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"provider": map[string]any{
			"type":  s.cfg.Provider.Type,
			"model": s.cfg.Provider.Model,
		},
		"approval_policy": s.cfg.ApprovalPolicy,
		"max_iterations":  s.cfg.MaxIterations,
		"stream":          s.cfg.Stream,
		"workspace":       s.workspace,
	})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSessions(w, r)
	case http.MethodPost:
		s.handleCreateSession(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
	}
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	limit := defaultSessionsLimit
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer")
			return
		}
		limit = value
	}
	summaries, warnings, err := s.store.List(limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sessions": summaries,
		"warnings": warnings,
	})
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Workspace string `json:"workspace"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	workspace := strings.TrimSpace(req.Workspace)
	if workspace == "" {
		workspace = s.workspace
	}
	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_workspace", err.Error())
		return
	}
	if !sameWorkspace(s.workspace, absWorkspace) {
		writeJSONError(w, http.StatusBadRequest, "workspace_mismatch", "web mode only supports the current workspace")
		return
	}
	sess := session.New(absWorkspace)
	if err := s.store.Save(sess); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"session": buildSessionPayload(sess),
	})
}

func (s *Server) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path, "/api/sessions/")
	if len(parts) == 0 {
		writeJSONError(w, http.StatusNotFound, "not_found", "session route not found")
		return
	}
	sessionID := parts[0]
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		s.handleGetSession(w, r, sessionID)
	case len(parts) == 2 && parts[1] == "resume" && r.Method == http.MethodPost:
		s.handleResumeSession(w, r, sessionID)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "unsupported session operation")
	}
}

func (s *Server) handleGetSession(w http.ResponseWriter, _ *http.Request, sessionID string) {
	sess, err := s.loadSessionInWorkspace(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "load_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session": buildSessionPayload(sess),
	})
}

func (s *Server) handleResumeSession(w http.ResponseWriter, _ *http.Request, sessionID string) {
	sess, err := s.loadSessionInWorkspace(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "resume_failed", err.Error())
		return
	}
	s.publish(streamEvent{
		Type:      "session_resumed",
		SessionID: sess.ID,
		Data: map[string]any{
			"session_id": sess.ID,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"session": buildSessionPayload(sess),
	})
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Mode      string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "invalid request payload")
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_session_id", "session_id is required")
		return
	}
	sess, err := s.loadSessionInWorkspace(req.SessionID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "load_failed", err.Error())
		return
	}
	mode := planpkg.NormalizeMode(req.Mode)
	sess.Mode = mode
	if err := s.store.Save(sess); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "save_failed", err.Error())
		return
	}
	s.publish(streamEvent{
		Type:      "mode_changed",
		SessionID: sess.ID,
		Data: map[string]any{
			"mode": mode,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"mode": mode,
	})
}

func (s *Server) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	skillsList, diagnostics := s.runner.ListSkills()
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	active := ""
	if sessionID != "" {
		sess, err := s.loadSessionInWorkspace(sessionID)
		if err == nil && sess.ActiveSkill != nil {
			active = strings.TrimSpace(sess.ActiveSkill.Name)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"skills":      skillsList,
		"diagnostics": diagnostics,
		"active":      active,
	})
}

func (s *Server) handleSkillClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "invalid request payload")
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_session_id", "session_id is required")
		return
	}
	sess, err := s.loadSessionInWorkspace(req.SessionID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "load_failed", err.Error())
		return
	}
	if err := s.runner.ClearActiveSkill(sess); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "clear_failed", err.Error())
		return
	}
	s.publish(streamEvent{
		Type:      "skill_cleared",
		SessionID: sess.ID,
		Data: map[string]any{
			"session_id": sess.ID,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetActiveRun(w, r)
	case http.MethodPost:
		s.handleStartRun(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET and POST are supported")
	}
}

func (s *Server) handleGetActiveRun(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"run": s.activeRunSnapshotLocked(),
	})
}

func (s *Server) handleStartRun(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string `json:"session_id"`
		Prompt    string `json:"prompt"`
		Mode      string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "invalid request payload")
		return
	}
	if strings.TrimSpace(req.SessionID) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_session_id", "session_id is required")
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_prompt", "prompt is required")
		return
	}

	sess, err := s.loadSessionInWorkspace(req.SessionID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "load_failed", err.Error())
		return
	}
	mode := planpkg.NormalizeMode(req.Mode)
	if strings.TrimSpace(req.Mode) == "" {
		mode = planpkg.NormalizeMode(string(sess.Mode))
	}

	s.mu.Lock()
	if s.activeRun != nil {
		active := s.activeRunSnapshotLocked()
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "run_in_progress",
			"run":   active,
		})
		return
	}
	run := s.startRunLocked(sess, strings.TrimSpace(req.Prompt), mode, "prompt")
	s.mu.Unlock()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"run": map[string]any{
			"id":         run.ID,
			"session_id": run.SessionID,
			"mode":       run.Mode,
			"phase":      run.Phase,
			"started_at": run.StartedAt,
		},
	})
}

func (s *Server) handleRunByID(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(r.URL.Path, "/api/runs/")
	if len(parts) < 2 {
		writeJSONError(w, http.StatusNotFound, "not_found", "run route not found")
		return
	}
	runID := parts[0]
	action := parts[1]
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is supported")
		return
	}
	switch action {
	case "btw":
		s.handleRunBTW(w, r, runID)
	case "cancel":
		s.handleRunCancel(w, r, runID)
	case "approval":
		s.handleRunApproval(w, r, runID)
	default:
		writeJSONError(w, http.StatusNotFound, "not_found", "unsupported run operation")
	}
}

func (s *Server) handleRunBTW(w http.ResponseWriter, r *http.Request, runID string) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "invalid request payload")
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		writeJSONError(w, http.StatusBadRequest, "missing_message", "message is required")
		return
	}

	var cancel context.CancelFunc
	var pendingCount int
	var dropped int
	var phase string
	var sessionID string

	s.mu.Lock()
	if s.activeRun == nil || s.activeRun.ID != runID {
		s.mu.Unlock()
		writeJSONError(w, http.StatusConflict, "run_not_active", "run is not active")
		return
	}
	run := s.activeRun
	run.PendingBTW, dropped = queueBTW(run.PendingBTW, message)
	pendingCount = len(run.PendingBTW)
	sessionID = run.SessionID
	phase = run.Phase

	if !run.Interrupting {
		run.Interrupting = true
		if run.Phase == "tool" {
			run.InterruptSafe = true
		} else {
			run.InterruptSafe = false
			cancel = run.Cancel
		}
	}
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.publish(streamEvent{
		Type:      "btw_queued",
		SessionID: sessionID,
		RunID:     runID,
		Data: map[string]any{
			"pending_count": pendingCount,
			"dropped":       dropped,
			"phase":         phase,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"pending_count": pendingCount,
		"dropped":       dropped,
	})
}

func (s *Server) handleRunCancel(w http.ResponseWriter, _ *http.Request, runID string) {
	var cancel context.CancelFunc
	var sessionID string
	s.mu.Lock()
	if s.activeRun == nil || s.activeRun.ID != runID {
		s.mu.Unlock()
		writeJSONError(w, http.StatusConflict, "run_not_active", "run is not active")
		return
	}
	sessionID = s.activeRun.SessionID
	cancel = s.activeRun.Cancel
	s.activeRun.Interrupting = false
	s.activeRun.InterruptSafe = false
	s.activeRun.PendingBTW = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.publish(streamEvent{
		Type:      "run_cancel_requested",
		SessionID: sessionID,
		RunID:     runID,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleRunApproval(w http.ResponseWriter, r *http.Request, runID string) {
	var req struct {
		ApprovalID string `json:"approval_id"`
		Approved   bool   `json:"approved"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_json", "invalid request payload")
		return
	}

	var pending *approvalState
	s.mu.Lock()
	if s.pendingApproval == nil || s.pendingApproval.RunID != runID {
		s.mu.Unlock()
		writeJSONError(w, http.StatusConflict, "approval_not_pending", "no approval is pending for this run")
		return
	}
	if strings.TrimSpace(req.ApprovalID) != "" && req.ApprovalID != s.pendingApproval.ID {
		s.mu.Unlock()
		writeJSONError(w, http.StatusConflict, "approval_mismatch", "approval_id does not match current pending approval")
		return
	}
	pending = s.pendingApproval
	s.pendingApproval = nil
	s.mu.Unlock()

	select {
	case pending.Reply <- req.Approved:
	default:
	}
	s.publish(streamEvent{
		Type:      "approval_resolved",
		SessionID: pending.SessionID,
		RunID:     pending.RunID,
		Data: map[string]any{
			"approval_id": pending.ID,
			"approved":    req.Approved,
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is supported")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "stream_unsupported", "response streaming is not supported")
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	sub := s.addSubscriber(sessionID)
	defer s.removeSubscriber(sub.ID)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	connected := streamEvent{
		Type:      "connected",
		SessionID: sessionID,
		Data: map[string]any{
			"session_id": sessionID,
		},
	}
	if err := writeSSE(w, connected); err != nil {
		return
	}
	flusher.Flush()

	s.mu.Lock()
	active := s.activeRunSnapshotLocked()
	s.mu.Unlock()
	if active != nil {
		_ = writeSSE(w, streamEvent{Type: "run_state", Data: active})
		flusher.Flush()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-sub.Ch:
			if err := writeSSE(w, event); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeJSONError(w, http.StatusNotFound, "not_found", "api route not found")
		return
	}
	switch r.URL.Path {
	case "/", "/index.html":
		serveStaticFile(w, "static/index.html", "text/html; charset=utf-8")
	case "/styles.css":
		serveStaticFile(w, "static/styles.css", "text/css; charset=utf-8")
	case "/app.js":
		serveStaticFile(w, "static/app.js", "application/javascript; charset=utf-8")
	default:
		http.NotFound(w, r)
	}
}

func serveStaticFile(w http.ResponseWriter, path, contentType string) {
	data, err := staticFiles.ReadFile(path)
	if err != nil {
		http.Error(w, "asset not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(data)
}

func (s *Server) startRunLocked(sess *session.Session, prompt string, mode planpkg.AgentMode, source string) *runState {
	mode = planpkg.NormalizeMode(string(mode))
	if sess.Mode != mode {
		sess.Mode = mode
		_ = s.store.Save(sess)
	}

	runID := newID("run")
	ctx, cancel := context.WithCancel(context.Background())
	run := &runState{
		ID:         runID,
		SessionID:  sess.ID,
		Mode:       mode,
		Phase:      "thinking",
		StartedAt:  time.Now().UTC(),
		Cancel:     cancel,
		PendingBTW: make([]string, 0, maxPendingBTW),
	}
	s.activeRun = run
	s.publish(streamEvent{
		Type:      "run_state",
		SessionID: run.SessionID,
		RunID:     run.ID,
		Data: map[string]any{
			"running":    true,
			"run_id":     run.ID,
			"session_id": run.SessionID,
			"mode":       run.Mode,
			"phase":      run.Phase,
			"source":     source,
		},
	})

	go func(runID string, workingSession *session.Session, runMode planpkg.AgentMode, runPrompt string) {
		_, err := s.runner.RunPromptWithInput(ctx, workingSession, agent.RunPromptInput{
			UserMessage: llm.NewUserTextMessage(runPrompt),
			DisplayText: runPrompt,
		}, string(runMode), nil)
		s.handleRunCompletion(runID, workingSession, err)
	}(runID, sess, mode, prompt)
	return run
}

func (s *Server) handleRunCompletion(runID string, sess *session.Session, err error) {
	var (
		oldRunID   string
		newRunID   string
		sessionID  string
		restarted  bool
		restartErr error
	)

	s.mu.Lock()
	if s.activeRun == nil || s.activeRun.ID != runID {
		s.mu.Unlock()
		return
	}
	current := s.activeRun
	oldRunID = current.ID
	sessionID = current.SessionID

	if s.pendingApproval != nil && s.pendingApproval.RunID == current.ID {
		select {
		case s.pendingApproval.Reply <- false:
		default:
		}
		s.pendingApproval = nil
	}

	if current.Interrupting && len(current.PendingBTW) > 0 {
		restartPrompt := composeBTWPrompt(current.PendingBTW)
		current.PendingBTW = nil
		current.Interrupting = false
		current.InterruptSafe = false

		latest, loadErr := s.store.Load(current.SessionID)
		if loadErr != nil {
			latest = sess
			restartErr = loadErr
		}
		nextRun := s.startRunLocked(latest, restartPrompt, current.Mode, "btw_restart")
		newRunID = nextRun.ID
		restarted = true
	} else {
		s.activeRun = nil
	}
	s.mu.Unlock()

	if restarted {
		s.publish(streamEvent{
			Type:      "run_finished",
			SessionID: sessionID,
			RunID:     oldRunID,
			Data: map[string]any{
				"status": "canceled",
				"error":  "run canceled to apply BTW update",
			},
		})
		s.publish(streamEvent{
			Type:      "btw_restarted",
			SessionID: sessionID,
			Data: map[string]any{
				"old_run_id": oldRunID,
				"new_run_id": newRunID,
				"warning":    errorString(restartErr),
			},
		})
		return
	}

	if err != nil {
		status := "failed"
		if errors.Is(err, context.Canceled) {
			status = "canceled"
		}
		s.publish(streamEvent{
			Type:      "run_finished",
			SessionID: sessionID,
			RunID:     oldRunID,
			Data: map[string]any{
				"status": status,
				"error":  err.Error(),
			},
		})
	}
	s.publish(streamEvent{
		Type:      "run_state",
		SessionID: sessionID,
		RunID:     oldRunID,
		Data: map[string]any{
			"running": false,
			"run_id":  oldRunID,
		},
	})
}

func (s *Server) handleRunnerEvent(event agent.Event) {
	var (
		runID      string
		cancel     context.CancelFunc
		shouldStop bool
	)
	s.mu.Lock()
	if s.activeRun != nil && s.activeRun.SessionID == event.SessionID {
		runID = s.activeRun.ID
		switch event.Type {
		case agent.EventRunStarted:
			s.activeRun.Phase = "thinking"
		case agent.EventAssistantDelta:
			s.activeRun.Phase = "responding"
		case agent.EventToolCallStarted:
			s.activeRun.Phase = "tool"
		case agent.EventToolCallCompleted:
			s.activeRun.Phase = "thinking"
			if s.activeRun.InterruptSafe && s.activeRun.Interrupting && len(s.activeRun.PendingBTW) > 0 {
				shouldStop = true
				cancel = s.activeRun.Cancel
				s.activeRun.InterruptSafe = false
			}
		case agent.EventPlanUpdated:
			s.activeRun.Phase = "plan"
		case agent.EventRunFinished:
			s.activeRun.Phase = "idle"
		}
	}
	s.mu.Unlock()

	if shouldStop && cancel != nil {
		cancel()
	}

	data := map[string]any{}
	switch event.Type {
	case agent.EventRunStarted:
		data["user_input"] = event.UserInput
	case agent.EventAssistantDelta, agent.EventAssistantMessage, agent.EventRunFinished:
		data["content"] = event.Content
	case agent.EventToolCallStarted:
		data["tool_name"] = event.ToolName
		data["tool_arguments"] = event.ToolArguments
	case agent.EventToolCallCompleted:
		data["tool_name"] = event.ToolName
		data["tool_result"] = event.ToolResult
		data["error"] = event.Error
	case agent.EventPlanUpdated:
		data["plan"] = event.Plan
	}
	s.publish(streamEvent{
		Type:      string(event.Type),
		SessionID: event.SessionID,
		RunID:     runID,
		Data:      data,
	})
}

func (s *Server) handleApprovalRequest(req tools.ApprovalRequest) (bool, error) {
	s.mu.Lock()
	if s.activeRun == nil {
		s.mu.Unlock()
		return false, errors.New("approval requested without an active run")
	}
	if s.pendingApproval != nil {
		s.mu.Unlock()
		return false, errors.New("another approval is already pending")
	}
	approval := &approvalState{
		ID:        newID("approval"),
		RunID:     s.activeRun.ID,
		SessionID: s.activeRun.SessionID,
		Command:   req.Command,
		Reason:    req.Reason,
		Reply:     make(chan bool, 1),
	}
	s.pendingApproval = approval
	s.mu.Unlock()

	s.publish(streamEvent{
		Type:      "approval_required",
		SessionID: approval.SessionID,
		RunID:     approval.RunID,
		Data: map[string]any{
			"approval_id": approval.ID,
			"command":     approval.Command,
			"reason":      approval.Reason,
		},
	})
	approved := <-approval.Reply
	return approved, nil
}

func (s *Server) addSubscriber(sessionID string) *subscriber {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	s.nextSubscriber++
	sub := &subscriber{
		ID:        s.nextSubscriber,
		SessionID: sessionID,
		Ch:        make(chan streamEvent, 256),
	}
	s.subscribers[sub.ID] = sub
	return sub
}

func (s *Server) removeSubscriber(id int) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	sub, ok := s.subscribers[id]
	if !ok {
		return
	}
	delete(s.subscribers, id)
	close(sub.Ch)
}

func (s *Server) publish(event streamEvent) {
	if strings.TrimSpace(event.Type) == "" {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	s.subsMu.Lock()
	targets := make([]*subscriber, 0, len(s.subscribers))
	for _, sub := range s.subscribers {
		if sub.SessionID != "" && event.SessionID != "" && sub.SessionID != event.SessionID {
			continue
		}
		targets = append(targets, sub)
	}
	s.subsMu.Unlock()

	for _, sub := range targets {
		select {
		case sub.Ch <- event:
		default:
		}
	}
}

func (s *Server) activeRunSnapshotLocked() map[string]any {
	if s.activeRun == nil {
		return nil
	}
	return map[string]any{
		"id":             s.activeRun.ID,
		"session_id":     s.activeRun.SessionID,
		"mode":           s.activeRun.Mode,
		"phase":          s.activeRun.Phase,
		"started_at":     s.activeRun.StartedAt,
		"interrupting":   s.activeRun.Interrupting,
		"interrupt_safe": s.activeRun.InterruptSafe,
		"pending_btw":    len(s.activeRun.PendingBTW),
	}
}

func (s *Server) loadSessionInWorkspace(sessionID string) (*session.Session, error) {
	sess, err := s.store.Load(sessionID)
	if err != nil {
		return nil, err
	}
	if !sameWorkspace(s.workspace, sess.Workspace) {
		return nil, fmt.Errorf("session %s belongs to workspace %s, current workspace is %s", sess.ID, sess.Workspace, s.workspace)
	}
	return sess, nil
}

func buildSessionPayload(sess *session.Session) sessionPayload {
	messages := make([]apiMessage, 0, len(sess.Messages))
	for _, msg := range sess.Messages {
		msg.Normalize()
		item := apiMessage{
			Role:       string(msg.Role),
			ToolCallID: strings.TrimSpace(msg.ToolCallID),
		}
		if text := strings.TrimSpace(msg.Text()); text != "" {
			item.Content = text
		}
		if len(msg.ToolCalls) > 0 {
			item.ToolCalls = msg.ToolCalls
		}
		if item.Content == "" && item.ToolCallID == "" && len(item.ToolCalls) == 0 {
			continue
		}
		messages = append(messages, item)
	}
	return sessionPayload{
		ID:          sess.ID,
		Workspace:   sess.Workspace,
		CreatedAt:   sess.CreatedAt,
		UpdatedAt:   sess.UpdatedAt,
		Mode:        sess.Mode,
		Plan:        sess.Plan,
		ActiveSkill: sess.ActiveSkill,
		Messages:    messages,
	}
}

func splitPath(path, prefix string) []string {
	trimmed := strings.TrimPrefix(path, prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func queueBTW(queue []string, value string) ([]string, int) {
	queue = append(queue, strings.TrimSpace(value))
	if len(queue) <= maxPendingBTW {
		return queue, 0
	}
	dropped := len(queue) - maxPendingBTW
	queue = queue[dropped:]
	return queue, dropped
}

func composeBTWPrompt(entries []string) string {
	trimmed := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry != "" {
			trimmed = append(trimmed, entry)
		}
	}
	if len(trimmed) == 0 {
		return "Continue from the current state."
	}
	if len(trimmed) == 1 {
		return strings.Join([]string{
			"You were executing an existing task and received a BTW update from the user.",
			"Continue from the current workspace state.",
			"User BTW update:",
			trimmed[0],
		}, "\n")
	}
	lines := []string{
		"You were executing an existing task and received multiple BTW updates.",
		"Continue from the current workspace state.",
		"Later updates have higher priority:",
	}
	for i, entry := range trimmed {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, entry))
	}
	return strings.Join(lines, "\n")
}

func sameWorkspace(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func newID(prefix string) string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixMilli(), hex.EncodeToString(buf))
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeSSE(w http.ResponseWriter, event streamEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":   code,
		"message": message,
	})
}
