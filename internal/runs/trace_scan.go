package runs

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

const defaultTraceScanLimit = 50

// UsageAggregate sums per-run LLM usage under one trace (only runs that report usage).
type UsageAggregate struct {
	InputTokens       int64 `json:"inputTokens"`
	OutputTokens      int64 `json:"outputTokens"`
	CacheReadTokens   int64 `json:"cacheReadTokens"`
	CacheWriteTokens  int64 `json:"cacheWriteTokens"`
	RunCountWithUsage int   `json:"runCountWithUsage"`
}

// TraceSummary is a filesystem projection of activity under one trace directory.
type TraceSummary struct {
	TraceID        string          `json:"traceId"`
	LastActivityAt time.Time       `json:"lastActivityAt"`
	RunCount       int             `json:"runCount"`
	TaskCount      int             `json:"taskCount"`
	Bees           []string        `json:"bees,omitempty"`
	HasFailures    bool            `json:"hasFailures"`
	HasActive      bool            `json:"hasActive"`
	Usage          *UsageAggregate `json:"usage,omitempty"`
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
	summaries, err := scanAllTraces(colonyRoot)
	if err != nil {
		return nil, err
	}
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}
	return summaries, nil
}

// ScanTracesAfter returns the `limit` summaries that sort strictly after `after`,
// in the same newest-first order — the page an operator gets by asking for what
// comes after the last row they hold. An empty `after.TraceID` means the newest
// page. Past the last trace the page is empty rather than an error, which is the
// honest answer to "what is after the end".
//
// The cursor beats an offset here because the list grows while it is being read:
// a rank is only stable until a new trail pushes everything below it, whereas a
// cursor names a trail and so survives anything arriving above it. It used to carry
// a ceiling of `limit * 10` traces, which was never a cost — the walk below is a
// full scan either way, because there is no index over `.paseka/runs` — and so was
// just a filter applied to rows the walk had already read and then thrown away.
// Reading them costs nothing more, and it is why the walk has no depth limit.
func ScanTracesAfter(colonyRoot string, after TraceSummary, limit int) ([]TraceSummary, error) {
	if limit <= 0 {
		limit = defaultTraceScanLimit
	}
	summaries, err := scanAllTraces(colonyRoot)
	if err != nil {
		return nil, err
	}
	page := summaries
	if after.TraceID != "" {
		page = summaries[:0]
		for _, s := range summaries {
			// Strictly after the cursor, so a page never repeats the row it was
			// built from — and the tie-break in TraceOrderBefore is what makes
			// that exact for trails sharing an activity instant.
			if TraceOrderBefore(after, s) {
				page = append(page, s)
			}
		}
	}
	if len(page) > limit {
		page = page[:limit]
	}
	return page, nil
}

// scanAllTraces materializes every trace summary in newest-first order. Both
// scan entry points share it, so a page and a plain read cannot disagree about
// which trail sorts where.
func scanAllTraces(colonyRoot string) ([]TraceSummary, error) {
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
		return TraceOrderBefore(summaries[i], summaries[j])
	})
	return summaries, nil
}

// TraceOrderBefore reports whether a sorts ahead of b: newest activity first,
// with the trace id breaking ties so the order is total. The tie-break is what
// makes a paging cursor exact — two trails sharing an activity instant must
// still order deterministically, or the boundary between two pages could name
// the same row twice or skip one.
func TraceOrderBefore(a, b TraceSummary) bool {
	if !a.LastActivityAt.Equal(b.LastActivityAt) {
		return a.LastActivityAt.After(b.LastActivityAt)
	}
	return a.TraceID < b.TraceID
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
	var usageAgg UsageAggregate
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
		if meta.Usage != nil {
			usageAgg.InputTokens += meta.Usage.InputTokens
			usageAgg.OutputTokens += meta.Usage.OutputTokens
			usageAgg.CacheReadTokens += meta.Usage.CacheReadTokens
			usageAgg.CacheWriteTokens += meta.Usage.CacheWriteTokens
			usageAgg.RunCountWithUsage++
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

	if usageAgg.RunCountWithUsage > 0 {
		summary.Usage = &usageAgg
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
