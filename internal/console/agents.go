package console

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

const liveAgentsLimit = 50

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

// AgentsView is the Queen Console projection of live agent processes.
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

	afkItems, err := scanLiveAFKRuns(ctx.ColonyRoot)
	if err != nil {
		return AgentsView{}, err
	}
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

func scanLiveAFKRuns(colonyRoot string) ([]AgentItem, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []AgentItem
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		tracePath := filepath.Join(runsRoot, traceID)
		agentDirs, err := os.ReadDir(tracePath)
		if err != nil {
			continue
		}
		for _, agentEntry := range agentDirs {
			if !agentEntry.IsDir() || agentEntry.Name() == "tasks" {
				continue
			}
			agentID := agentEntry.Name()
			d := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
			if !fileExists(d.RequestPath()) {
				continue
			}
			if fileExists(d.SessionPath()) {
				continue
			}
			snap, err := d.ReadStatus()
			if err != nil {
				continue
			}
			if snap.State != protocol.StatusRunning || snap.PID <= 0 || !colony.ProcessAlive(snap.PID) {
				continue
			}
			req, err := d.ReadRequest()
			if err != nil {
				continue
			}
			startedAt := snap.StartedAt
			if startedAt.IsZero() {
				startedAt = req.CreatedAt
			}
			items = append(items, AgentItem{
				Kind:      "afk",
				Bee:       req.Bee,
				PID:       snap.PID,
				TraceID:   traceID,
				AgentID:   agentID,
				StartedAt: startedAt.UTC().Format(time.RFC3339),
				RunDir:    d.Root(),
			})
		}
	}
	return items, nil
}

func collectLiveSessions(ctx colony.Context, mgr *sessions.Manager) ([]AgentItem, error) {
	byID := map[string]AgentItem{}

	activeEntries, err := colony.ListSessions(ctx.Slug)
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

func agentItemFromSessionEntry(e colony.SessionEntry, colonyRoot string) AgentItem {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
