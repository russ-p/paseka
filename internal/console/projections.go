package console

import (
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

const (
	recentSessionLimit = 50
	recentRunLimit     = 50
)

var interactiveAdapters = map[string]bool{
	"cursor": true,
	"pi":     true,
}

// BeeView is a launchable interactive bee.
type BeeView struct {
	Role           string `json:"role"`
	Adapter        string `json:"adapter"`
	PromptTemplate string `json:"promptTemplate"`
	Worktree       bool   `json:"worktree"`
}

// SessionView is a console projection of one interactive session.
type SessionView struct {
	SessionID  string     `json:"sessionId"`
	TraceID    string     `json:"traceId"`
	AgentID    string     `json:"agentId"`
	Bee        string     `json:"bee"`
	Adapter    string     `json:"adapter,omitempty"`
	Workspace  string     `json:"workspace"`
	RunDir     string     `json:"runDir"`
	ColonyRoot string     `json:"colonyRoot,omitempty"`
	State      string     `json:"state"`
	PID        int        `json:"pid,omitempty"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	Active     bool       `json:"active"`
}

// TranscriptPage is a cursor-based transcript slice.
type TranscriptPage struct {
	Entries    []runs.TranscriptEntry `json:"entries"`
	NextCursor int                    `json:"nextCursor"`
}

// RunView is a console projection of one headless adapter run.
type RunView struct {
	TraceID    string     `json:"traceId"`
	AgentID    string     `json:"agentId"`
	Bee        string     `json:"bee"`
	Adapter    string     `json:"adapter"`
	Workspace  string     `json:"workspace"`
	ColonyRoot string     `json:"colonyRoot,omitempty"`
	TaskID     string     `json:"taskId,omitempty"`
	Task       string     `json:"task,omitempty"`
	Intent     string     `json:"intent,omitempty"`
	State      string     `json:"state"`
	Summary    string     `json:"summary,omitempty"`
	RunDir     string     `json:"runDir"`
	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	HasEvents  bool       `json:"hasEvents"`
	HasSession bool       `json:"hasSession"`
}

// EventsPage is a cursor-based events.ndjson slice.
type EventsPage struct {
	Entries    []protocol.Event `json:"entries"`
	NextCursor int              `json:"nextCursor"`
}

// ListInteractiveBees returns bees whose adapters support interactive sessions.
func ListInteractiveBees(colonyRoot string) ([]BeeView, error) {
	beesDir := colony.BeesDir(colonyRoot)
	entries, err := filepath.Glob(filepath.Join(beesDir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var out []BeeView
	for _, path := range entries {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".local.yaml") {
			continue
		}
		role := strings.TrimSuffix(base, ".yaml")
		bee, _, err := colony.LoadBee(colonyRoot, role)
		if err != nil {
			continue
		}
		adapterName, err := bee.ResolveAdapter()
		if err != nil || !interactiveAdapters[adapterName] {
			continue
		}
		out = append(out, BeeView{
			Role:           bee.Role,
			Adapter:        adapterName,
			PromptTemplate: bee.PromptTemplate,
			Worktree:       bee.Worktree,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Role < out[j].Role })
	return out, nil
}

// ListSessions merges active registry entries with recent filesystem history.
func ListSessions(ctx colony.Context, mgr *sessions.Manager) ([]SessionView, error) {
	byID := map[string]SessionView{}

	activeEntries, err := colony.ListSessions(ctx.Slug)
	if err != nil {
		return nil, err
	}
	for _, e := range activeEntries {
		view := sessionViewFromRegistry(e, ctx.ColonyRoot)
		view.Active = true
		view.State = string(adapters.SessionActive)
		byID[e.SessionID] = view
	}

	for _, e := range mgr.ListActive() {
		view := sessionViewFromHandle(e.Handle, e.RunDir.Root())
		view.Active = true
		byID[e.Handle.SessionID] = view
	}

	recent, err := runs.ScanRecentSessions(ctx.ColonyRoot, recentSessionLimit)
	if err != nil {
		return nil, err
	}
	for _, meta := range recent {
		if _, ok := byID[meta.SessionID]; ok {
			continue
		}
		byID[meta.SessionID] = sessionViewFromMeta(meta)
	}

	out := make([]SessionView, 0, len(byID))
	for _, v := range byID {
		out = append(out, v)
	}
	sortSessions(out)
	return out, nil
}

// GetSession returns one session by ID from active manager state or run artifacts.
func GetSession(ctx colony.Context, mgr *sessions.Manager, sessionID string) (SessionView, bool, error) {
	if entry, ok := mgr.Get(sessionID); ok {
		view := sessionViewFromHandle(entry.Handle, entry.RunDir.Root())
		view.Active = true
		return view, true, nil
	}

	if reg, err := colony.FindSession(ctx.Slug, sessionID); err == nil {
		view := sessionViewFromRegistry(reg, ctx.ColonyRoot)
		view.Active = true
		view.State = string(adapters.SessionActive)
		return view, true, nil
	}

	recent, err := runs.ScanRecentSessions(ctx.ColonyRoot, recentSessionLimit*4)
	if err != nil {
		return SessionView{}, false, err
	}
	for _, meta := range recent {
		if meta.SessionID == sessionID {
			return sessionViewFromMeta(meta), true, nil
		}
	}

	if meta, ok, err := runs.FindSessionMeta(ctx.ColonyRoot, sessionID); err != nil {
		return SessionView{}, false, err
	} else if ok {
		return sessionViewFromMeta(meta), true, nil
	}
	return SessionView{}, false, nil
}

// ListRuns returns recent headless adapter runs from the filesystem.
func ListRuns(ctx colony.Context) ([]RunView, error) {
	metas, err := runs.ScanRecentRuns(ctx.ColonyRoot, recentRunLimit)
	if err != nil {
		return nil, err
	}
	out := make([]RunView, 0, len(metas))
	for _, meta := range metas {
		out = append(out, runViewFromMeta(meta))
	}
	sortRuns(out)
	return out, nil
}

// GetRun returns one run by trace and agent identifiers.
func GetRun(ctx colony.Context, traceID, agentID string) (RunView, bool, error) {
	meta, ok, err := runs.FindRun(ctx.ColonyRoot, traceID, agentID)
	if err != nil {
		return RunView{}, false, err
	}
	if !ok {
		return RunView{}, false, nil
	}
	return runViewFromMeta(meta), true, nil
}

func runViewFromMeta(meta runs.RunMeta) RunView {
	view := RunView{
		TraceID:    meta.TraceID,
		AgentID:    meta.AgentID,
		Bee:        meta.Bee,
		Adapter:    meta.Adapter,
		Workspace:  meta.Workspace,
		ColonyRoot: meta.ColonyRoot,
		TaskID:     meta.TaskID,
		Task:       meta.Task,
		Intent:     meta.Intent,
		State:      meta.State,
		Summary:    meta.Summary,
		RunDir:     meta.RunDir,
		StartedAt:  meta.StartedAt,
		HasEvents:  meta.HasEvents,
		HasSession: meta.HasSession,
	}
	if !meta.FinishedAt.IsZero() {
		finished := meta.FinishedAt
		view.FinishedAt = &finished
	}
	return view
}

func sessionViewFromHandle(h adapters.SessionHandle, runDir string) SessionView {
	state := string(h.State)
	if state == "" {
		state = string(adapters.SessionActive)
	}
	return SessionView{
		SessionID:  h.SessionID,
		TraceID:    h.TraceID,
		AgentID:    h.AgentID,
		Bee:        h.Bee,
		Adapter:    h.Adapter,
		Workspace:  h.Workspace,
		RunDir:     runDir,
		ColonyRoot: h.ColonyRoot,
		State:      state,
		PID:        h.PID,
		StartedAt:  h.StartedAt,
		Active:     h.State == adapters.SessionActive || h.State == "",
	}
}

func sessionViewFromRegistry(e colony.SessionEntry, colonyRoot string) SessionView {
	runDir := e.RunDir
	if runDir == "" {
		runDir = runs.Dir{
			ColonyRoot: colonyRoot,
			TraceID:    e.TraceID,
			AgentID:    e.AgentID,
		}.Root()
	}
	return SessionView{
		SessionID: e.SessionID,
		TraceID:   e.TraceID,
		AgentID:   e.AgentID,
		Bee:       e.Bee,
		Workspace: colonyRoot,
		RunDir:    runDir,
		State:     string(adapters.SessionActive),
		PID:       e.PID,
		StartedAt: e.StartedAt,
		Active:    true,
	}
}

func sessionViewFromMeta(meta runs.SessionMeta) SessionView {
	view := SessionView{
		SessionID:  meta.SessionID,
		TraceID:    meta.TraceID,
		AgentID:    meta.AgentID,
		Bee:        meta.Bee,
		Adapter:    meta.Adapter,
		Workspace:  meta.Workspace,
		RunDir:     runs.Dir{ColonyRoot: meta.ColonyRoot, TraceID: meta.TraceID, AgentID: meta.AgentID}.Root(),
		ColonyRoot: meta.ColonyRoot,
		State:      meta.State,
		PID:        meta.PID,
		StartedAt:  meta.StartedAt,
		Active:     meta.State == string(adapters.SessionActive),
	}
	if !meta.FinishedAt.IsZero() {
		finished := meta.FinishedAt
		view.FinishedAt = &finished
	}
	return view
}

func sortSessions(out []SessionView) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
}

func sortRuns(out []RunView) {
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
}
