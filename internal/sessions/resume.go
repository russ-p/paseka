package sessions

import (
	"context"
	"errors"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/runs"
)

const (
	ResumeErrStillActive         = "still_active"
	ResumeErrProviderSessionBusy = "provider_session_busy"
	ResumeErrNotCursor           = "not_cursor"
	ResumeErrNoProviderSessionID = "no_provider_session_id"
	ResumeErrCommandOverride     = "command_override"
	ResumeErrAdapterChanged      = "adapter_changed"
	ResumeErrBeeGone             = "bee_gone"
)

// ResumeError is an eligibility failure for Cursor HITL resume.
type ResumeError struct {
	Code    string
	Message string
	Status  int
}

func (e *ResumeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

func resumeNotFound() *ResumeError {
	return &ResumeError{Status: 404, Message: "session not found"}
}

func resumeErr(code string, status int, msg string) *ResumeError {
	return &ResumeError{Code: code, Status: status, Message: msg}
}

// AsResumeError extracts a ResumeError from err.
func AsResumeError(err error) (*ResumeError, bool) {
	var re *ResumeError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}

// ResumeRequest continues a finished Cursor HITL session.
type ResumeRequest struct {
	StartDir  string
	SessionID string
	Continue  string
	// Ready is called after the new session is launched and before the
	// current terminal is attached (interactive path only).
	Ready func(*RunResult)
}

// ResumeDetached starts a continuation owned by this process for Console PTY attach.
func (m *Manager) ResumeDetached(ctx context.Context, req ResumeRequest) (*RunResult, error) {
	runReq, err := m.prepareResume(req)
	if err != nil {
		return nil, err
	}
	return m.StartDetached(ctx, runReq)
}

// ResumeInteractive starts a continuation and attaches the current terminal.
func (m *Manager) ResumeInteractive(ctx context.Context, req ResumeRequest) (*RunResult, error) {
	runReq, err := m.prepareResume(req)
	if err != nil {
		return nil, err
	}
	active, err := m.launch(ctx, runReq, false)
	if err != nil {
		return nil, err
	}
	result := launchResult(active)
	if req.Ready != nil {
		req.Ready(result)
	}
	if err := attachPTY(active.process); err != nil {
		_ = m.stopSession(active.entry.Handle.SessionID)
		return nil, err
	}
	<-active.done
	out := launchResult(active)
	if meta, err := active.entry.RunDir.ReadSession(); err == nil {
		out.State = adapters.SessionState(meta.State)
	}
	return out, nil
}

// prepareResume checks eligibility and builds a RunRequest for launch.
func (m *Manager) prepareResume(req ResumeRequest) (RunRequest, error) {
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		return RunRequest{}, resumeNotFound()
	}
	ctxColony, err := colony.ResolveContext(req.StartDir)
	if err != nil {
		return RunRequest{}, err
	}
	source, ok, err := m.findSessionMeta(ctxColony, sessionID)
	if err != nil {
		return RunRequest{}, err
	}
	if !ok {
		return RunRequest{}, resumeNotFound()
	}
	if live, err := m.sessionIsActive(ctxColony.Slug, sessionID); err != nil {
		return RunRequest{}, err
	} else if live {
		return RunRequest{}, resumeErr(ResumeErrStillActive, 409, "source session is still active")
	}
	if strings.TrimSpace(source.Adapter) != "cursor" {
		return RunRequest{}, resumeErr(ResumeErrNotCursor, 400, "source session is not a Cursor HITL chat")
	}
	providerID := strings.TrimSpace(source.ProviderSessionID)
	if providerID == "" {
		return RunRequest{}, resumeErr(ResumeErrNoProviderSessionID, 400, "source session has no providerSessionId")
	}
	bee, _, err := ctxColony.LoadBee(source.Bee)
	if err != nil {
		return RunRequest{}, resumeErr(ResumeErrBeeGone, 400, "source bee is gone or unreadable")
	}
	adapterName, err := bee.ResolveAdapter()
	if err != nil || adapterName != "cursor" {
		return RunRequest{}, resumeErr(ResumeErrAdapterChanged, 400, "source bee no longer uses the Cursor session adapter")
	}
	m.mu.RLock()
	_, adapterOK := m.adapters[adapterName]
	m.mu.RUnlock()
	if !adapterOK {
		return RunRequest{}, resumeErr(ResumeErrAdapterChanged, 400, "source bee no longer uses the Cursor session adapter")
	}
	if bee.Command.IsSet() {
		return RunRequest{}, resumeErr(ResumeErrCommandOverride, 400, "source bee uses a command override")
	}
	busy, err := m.providerSessionBusy(ctxColony, providerID, sessionID)
	if err != nil {
		return RunRequest{}, err
	}
	if busy {
		return RunRequest{}, resumeErr(ResumeErrProviderSessionBusy, 409, "an active session already uses this providerSessionId")
	}

	return RunRequest{
		StartDir:         req.StartDir,
		Bee:              source.Bee,
		TraceID:          source.TraceID,
		Task:             strings.TrimSpace(req.Continue),
		resumeFrom:       sessionID,
		resumeProviderID: providerID,
	}, nil
}

