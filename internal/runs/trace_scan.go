package runs

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/logging"
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
	EventCount     int             `json:"eventCount"`
	Bees           []string        `json:"bees,omitempty"`
	HasFailures    bool            `json:"hasFailures"`
	HasActive      bool            `json:"hasActive"`
	Usage          *UsageAggregate `json:"usage,omitempty"`
}

// ScannedEvent pairs a protocol event with its producer attribution: the bee
// role when a run exists, otherwise a label for the surface that published it.
type ScannedEvent struct {
	Event    protocol.Event `json:"event"`
	Bee      string         `json:"bee,omitempty"`
	Producer string         `json:"producer,omitempty"`
}

// ScanRecentTraces walks .paseka/runs and returns trace summaries ordered by
// last activity, newest first.
func ScanRecentTraces(colonyRoot string, limit int) ([]TraceSummary, error) {
	if limit <= 0 {
		limit = defaultTraceScanLimit
	}
	walk, err := scanTraces(colonyRoot, walkOptions{keep: limit})
	if err != nil {
		return nil, err
	}
	return walk.summaries(), nil
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
	// A cursor page needs every trail the walk saw, because anything below the
	// cursor is a row it may still have to return.
	walk, err := scanTraces(colonyRoot, walkOptions{})
	if err != nil {
		return nil, err
	}
	summaries := walk.summaries()
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

// scannedTrail is one trail from a walk: its summary, and — when the caller
// asked to retain records — the *distinct* events of the trail record the walk
// already read to derive that summary. Raw lines are never handed to a
// projection: the record is at-least-once, so a raw read holds a re-flushed
// event twice and a feed built from it would repeat the row.
type scannedTrail struct {
	summary TraceSummary
	events  []protocol.Event
}

// walkOptions shapes one pass over .paseka/runs.
//
// keep <= 0 means every trail, which a cursor page needs because anything below
// the cursor is a row it may still have to return. A bounded keep keeps only the
// newest, so the walk holds no more than the caller asked for.
//
// retainRecords keeps the record each walk read. It is off for the summary-only
// callers: a trail record carries payloads, and a list read has no use for them.
type walkOptions struct {
	keep          int
	retainRecords bool
}

type traceWalk struct {
	opts  walkOptions
	order []scannedTrail
}

// scanTraces walks .paseka/runs once and returns its trails newest activity
// first. Every scan entry point shares it, so a page and a plain read cannot
// disagree about which trail sorts where.
func scanTraces(colonyRoot string, opts walkOptions) (traceWalk, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return traceWalk{opts: opts}, nil
		}
		return traceWalk{opts: opts}, err
	}

	walk := traceWalk{opts: opts}
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		// One read per trail. A summary needs the record for last activity, and
		// a caller that retained it reads the same bytes rather than a second time.
		events := distinctTrailEvents(colonyRoot, traceID)
		summary, err := loadTraceSummaryFrom(colonyRoot, traceID, events)
		if err != nil {
			continue
		}
		if summary.RunCount == 0 && summary.TaskCount == 0 && summary.EventCount == 0 {
			continue
		}
		// The summary has what it needed; only a caller that asked for the events
		// keeps them alive past this line.
		if !opts.retainRecords {
			events = nil
		}
		walk.add(scannedTrail{summary: summary, events: events})
	}
	return walk, nil
}

// add keeps the newest trail ahead of the rest, dropping the stalest once the
// walk holds more than the caller asked for. TraceOrderBefore is a total order,
// so this lands on the same trails a final sort over everything would have kept.
func (w *traceWalk) add(t scannedTrail) {
	w.order = append(w.order, t)
	sort.Slice(w.order, func(i, j int) bool {
		return TraceOrderBefore(w.order[i].summary, w.order[j].summary)
	})
	if w.opts.keep > 0 && len(w.order) > w.opts.keep {
		w.order = w.order[:w.opts.keep]
	}
}

func (w traceWalk) summaries() []TraceSummary {
	out := make([]TraceSummary, 0, len(w.order))
	for _, trail := range w.order {
		out = append(out, trail.summary)
	}
	return out
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
//
// It reads each trail's trail-level record, not its per-run logs: a colony-wide
// feed costs one file per trail rather than every run log in every scanned
// trail, and it survives a trail whose run data was partially cleaned. Trails
// written before the trail log existed hold no colony-wide rows; they remain
// complete in the trail-scoped reader and in an export.
func ScanRecentEvents(colonyRoot string, traceLimit, eventLimit int) ([]ScannedEvent, error) {
	if traceLimit <= 0 {
		traceLimit = defaultTraceScanLimit
	}
	if eventLimit <= 0 {
		eventLimit = 100
	}

	// One walk, one read per trail: the record that ranked each trail is the same
	// record this feed needs, and retention is bounded to the trails the page can
	// actually show.
	walk, err := scanTraces(colonyRoot, walkOptions{keep: traceLimit, retainRecords: true})
	if err != nil {
		return nil, err
	}

	producerByAgent := map[string]EventProducer{}
	var out []ScannedEvent
	for _, trail := range walk.order {
		for _, ev := range trail.events {
			producer := ProducerForEvent(colonyRoot, trail.summary.TraceID, ev.AgentID, producerByAgent)
			out = append(out, ScannedEvent{Event: ev, Bee: producer.Bee, Producer: producer.Producer})
		}
	}

	// Newest first, and total: the identity is the last key, so two events that
	// share every other one still have a defined order and a page boundary can
	// never fall between them.
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
		if a.AgentID != b.AgentID {
			return a.AgentID > b.AgentID
		}
		return a.ID > b.ID
	})

	if len(out) > eventLimit {
		out = out[:eventLimit]
	}
	return out, nil
}

