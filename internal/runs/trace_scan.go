package runs

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

const defaultTraceScanLimit = 50

// TraceSummary is a filesystem projection of activity under one trace directory.
type TraceSummary struct {
	TraceID        string    `json:"traceId"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	RunCount       int       `json:"runCount"`
	TaskCount      int       `json:"taskCount"`
	Bees           []string  `json:"bees,omitempty"`
	HasFailures    bool      `json:"hasFailures"`
	HasActive      bool      `json:"hasActive"`
}

// ScannedEvent pairs a protocol event with optional run metadata enrichment.
type ScannedEvent struct {
	Event protocol.Event `json:"event"`
	Bee   string         `json:"bee,omitempty"`
}

// ScanRecentTraces walks .paseka/runs and returns trace summaries ordered by
// last activity, newest first.
func ScanRecentTraces(colonyRoot string, limit int) ([]TraceSummary, error) {
	if limit <= 0 {
		limit = defaultTraceScanLimit
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var summaries []TraceSummary
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		summary, err := loadTraceSummary(colonyRoot, traceID)
		if err != nil {
			continue
		}
		if summary.RunCount == 0 && summary.TaskCount == 0 {
			continue
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivityAt.After(summaries[j].LastActivityAt)
	})
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

// ScanRecentEvents loads events from recent traces and returns them newest-first.
// traceLimit bounds how many trace directories are scanned; eventLimit caps results.
func ScanRecentEvents(colonyRoot string, traceLimit, eventLimit int) ([]ScannedEvent, error) {
	if traceLimit <= 0 {
		traceLimit = defaultTraceScanLimit
	}
	if eventLimit <= 0 {
		eventLimit = 100
	}

	traces, err := ScanRecentTraces(colonyRoot, traceLimit)
	if err != nil {
		return nil, err
	}

	beeByAgent := map[string]string{}
	var out []ScannedEvent
	for _, trace := range traces {
		events, err := ReadTraceEvents(colonyRoot, trace.TraceID)
		if err != nil {
			return nil, err
		}
		for _, ev := range events {
			bee := beeForEvent(colonyRoot, trace.TraceID, ev.AgentID, beeByAgent)
			out = append(out, ScannedEvent{Event: ev, Bee: bee})
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Event, out[j].Event
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.After(b.CreatedAt)
		}
		if a.Seq != b.Seq {
			return a.Seq > b.Seq
		}
		if a.TraceID != b.TraceID {
			return a.TraceID > b.TraceID
		}
		return a.AgentID > b.AgentID
	})

	if len(out) > eventLimit {
		out = out[:eventLimit]
	}
	return out, nil
}

// LoadTraceSummary builds a trace summary from filesystem projections.
func LoadTraceSummary(colonyRoot, traceID string) (TraceSummary, error) {
	return loadTraceSummary(colonyRoot, traceID)
}

func loadTraceSummary(colonyRoot, traceID string) (TraceSummary, error) {
	summary := TraceSummary{TraceID: traceID}
	traceRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(traceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return summary, nil
		}
		return summary, err
	}

	bees := map[string]struct{}{}
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		name := ent.Name()
		if name == "tasks" {
			taskIDs, err := ListTraceTaskIDs(colonyRoot, traceID)
			if err != nil {
				return summary, err
			}
			summary.TaskCount = len(taskIDs)
			continue
		}

		d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: name}
		if !fileExists(d.RequestPath()) {
			continue
		}
		meta, err := LoadRunMeta(d)
		if err != nil {
			continue
		}
		summary.RunCount++
		if meta.Bee != "" {
			bees[meta.Bee] = struct{}{}
		}
		activityAt := meta.StartedAt
		if !meta.FinishedAt.IsZero() && meta.FinishedAt.After(activityAt) {
			activityAt = meta.FinishedAt
		}
		if activityAt.After(summary.LastActivityAt) {
			summary.LastActivityAt = activityAt
		}
		switch meta.State {
		case string(protocol.StatusFailed), string(protocol.StatusCancelled):
			summary.HasFailures = true
		case string(protocol.StatusRunning), string(protocol.StatusQueued):
			summary.HasActive = true
		}
	}

	for bee := range bees {
		summary.Bees = append(summary.Bees, bee)
	}
	sort.Strings(summary.Bees)
	return summary, nil
}

func beeForEvent(colonyRoot, traceID, agentID string, cache map[string]string) string {
	key := traceID + "/" + agentID
	if bee, ok := cache[key]; ok {
		return bee
	}
	meta, ok, err := FindRun(colonyRoot, traceID, agentID)
	if err != nil || !ok {
		cache[key] = ""
		return ""
	}
	cache[key] = meta.Bee
	return meta.Bee
}
