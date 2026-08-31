package hiveview

import (
	"sort"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

const recentRunLimit = 50

// RunView is a projection of one headless adapter run.
type RunView struct {
	TraceID           string          `json:"traceId"`
	AgentID           string          `json:"agentId"`
	Bee               string          `json:"bee"`
	Adapter           string          `json:"adapter"`
	Workspace         string          `json:"workspace"`
	ColonyRoot        string          `json:"colonyRoot,omitempty"`
	TaskID            string          `json:"taskId,omitempty"`
	Body              string          `json:"body,omitempty"`
	Intent            string          `json:"intent,omitempty"`
	State             string          `json:"state"`
	Summary           string          `json:"summary,omitempty"`
	Usage             *protocol.Usage `json:"usage,omitempty"`
	ProviderSessionID string          `json:"providerSessionId,omitempty"`
	RunDir            string          `json:"runDir"`
	StartedAt         time.Time       `json:"startedAt"`
	FinishedAt        *time.Time      `json:"finishedAt,omitempty"`
	HasEvents         bool            `json:"hasEvents"`
	HasSession        bool            `json:"hasSession"`
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

// SortRunsAsc sorts runs oldest-first by StartedAt.
func SortRunsAsc(out []RunView) {
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
}

func runViewFromMeta(meta runs.RunMeta) RunView {
	view := RunView{
		TraceID:           meta.TraceID,
		AgentID:           meta.AgentID,
		Bee:               meta.Bee,
		Adapter:           meta.Adapter,
		Workspace:         meta.Workspace,
		ColonyRoot:        meta.ColonyRoot,
		TaskID:            meta.TaskID,
		Body:              meta.Task,
		Intent:            meta.Intent,
		State:             meta.State,
		Summary:           meta.Summary,
		Usage:             meta.Usage,
		ProviderSessionID: meta.ProviderSessionID,
		RunDir:            meta.RunDir,
		StartedAt:         meta.StartedAt,
		HasEvents:         meta.HasEvents,
		HasSession:        meta.HasSession,
	}
	if !meta.FinishedAt.IsZero() {
		finished := meta.FinishedAt
		view.FinishedAt = &finished
	}
	return view
}

func sortRuns(out []RunView) {
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
}