// LoadTraceSummary builds a trace summary from filesystem projections.
func LoadTraceSummary(colonyRoot, traceID string) (TraceSummary, error) {
	return loadTraceSummaryFrom(colonyRoot, traceID, distinctTrailEvents(colonyRoot, traceID))
}

// readTrailEventsOrWarn reads one trail's record raw. A damaged record costs its
// own trail its events, never the colony's read.
func readTrailEventsOrWarn(colonyRoot, traceID string) []protocol.Event {
	events, err := ReadTrailEvents(colonyRoot, traceID)
	if err != nil {
		traceLog.Warn("skip unreadable trail event log",
			logging.F("traceId", traceID),
			logging.F("error", err.Error()))
		return nil
	}
	return events
}

// distinctTrailEvents reads a trail's record the way every projection wants it:
// one entry per event. It is the single read path a walk uses, so a summary and
// the feed it feeds can never disagree about how many events a trail holds.
func distinctTrailEvents(colonyRoot, traceID string) []protocol.Event {
	return DedupeTrailEvents(readTrailEventsOrWarn(colonyRoot, traceID))
}

// loadTraceSummaryFrom builds a summary from the trail's distinct events. It
// takes them rather than reading, so a walk that already has them pays for one
// read instead of two, and a nil means "this trail has no record", not "go
// look".
func loadTraceSummaryFrom(colonyRoot, traceID string, trailEvents []protocol.Event) (TraceSummary, error) {
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

	// A trail's history is not only what its runs did. An intake request, a
	// honey charge, a kill, or a runtime-synthesized title is the trail's
	// newest fact long before any run flies, so the trail log counts toward
	// last activity and keeps an events-only trail in the listing at all.
	summary.EventCount = len(trailEvents)
	for _, ev := range trailEvents {
		if ev.CreatedAt.After(summary.LastActivityAt) {
			summary.LastActivityAt = ev.CreatedAt
		}
	}
	return summary, nil
}

// Producer labels for events whose agent id names something other than a bee
// run. A beekeeper's own action reads as a beekeeper, and the runtime's
// synthesized events read as the runtime — otherwise both show up as an unknown
// bee, which is the one thing an operator cannot act on.
//
// Both labels are words the record already uses — a runtime event publishes
// under `runtime`, and a human action under one of the ids below — so filtering
// the feed by one is filtering on what the colony said, not on a guess made
// here.
const (
	ProducerBeekeeper = "beekeeper"
	ProducerRuntime   = "runtime"
)

// beekeeperAgentIDs are the agent ids a human action publishes under anywhere in
// the tree: the CLI (`paseka signal`, `paseka event emit`), the Telegram Human
// Gateway, Console writes, the review package's human records, and the invites
// flow's operator-side record.
//
// The set is what the tree actually publishes, not a guess at what a human
// might call themselves: an agent id a colony invents (`paseka signal --agent …`)
// is not attributed rather than being folded into the beekeeper bucket.
var beekeeperAgentIDs = map[string]struct{}{
	"cli":       {},
	"console":   {},
	"telegram":  {},
	"human":     {},
	"beekeeper": {},
}

// runtimeAgentIDs are the agent ids the runtime publishes synthesized events
// under: task status and completion transitions, energy charges, the final
// review gate's plan and verdict, and the invites completion record.
//
// Matching is exact rather than a prefix heuristic. A trail's older records
// carry a synthetic `agent-<traceId>` agent id that no code in the tree mints —
// see the `trace.summary` backlog item, where two of them appear on the bus —
// and attributing an id nothing writes to a producer nothing confirms would be a
// guess. An unrecognized id stays unattributed and reads as unknown.
var runtimeAgentIDs = map[string]struct{}{
	"runtime": {},
}

// EventProducer is who produced one event: a bee by role where a run exists,
// otherwise a label for the surface that published it.
type EventProducer struct {
	Bee      string
	Producer string
}

// ProducerForEvent resolves who produced an event, caching per agent id so a
// colony-wide feed pays the run lookup once per producer rather than once per
// event.
func ProducerForEvent(colonyRoot, traceID, agentID string, cache map[string]EventProducer) EventProducer {
	if cache == nil {
		return resolveEventProducer(colonyRoot, traceID, agentID)
	}
	key := traceID + "/" + agentID
	if got, ok := cache[key]; ok {
		return got
	}
	got := resolveEventProducer(colonyRoot, traceID, agentID)
	cache[key] = got
	return got
}

func resolveEventProducer(colonyRoot, traceID, agentID string) EventProducer {
	// A run wins over any label: an agent id a bee happened to be invoked under
	// still belongs to that bee.
	if meta, ok, err := FindRun(colonyRoot, traceID, agentID); err == nil && ok && meta.Bee != "" {
		return EventProducer{Bee: meta.Bee}
	}
	if _, ok := beekeeperAgentIDs[agentID]; ok {
		return EventProducer{Producer: ProducerBeekeeper}
	}
	if _, ok := runtimeAgentIDs[agentID]; ok {
		return EventProducer{Producer: ProducerRuntime}
	}
	return EventProducer{}
}
