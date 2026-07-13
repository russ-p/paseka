package console

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/invites"
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

func (a *api) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := GetAgents(a.ctx, a.sessions)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
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

func (a *api) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := GetDashboard(a.ctx, a.runtime, a.sessions)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleReviewQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := ListReviewQueue(a.ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		view, err := ListTaskBoard(a.ctx)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, view)
	case http.MethodPost:
		a.createTask(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *api) createTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Title == "" && strings.TrimSpace(req.Body) == "" {
		http.Error(w, "title or body is required", http.StatusBadRequest)
		return
	}

	res, err := CreateTask(r.Context(), a.ctx, req)
	if err != nil {
		writeTaskError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, res)
}

func (a *api) handleTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	list, err := ListTraces(a.ctx, 20)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, list)
}

func (a *api) handleTraceByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/traces/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	traceID := parts[0]
	suffix := ""
	if len(parts) > 1 {
		suffix = parts[1]
	}

	if suffix == "events" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.handleTraceEvents(w, r, traceID)
		return
	}
	if suffix == "merge-diff" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		view, err := GetMergeDiff(a.ctx, traceID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, view)
		return
	}
	if suffix == "tasks" || (len(parts) > 1 && parts[1] == "tasks") {
		a.handleTraceTasks(w, r, traceID, parts[1:])
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
	view, ok, err := GetTrace(a.ctx, traceID)
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

func (a *api) handleTraceEvents(w http.ResponseWriter, r *http.Request, traceID string) {
	filter := ParseEventFilter(r.URL.Query())
	filter.TraceID = traceID
	page, err := ListEventFeed(a.ctx, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, page)
}

func (a *api) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filter := ParseEventFilter(r.URL.Query())
	page, err := ListEventFeed(a.ctx, filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, page)
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

func (a *api) handleInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status := r.URL.Query().Get("status")
	list, err := ListInvites(a.ctx, status)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, list)
}

func (a *api) handleInviteByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/invites/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(path, "/")
	inviteID := parts[0]
	if inviteID == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		http.NotFound(w, r)
		return
	}
	action := parts[1]
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch action {
	case "accept":
		a.acceptInvite(w, r, inviteID)
	case "reject":
		a.rejectInvite(w, r, inviteID)
	default:
		http.NotFound(w, r)
	}
}

func (a *api) acceptInvite(w http.ResponseWriter, r *http.Request, inviteID string) {
	client, err := bus.ConnectColony(a.ctx, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if client == nil {
		http.Error(w, "nats url not configured", http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	svc := &invites.Service{
		Colony:   a.ctx,
		Bus:      client,
		Sessions: a.sessions,
	}
	res, err := svc.Accept(r.Context(), inviteID, false)
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
			Bee:       res.Invite.Bee,
			State:     string(adapters.SessionActive),
			Active:    true,
		}
	}
	writeJSON(w, map[string]any{
		"inviteId":  res.Invite.InviteID,
		"traceId":   res.TraceID,
		"sessionId": res.SessionID,
		"session":   view,
	})
}

func (a *api) rejectInvite(w http.ResponseWriter, r *http.Request, inviteID string) {
	client, err := bus.ConnectColony(a.ctx, false)
	if err != nil {
		writeError(w, err)
		return
	}
	if client == nil {
		http.Error(w, "nats url not configured", http.StatusServiceUnavailable)
		return
	}
	defer client.Close()

	svc := &invites.Service{Colony: a.ctx, Bus: client}
	invite, err := svc.Reject(r.Context(), inviteID, false)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, InviteView{
		InviteID:  invite.InviteID,
		TraceID:   invite.TraceID,
		Bee:       invite.Bee,
		Intent:    invite.Intent,
		Task:      invite.Task,
		Status:    invite.Status,
		SpecRef:   invite.SpecRef,
		SessionID: invite.SessionID,
		CreatedAt: invite.CreatedAt,
		UpdatedAt: invite.UpdatedAt,
	})
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
	if sessionID, ok := parseSessionPTYPath(path); ok {
		a.handleSessionPTY(w, r, sessionID)
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

func (a *api) handleTraceTasks(w http.ResponseWriter, r *http.Request, traceID string, parts []string) {
	// parts[0] is always "tasks"
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		list, err := ListTraceTasks(a.ctx, traceID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, list)
		return
	}

	if len(parts) == 2 && parts[1] == "start" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := StartTask(r.Context(), a.ctx, traceID, "")
		if err != nil {
			writeTaskError(w, err)
			return
		}
		writeJSON(w, res)
		return
	}

	taskID := parts[1]
	if len(parts) == 2 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		view, ok, err := GetTask(a.ctx, traceID, taskID)
		if err != nil {
			writeError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, view)
		return
	}

	if len(parts) == 3 && parts[2] == "start" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		res, err := StartTask(r.Context(), a.ctx, traceID, taskID)
		if err != nil {
			writeTaskError(w, err)
			return
		}
		writeJSON(w, res)
		return
	}

	if len(parts) == 3 && parts[2] == "approve" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.approveTask(w, r, traceID, taskID)
		return
	}

	if len(parts) == 3 && parts[2] == "reject" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.rejectTask(w, r, traceID, taskID)
		return
	}

	http.NotFound(w, r)
}

func (a *api) approveTask(w http.ResponseWriter, r *http.Request, traceID, taskID string) {
	var req ApproveTaskRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	res, err := ApproveTask(r.Context(), a.ctx, traceID, taskID, req)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, res)
}

func (a *api) rejectTask(w http.ResponseWriter, r *http.Request, traceID, taskID string) {
	var req RejectTaskRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	res, err := RejectTask(r.Context(), a.ctx, traceID, taskID, req)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	writeJSON(w, res)
}

func writeReviewError(w http.ResponseWriter, err error) {
	msg := err.Error()
	status := http.StatusInternalServerError
	if strings.Contains(strings.ToLower(msg), "not configured") {
		status = http.StatusServiceUnavailable
	} else if strings.Contains(msg, "not found") {
		status = http.StatusNotFound
	} else if strings.Contains(msg, "required") || strings.Contains(msg, "expected") || strings.Contains(msg, "not a review") {
		status = http.StatusBadRequest
	}
	http.Error(w, msg, status)
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

func writeTaskError(w http.ResponseWriter, err error) {
	msg := mapTaskError(err)
	if msg == "" {
		msg = err.Error()
	}
	status := http.StatusInternalServerError
	if strings.Contains(strings.ToLower(msg), "not configured") {
		status = http.StatusServiceUnavailable
	} else if isTaskClientError(err) {
		status = http.StatusBadRequest
	}
	http.Error(w, msg, status)
}
