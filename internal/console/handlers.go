package console

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
)

type createSessionRequest struct {
	Bee          string `json:"bee"`
	Task         string `json:"task"`
	RawPrompt    string `json:"rawPrompt"`
	TraceID      string `json:"traceId"`
	Intent       string `json:"intent"`
	UseRawPrompt bool   `json:"useRawPrompt"`
}

type api struct {
	ctx      colony.Context
	sessions *sessions.Manager
	runtime  *runtime.Supervisor
}

func (a *api) handleRuntime(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := GetRuntime(a.ctx, a.runtime)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, view)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *api) handleRuntimeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := StartRuntime(a.ctx, a.runtime)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleRuntimeStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := StopRuntime(a.ctx, a.runtime)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleBees(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bees, err := ListInteractiveBees(a.ctx.ColonyRoot)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, bees)
}

func (a *api) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := ListRuns(a.ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, list)
}

func (a *api) handleRunByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/runs/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	traceID := parts[0]
	agentID := parts[1]
	suffix := ""
	if len(parts) > 2 {
		suffix = parts[2]
	}

	if suffix == "events" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleRunEvents(w, r, traceID, agentID)
		return
	}
	if suffix != "" {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, ok, err := GetRun(a.ctx, traceID, agentID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleRunEvents(w http.ResponseWriter, r *http.Request, traceID, agentID string) {
	view, ok, err := GetRun(a.ctx, traceID, agentID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	after := 0
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			http.Error(w, "invalid after cursor", http.StatusBadRequest)
			return
		}
		after = n
	}

	d := runs.Dir{
		ColonyRoot: a.ctx.ColonyRoot,
		TraceID:    view.TraceID,
		AgentID:    view.AgentID,
	}
	entries, next, err := d.ReadEventsAfter(after)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, EventsPage{Entries: entries, NextCursor: next})
}

func (a *api) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, err := ListSessions(a.ctx, a.sessions)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, list)
	case http.MethodPost:
		a.createSession(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *api) createSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Bee == "" {
		http.Error(w, "bee is required", http.StatusBadRequest)
		return
	}

	runReq := sessions.RunRequest{
		StartDir: a.ctx.ColonyRoot,
		Bee:      req.Bee,
		TraceID:  req.TraceID,
		Intent:   req.Intent,
	}
	if req.UseRawPrompt {
		runReq.InlinePrompt = req.RawPrompt
	} else {
		runReq.Task = req.Task
	}
	if runReq.Task == "" && runReq.InlinePrompt == "" {
		http.Error(w, "task or raw prompt is required", http.StatusBadRequest)
		return
	}

	res, err := a.sessions.StartDetached(r.Context(), runReq)
	if err != nil {
		writeError(w, err)
		return
	}
	view, ok, err := GetSession(a.ctx, a.sessions, res.SessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		view = SessionView{
			SessionID: res.SessionID,
			TraceID:   res.TraceID,
			AgentID:   res.AgentID,
			Bee:       req.Bee,
			Workspace: res.Workspace,
			RunDir:    res.RunDir,
			State:     string(res.State),
			Active:    true,
		}
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, view)
}

func (a *api) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	if strings.HasSuffix(path, "/transcript") {
		sessionID := strings.TrimSuffix(path, "/transcript")
		sessionID = strings.Trim(sessionID, "/")
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleTranscript(w, r, sessionID)
		return
	}
	if strings.HasSuffix(path, "/stop") {
		sessionID := strings.TrimSuffix(path, "/stop")
		sessionID = strings.Trim(sessionID, "/")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleStop(w, r, sessionID)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, ok, err := GetSession(a.ctx, a.sessions, path)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleTranscript(w http.ResponseWriter, r *http.Request, sessionID string) {
	view, ok, err := GetSession(a.ctx, a.sessions, sessionID)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}

	after := 0
	if raw := r.URL.Query().Get("after"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			http.Error(w, "invalid after cursor", http.StatusBadRequest)
			return
		}
		after = n
	}

	d := runs.Dir{
		ColonyRoot: a.ctx.ColonyRoot,
		TraceID:    view.TraceID,
		AgentID:    view.AgentID,
	}
	entries, next, err := d.ReadTranscriptAfter(after)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, TranscriptPage{Entries: entries, NextCursor: next})
}

func (a *api) handleStop(w http.ResponseWriter, r *http.Request, sessionID string) {
	if err := a.sessions.Stop(sessionID); err == nil {
		writeJSON(w, map[string]string{"status": "stopped"})
		return
	}
	if err := sessions.StopRemote(a.ctx.Slug, sessionID); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "signalled"})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	msg := err.Error()
	status := http.StatusInternalServerError
	if strings.Contains(msg, "not found") {
		status = http.StatusNotFound
	} else if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		status = http.StatusBadRequest
	}
	http.Error(w, msg, status)
}