func (m *Manager) findSessionMeta(ctxColony colony.Context, sessionID string) (runs.SessionMeta, bool, error) {
	if entry, ok := m.Get(sessionID); ok {
		if meta, err := entry.RunDir.ReadSession(); err == nil {
			return meta, true, nil
		}
		h := entry.Handle
		return runs.SessionMeta{
			SessionID:         h.SessionID,
			TraceID:           h.TraceID,
			AgentID:           h.AgentID,
			Bee:               h.Bee,
			Adapter:           h.Adapter,
			Workspace:         h.Workspace,
			ColonyRoot:        h.ColonyRoot,
			PID:               h.PID,
			State:             string(h.State),
			ProviderSessionID: h.ProviderSessionID,
			ResumedFrom:       h.ResumedFrom,
			StartedAt:         h.StartedAt,
		}, true, nil
	}
	if reg, err := homestate.FindSession(ctxColony.Slug, sessionID); err == nil {
		d := runs.Dir{ColonyRoot: ctxColony.ColonyRoot, TraceID: reg.TraceID, AgentID: reg.AgentID}
		if meta, err := d.ReadSession(); err == nil {
			return meta, true, nil
		}
		return runs.SessionMeta{
			SessionID:  reg.SessionID,
			TraceID:    reg.TraceID,
			AgentID:    reg.AgentID,
			Bee:        reg.Bee,
			ColonyRoot: ctxColony.ColonyRoot,
			PID:        reg.PID,
			State:      "active",
			StartedAt:  reg.StartedAt,
		}, true, nil
	}
	return runs.FindSessionMeta(ctxColony.ColonyRoot, sessionID)
}

func (m *Manager) sessionIsActive(slug, sessionID string) (bool, error) {
	if _, ok := m.Get(sessionID); ok {
		return true, nil
	}
	reg, err := homestate.FindSession(slug, sessionID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return colony.ProcessAlive(reg.PID), nil
}

func (m *Manager) providerSessionBusy(ctxColony colony.Context, providerID, sourceSessionID string) (bool, error) {
	seen := map[string]struct{}{}
	for _, e := range m.ListActive() {
		id := strings.TrimSpace(e.Handle.ProviderSessionID)
		if id == "" {
			if meta, err := e.RunDir.ReadSession(); err == nil {
				id = strings.TrimSpace(meta.ProviderSessionID)
			}
		}
		seen[e.Handle.SessionID] = struct{}{}
		if e.Handle.SessionID != sourceSessionID && id == providerID {
			return true, nil
		}
	}
	entries, err := homestate.ListSessions(ctxColony.Slug)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.SessionID == sourceSessionID {
			continue
		}
		if _, ok := seen[e.SessionID]; ok {
			continue
		}
		if !colony.ProcessAlive(e.PID) {
			continue
		}
		id := ""
		d := runs.Dir{ColonyRoot: ctxColony.ColonyRoot, TraceID: e.TraceID, AgentID: e.AgentID}
		if meta, err := d.ReadSession(); err == nil {
			id = strings.TrimSpace(meta.ProviderSessionID)
		}
		if id == providerID {
			return true, nil
		}
	}
	return false, nil
}
