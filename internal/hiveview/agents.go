package hiveview

import (
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/liveafk"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

const liveAgentsLimit = 50

// LiveAFKOnTrace returns live headless adapter runs for one Flight Trail.
// Interactive sessions are omitted (they are not a standing tick).
func LiveAFKOnTrace(colonyRoot, traceID string) ([]AgentItem, error) {
	found, err := liveafk.OnTrace(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	items := agentItemsFromLiveAFK(found)
	sortLiveAgents(items)
	return items, nil
}

// AgentItem is one live AFK or interactive session process.
type AgentItem struct {
	Kind      string `json:"kind"`
	Bee       string `json:"bee"`
	PID       int    `json:"pid"`
	TraceID   string `json:"traceId"`
	AgentID   string `json:"agentId"`
	SessionID string `json:"sessionId,omitempty"`
	StartedAt string `json:"startedAt"`
	RunDir    string `json:"runDir"`
}

// AgentsView is a projection of live agent processes.
type AgentsView struct {
	Count    int         `json:"count"`
	AFK      int         `json:"afk"`
	Sessions int         `json:"sessions"`
	Items    []AgentItem `json:"items"`
}

// GetAgents returns live AFK runs and interactive sessions with alive PIDs.
func GetAgents(ctx colony.Context, mgr *sessions.Manager) (AgentsView, error) {
	if mgr == nil {
		mgr = sessions.NewManager()
	}

	afkRuns, err := liveafk.ScanColony(ctx.ColonyRoot)
	if err != nil {
		return AgentsView{}, err
	}
	afkItems := agentItemsFromLiveAFK(afkRuns)
	sessionItems, err := collectLiveSessions(ctx, mgr)
	if err != nil {
		return AgentsView{}, err
	}

	items := append(afkItems, sessionItems...)
	sortLiveAgents(items)
	if len(items) > liveAgentsLimit {
		items = items[:liveAgentsLimit]
	}

	afk := 0
	sess := 0
	for _, item := range items {
		switch item.Kind {
		case "afk":
			afk++
		case "session":
			sess++
		}
	}

	return AgentsView{
		Count:    len(items),
		AFK:      afk,
		Sessions: sess,
		Items:    items,
	}, nil
}

func agentItemsFromLiveAFK(found []liveafk.Run) []AgentItem {
	items := make([]AgentItem, 0, len(found))
	for _, run := range found {
		items = append(items, AgentItem{
			Kind:      "afk",
			Bee:       run.Bee,
			PID:       run.PID,
			TraceID:   run.TraceID,
			AgentID:   run.AgentID,
			StartedAt: run.StartedAt.UTC().Format(time.RFC3339),
			RunDir:    run.RunDir,
		})
	}
	return items
}

func collectLiveSessions(ctx colony.Context, mgr *sessions.Manager) ([]AgentItem, error) {
	byID := map[string]AgentItem{}

	activeEntries, err := homestate.ListSessions(ctx.Slug)
	if err != nil {
		return nil, err
	}
	for _, e := range activeEntries {
		if e.PID <= 0 || !colony.ProcessAlive(e.PID) {
			continue
		}
		byID[e.SessionID] = agentItemFromSessionEntry(e, ctx.ColonyRoot)
	}

	for _, e := range mgr.ListActive() {
		h := e.Handle
		if h.PID <= 0 || !colony.ProcessAlive(h.PID) {
			continue
		}
		byID[h.SessionID] = agentItemFromHandle(h, e.RunDir.Root())
	}

	out := make([]AgentItem, 0, len(byID))
	for _, item := range byID {
		out = append(out, item)
	}
	return out, nil
}

func agentItemFromSessionEntry(e homestate.SessionEntry, colonyRoot string) AgentItem {
	runDir := e.RunDir
	if runDir == "" {
		runDir = runs.Dir{
			ColonyRoot: colonyRoot,
			TraceID:    e.TraceID,
			AgentID:    e.AgentID,
		}.Root()
	}
	return AgentItem{
		Kind:      "session",
		Bee:       e.Bee,
		PID:       e.PID,
		TraceID:   e.TraceID,
		AgentID:   e.AgentID,
		SessionID: e.SessionID,
		StartedAt: e.StartedAt.UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}
}

func agentItemFromHandle(h adapters.SessionHandle, runDir string) AgentItem {
	return AgentItem{
		Kind:      "session",
		Bee:       h.Bee,
		PID:       h.PID,
		TraceID:   h.TraceID,
		AgentID:   h.AgentID,
		SessionID: h.SessionID,
		StartedAt: h.StartedAt.UTC().Format(time.RFC3339),
		RunDir:    runDir,
	}
}

func sortLiveAgents(items []AgentItem) {
	sort.SliceStable(items, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, items[i].StartedAt)
		tj, _ := time.Parse(time.RFC3339, items[j].StartedAt)
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		return items[i].Bee < items[j].Bee
	})
}
