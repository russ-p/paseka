package runs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

// RunMeta is a read-only projection of one headless adapter run directory.
type RunMeta struct {
	TraceID    string    `json:"traceId"`
	AgentID    string    `json:"agentId"`
	Bee        string    `json:"bee"`
	Adapter    string    `json:"adapter"`
	Workspace  string    `json:"workspace"`
	ColonyRoot string    `json:"colonyRoot"`
	TaskID     string    `json:"taskId,omitempty"`
	Task       string    `json:"task,omitempty"`
	Intent     string    `json:"intent,omitempty"`
	State      string    `json:"state"`
	Summary    string    `json:"summary,omitempty"`
	RunDir     string    `json:"runDir"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	HasEvents  bool      `json:"hasEvents"`
	HasSession bool      `json:"hasSession"`
}

// LoadRunMeta reads a run projection from an existing run directory.
func LoadRunMeta(d Dir) (RunMeta, error) {
	req, err := d.ReadRequest()
	if err != nil {
		return RunMeta{}, err
	}

	meta := RunMeta{
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
		Bee:        req.Bee,
		Adapter:    req.Adapter,
		Workspace:  req.Workspace,
		ColonyRoot: req.ColonyRoot,
		TaskID:     req.TaskID,
		Task:       req.Task,
		Intent:     req.Intent,
		RunDir:     d.Root(),
		StartedAt:  req.CreatedAt,
	}

	if snap, err := d.ReadStatus(); err == nil {
		if !snap.StartedAt.IsZero() {
			meta.StartedAt = snap.StartedAt
		}
		if snap.State != "" {
			meta.State = string(snap.State)
		}
		if !snap.FinishedAt.IsZero() {
			meta.FinishedAt = snap.FinishedAt
		}
	}

	if res, err := d.ReadResultJSON(); err == nil {
		if res.Status != "" {
			meta.State = string(res.Status)
		}
		if res.Summary != "" {
			meta.Summary = res.Summary
		}
		if !res.FinishedAt.IsZero() {
			meta.FinishedAt = res.FinishedAt
		}
	} else if summary, err := d.ReadResult(); err == nil {
		meta.Summary = summary
	}

	if meta.State == "" {
		meta.State = string(protocol.StatusQueued)
	}
	if meta.StartedAt.IsZero() {
		if legacy, err := readLegacyMetaStartedAt(d); err == nil {
			meta.StartedAt = legacy
		}
	}

	meta.HasEvents = fileExists(d.EventsPath())
	meta.HasSession = fileExists(d.SessionPath())
	return meta, nil
}

// FindRun loads one run by trace and agent identifiers.
func FindRun(colonyRoot, traceID, agentID string) (RunMeta, bool, error) {
	if colonyRoot == "" || traceID == "" || agentID == "" {
		return RunMeta{}, false, nil
	}
	d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	if !fileExists(d.RequestPath()) {
		return RunMeta{}, false, nil
	}
	meta, err := LoadRunMeta(d)
	if err != nil {
		return RunMeta{}, false, err
	}
	return meta, true, nil
}

// ScanRecentRuns walks .paseka/runs and returns up to limit run directories with
// request.json, newest first by StartedAt.
func ScanRecentRuns(colonyRoot string, limit int) ([]RunMeta, error) {
	if limit <= 0 {
		return nil, nil
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []RunMeta
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		traceMetas, err := listRunsInTraceDir(colonyRoot, traceID, filepath.Join(runsRoot, traceID))
		if err != nil {
			continue
		}
		metas = append(metas, traceMetas...)
	}

	sortRunsByStartedAt(metas)
	if len(metas) > limit {
		metas = metas[:limit]
	}
	return metas, nil
}

// ListRunsForTrace returns every valid run under .paseka/runs/<traceId>/,
// newest first by StartedAt. The tasks projection directory is ignored.
func ListRunsForTrace(colonyRoot, traceID string) ([]RunMeta, error) {
	if colonyRoot == "" || traceID == "" {
		return nil, nil
	}
	tracePath := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	metas, err := listRunsInTraceDir(colonyRoot, traceID, tracePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sortRunsByStartedAt(metas)
	return metas, nil
}

func listRunsInTraceDir(colonyRoot, traceID, tracePath string) ([]RunMeta, error) {
	agentDirs, err := os.ReadDir(tracePath)
	if err != nil {
		return nil, err
	}
	var metas []RunMeta
	for _, agentEntry := range agentDirs {
		if !agentEntry.IsDir() || agentEntry.Name() == "tasks" {
			continue
		}
		d := Dir{
			ColonyRoot: colonyRoot,
			TraceID:    traceID,
			AgentID:    agentEntry.Name(),
		}
		if !fileExists(d.RequestPath()) {
			continue
		}
		meta, err := LoadRunMeta(d)
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// ReadEventsAfter returns protocol events with index > after and the next cursor.
func (d Dir) ReadEventsAfter(after int) ([]protocol.Event, int, error) {
	events, err := d.readEventsFrom(after)
	if err != nil {
		return nil, after, err
	}
	next := after + len(events)
	return events, next, nil
}

func (d Dir) readEventsFrom(skip int) ([]protocol.Event, error) {
	all, err := d.ReadEvents()
	if err != nil {
		return nil, err
	}
	if skip >= len(all) {
		return nil, nil
	}
	return all[skip:], nil
}

func readLegacyMetaStartedAt(d Dir) (time.Time, error) {
	data, err := os.ReadFile(d.MetaPath())
	if err != nil {
		return time.Time{}, err
	}
	var legacy Meta
	if err := json.Unmarshal(data, &legacy); err != nil {
		return time.Time{}, err
	}
	return legacy.StartedAt, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortRunsByStartedAt(metas []RunMeta) {
	for i := 0; i < len(metas); i++ {
		for j := i + 1; j < len(metas); j++ {
			if metas[j].StartedAt.After(metas[i].StartedAt) {
				metas[i], metas[j] = metas[j], metas[i]
			}
		}
	}
}
