package hiveview

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
	"github.com/russ-p/paseka/internal/worktree"
)

const (
	defaultEventFeedLimit = 50
	maxEventFeedLimit     = 200
	maxEventScanTraces    = 50
	maxEventScanEvents    = 500
)

// EventLink points to a related hive resource.
type EventLink struct {
	Kind      string `json:"kind"`
	TraceID   string `json:"traceId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// EventFeedItem is a normalized timeline row for trace event feeds.
type EventFeedItem struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	TraceID   string    `json:"traceId"`
	AgentID   string    `json:"agentId"`
	Bee       string    `json:"bee,omitempty"`
	// Producer names a non-bee producer — the beekeeper, or the runtime on a
	// bee's behalf — so a human action is not read as an unknown bee.
	Producer    string             `json:"producer,omitempty"`
	Type        protocol.EventType `json:"type"`
	PayloadKind string             `json:"payloadKind,omitempty"`
	TaskID      string             `json:"taskId,omitempty"`
	Severity    string             `json:"severity,omitempty"`
	Summary     string             `json:"summary"`
	Link        *EventLink         `json:"link,omitempty"`
	Raw         protocol.Event     `json:"raw"`
}

// EventFeedPage is a cursor-paginated event feed.
type EventFeedPage struct {
	Items      []EventFeedItem `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
	HasMore    bool            `json:"hasMore"`
}

// EventFilter selects events for colony-wide or trace-scoped feeds.
type EventFilter struct {
	TraceID     string
	TaskID      string
	AgentID     string
	Bee         string
	EventType   string
	PayloadKind string
	Severity    string
	Limit       int
	AfterCursor string
}

// TraceSummaryView is a projection of one trace.
type TraceSummaryView struct {
	TraceID         string               `json:"traceId"`
	Title           string               `json:"title,omitempty"`
	Summary         string               `json:"summary,omitempty"`
	LastActivityAt  time.Time            `json:"lastActivityAt"`
	RunCount        int                  `json:"runCount"`
	TaskCount       int                  `json:"taskCount"`
	EventCount      int                  `json:"eventCount"`
	Bees            []string             `json:"bees,omitempty"`
	HasFailures     bool                 `json:"hasFailures"`
	HasActive       bool                 `json:"hasActive"`
	EnergyBudget    int                  `json:"energyBudget,omitempty"`
	EnergyRemaining int                  `json:"energyRemaining,omitempty"`
	EnergyAdded     int                  `json:"energyAdded,omitempty"`
	EnergyAllocated int                  `json:"energyAllocated,omitempty"`
	LowEnergy       bool                 `json:"lowEnergy,omitempty"`
	Standing        bool                 `json:"standing,omitempty"`
	Usage           *runs.UsageAggregate `json:"usage,omitempty"`

	// Trail collection evidence. ConfigRef is the active retained
	// configuration; ForkOf is the source trail for an experiment.
	ConfigBound          bool                `json:"configBound"`
	ConfigRef            string              `json:"configRef,omitempty"`
	InitialConfigRef     string              `json:"initialConfigRef,omitempty"`
	ActiveConfigRef      string              `json:"activeConfigRef,omitempty"`
	ConfigRevision       int                 `json:"configRevision,omitempty"`
	CandidateConfigRef   string              `json:"candidateConfigRef,omitempty"`
	ConfigPolicy         string              `json:"configPolicy,omitempty"`
	ForkID               string              `json:"forkId,omitempty"`
	ForkOf               string              `json:"forkOf,omitempty"`
	SourceTask           string              `json:"sourceTask,omitempty"`
	SourceAttempt        string              `json:"sourceAttempt,omitempty"`
	SourceConfigRef      string              `json:"sourceConfigRef,omitempty"`
	SourceConfigRevision int                 `json:"sourceConfigRevision,omitempty"`
	SourceBaseSHA        string              `json:"sourceBaseSha,omitempty"`
	BaseSHA              string              `json:"baseSha,omitempty"`
	Delta                *trailfork.Delta    `json:"delta,omitempty"`
	Budget               int                 `json:"budget,omitempty"`
	Branch               string              `json:"branch,omitempty"`
	AdmissionState       string              `json:"admissionState,omitempty"`
	PendingConfig        *trailconfig.Change `json:"pendingConfig,omitempty"`
	PendingForkRequests  int                 `json:"pendingForkRequests,omitempty"`
	Evidence             []string            `json:"evidence,omitempty"`
}

// TaskSummaryView is a lightweight task row for trace detail.
type TaskSummaryView struct {
	TaskID string `json:"taskId"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Bee    string `json:"bee,omitempty"`
}

// WorktreeView exposes one active worktree entry.
type WorktreeView struct {
	TraceID   string    `json:"traceId"`
	Path      string    `json:"path"`
	BaseSHA   string    `json:"baseSha"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// TraceDetailView aggregates one trace for inspection UIs and export.
type TraceDetailView struct {
	TraceSummaryView
	Tasks        []TaskSummaryView     `json:"tasks"`
	Runs         []RunView             `json:"runs"`
	Worktree     *WorktreeView         `json:"worktree,omitempty"`
	PullRequest  *PullRequestView      `json:"pullRequest,omitempty"`
	RecentEvents []EventFeedItem       `json:"recentEvents"`
	Config       *trailconfig.Manifest `json:"config,omitempty"`
	Lineage      *trailfork.Lineage    `json:"lineage,omitempty"`
}

// PullRequestView is machine-local forge identity on a trace.
type PullRequestView struct {
	URL    string `json:"url,omitempty"`
	Number int    `json:"number,omitempty"`
	Head   string `json:"head,omitempty"`
	State  string `json:"state,omitempty"`
	Draft  bool   `json:"draft,omitempty"`
}

// InsightHighlight is a recent narrative insight for the dashboard.
type InsightHighlight struct {
	CreatedAt   time.Time `json:"createdAt"`
	TraceID     string    `json:"traceId"`
	AgentID     string    `json:"agentId"`
	Bee         string    `json:"bee,omitempty"`
	PayloadKind string    `json:"payloadKind"`
	Summary     string    `json:"summary"`
	Severity    string    `json:"severity,omitempty"`
}

// ParseEventFilter reads query parameters into an EventFilter.
func ParseEventFilter(values url.Values) EventFilter {
	limit := defaultEventFeedLimit
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxEventFeedLimit {
		limit = maxEventFeedLimit
	}
	return EventFilter{
		TraceID:     strings.TrimSpace(values.Get("traceId")),
		TaskID:      strings.TrimSpace(values.Get("taskId")),
		AgentID:     strings.TrimSpace(values.Get("agentId")),
		Bee:         strings.TrimSpace(values.Get("bee")),
		EventType:   strings.TrimSpace(values.Get("type")),
		PayloadKind: strings.TrimSpace(values.Get("kind")),
		Severity:    strings.TrimSpace(values.Get("severity")),
		Limit:       limit,
		AfterCursor: strings.TrimSpace(values.Get("after")),
	}
}

// DefaultTracePageLimit is the trace page size when the caller asks for none.
const DefaultTracePageLimit = 20

// MaxTracePageLimit caps how much one trace page may cost the scan.
const MaxTracePageLimit = 200

// TracePageQuery is one page of the trace history: the `Limit` trails that sort
// strictly after the `Before` cursor, or the newest `Limit` when it is empty.
type TracePageQuery struct {
	Limit  int
	Before string
}

// ParseTracePageQuery reads limit and before from a query string. An absent
// limit falls back to the default and an oversized one clamps; a limit that is
// not a positive integer, or a malformed cursor, is an error the caller should
// answer 400 with.
func ParseTracePageQuery(values url.Values) (TracePageQuery, error) {
	query := TracePageQuery{Limit: DefaultTracePageLimit}
	if raw := strings.TrimSpace(values.Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 {
			return TracePageQuery{}, fmt.Errorf("limit must be a positive integer")
		}
		query.Limit = min(limit, MaxTracePageLimit)
	}
	if raw := strings.TrimSpace(values.Get("before")); raw != "" {
		if _, _, err := ParseTraceCursor(raw); err != nil {
			return TracePageQuery{}, err
		}
		query.Before = raw
	}
	return query, nil
}

// ParseTraceCursor splits a `<rfc3339nano>|<traceId>` cursor. The timestamp is
// the last row's activity and the id breaks ties, so paging over trails that
// share an activity instant stays exact.
func ParseTraceCursor(cursor string) (time.Time, string, error) {
	at, id, found := strings.Cut(strings.TrimSpace(cursor), "|")
	if !found || strings.TrimSpace(id) == "" {
		return time.Time{}, "", fmt.Errorf("before must look like <rfc3339nano>|<traceId>")
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(at))
	if err != nil {
		return time.Time{}, "", fmt.Errorf("before must start with an RFC3339 timestamp")
	}
	return parsed, strings.TrimSpace(id), nil
}

// TraceCursorFor builds the cursor that resumes after the given summary.
func TraceCursorFor(summary runs.TraceSummary) string {
	return summary.LastActivityAt.UTC().Format(time.RFC3339Nano) + "|" + summary.TraceID
}

// ListTracesPage returns one page of trace summaries, newest first. A `Before`
// cursor is exclusive, so paging with the last row of a page never repeats it.
// The cursor anchors the page to a trail rather than to a rank, so trails arriving
// mid-read push nothing out from under it.
func ListTracesPage(ctx colony.Context, query TracePageQuery) ([]TraceSummaryView, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = DefaultTracePageLimit
	}
	limit = min(limit, MaxTracePageLimit)

	var after runs.TraceSummary
	if query.Before != "" {
		at, id, err := ParseTraceCursor(query.Before)
		if err != nil {
			return nil, err
		}
		after = runs.TraceSummary{TraceID: id, LastActivityAt: at}
	}

	summaries, err := runs.ScanTracesAfter(ctx.ColonyRoot, after, limit)
	if err != nil {
		return nil, err
	}
	return projectTraces(ctx, summaries)
}

// projectTraces enriches raw run summaries into the Console projection.
func projectTraces(ctx colony.Context, summaries []runs.TraceSummary) ([]TraceSummaryView, error) {
	standing := LoadStandingTraceIDs(ctx.ColonyRoot)
	trails, _, err := trailview.LoadSummaries(ctx, summaries)
	if err != nil {
		return nil, err
	}
	out := make([]TraceSummaryView, 0, len(summaries))
	for i, s := range summaries {
		view := TraceSummaryFromRuns(s)
		EnrichTraceTitle(ctx, &view)
		EnrichTraceSummary(ctx, &view)
		EnrichTraceStanding(standing, &view)
		if view.Standing {
			EnrichTraceEnergy(ctx, &view)
		}
		if i < len(trails) {
			applyTrailView(&view, trails[i])
		}
		out = append(out, view)
	}
	return out, nil
}

// applyTrailView copies the shared trail collection fields onto a Console row.
// The raw run summary stays authoritative for counts and activity; the read
// model contributes configuration and lineage evidence.
func applyTrailView(view *TraceSummaryView, trail trailview.Trail) {
	view.ConfigBound = trail.ConfigBound
	view.ConfigRef = trail.ConfigRef
	view.InitialConfigRef = trail.InitialConfigRef
	view.ActiveConfigRef = trail.ActiveConfigRef
	view.ConfigRevision = trail.ConfigRevision
	view.CandidateConfigRef = trail.CandidateConfigRef
	view.ConfigPolicy = trail.Policy
	view.ForkID = trail.ForkID
	view.ForkOf = trail.ForkOf
	view.SourceTask = trail.SourceTask
	view.SourceAttempt = trail.SourceAttempt
	view.SourceConfigRef = trail.SourceConfigRef
	view.SourceConfigRevision = trail.SourceConfigRevision
	view.SourceBaseSHA = trail.SourceBaseSHA
	view.BaseSHA = trail.BaseSHA
	view.Delta = trail.Delta
	view.Budget = trail.Budget
	view.Branch = trail.Branch
	view.AdmissionState = trail.AdmissionState
	view.PendingConfig = trail.PendingConfig
	view.PendingForkRequests = trail.PendingForkRequests
	view.Evidence = trail.Evidence
}

// ListTraces returns recent trace summaries.
func ListTraces(ctx colony.Context, limit int) ([]TraceSummaryView, error) {
	if limit <= 0 {
		limit = DefaultTracePageLimit
	}
	summaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, limit)
	if err != nil {
		return nil, err
	}
	return projectTraces(ctx, summaries)
}

// GetTrace returns one trace detail view.
func GetTrace(ctx colony.Context, traceID string) (TraceDetailView, bool, error) {
	if traceID == "" {
		return TraceDetailView{}, false, nil
	}
	summaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, maxEventScanTraces*4)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	var summary runs.TraceSummary
	found := false
	for _, s := range summaries {
		if s.TraceID == traceID {
			summary = s
			found = true
			break
		}
	}
	if !found {
		summary, err = runs.LoadTraceSummary(ctx.ColonyRoot, traceID)
		if err != nil {
			return TraceDetailView{}, false, err
		}
		// A trail that only ever received events still exists: its history is
		// on disk even though it never flew a bee.
		if summary.RunCount == 0 && summary.TaskCount == 0 && summary.EventCount == 0 {
			return TraceDetailView{}, false, nil
		}
	}

	// Tasks and Runs start as empty lists rather than being appended onto nil:
	// a trace that has neither — or only one of them — reaches the console as a
	// JSON array, and a nil slice there is `null`, which the page cannot iterate.
	view := TraceDetailView{
		TraceSummaryView: TraceSummaryFromRuns(summary),
		Tasks:            []TaskSummaryView{},
		Runs:             []RunView{},
	}
	EnrichTraceEnergy(ctx, &view.TraceSummaryView)
	EnrichTraceTitle(ctx, &view.TraceSummaryView)
	EnrichTraceSummary(ctx, &view.TraceSummaryView)
	EnrichTraceStanding(LoadStandingTraceIDs(ctx.ColonyRoot), &view.TraceSummaryView)

	trails, _, err := trailview.LoadSummaries(ctx, []runs.TraceSummary{summary})
	if err != nil {
		return TraceDetailView{}, false, err
	}
	if len(trails) > 0 {
		applyTrailView(&view.TraceSummaryView, trails[0])
		view.Config = trails[0].Config
		view.Lineage = trails[0].Lineage
	}

	taskSnap, err := loadTraceTasksOfflineFirst(ctx, traceID)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	ids := make([]string, 0, len(taskSnap.Tasks))
	for id := range taskSnap.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		task := taskSnap.Tasks[id]
		title := task.Title
		if title == "" {
			title = task.TaskID
		}
		view.Tasks = append(view.Tasks, TaskSummaryView{
			TaskID: task.TaskID,
			Title:  title,
			Status: string(task.Status),
			Bee:    task.Bee,
		})
	}
	// Align summary count with the tasks actually rendered (KV may be newer than FS scan).
	view.TaskCount = len(view.Tasks)

	runMetas, err := runs.ListRunsForTrace(ctx.ColonyRoot, traceID)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	for _, meta := range runMetas {
		view.Runs = append(view.Runs, runViewFromMeta(meta))
	}
	sortRuns(view.Runs)

	st, err := homestate.LoadState(ctx.Slug)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	for _, wt := range st.Worktrees {
		if wt.TraceID == traceID {
			branch := wt.Branch
			if resolved, err := worktree.ResolvedBranch(ctx.ColonyRoot, traceID, ctx.Slug); err == nil && resolved != "" {
				branch = resolved
			}
			view.Worktree = &WorktreeView{
				TraceID:   wt.TraceID,
				Path:      wt.Path,
				BaseSHA:   wt.BaseSHA,
				Branch:    branch,
				CreatedAt: wt.CreatedAt,
			}
			break
		}
	}
	if pr, ok, err := homestate.FindPullRequest(ctx.Slug, traceID); err == nil && ok {
		view.PullRequest = &PullRequestView{
			URL:    pr.URL,
			Number: pr.Number,
			Head:   pr.Head,
			State:  pr.State,
			Draft:  pr.Draft,
		}
	}

	events, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Limit: 20})
	if err != nil {
		return TraceDetailView{}, false, err
	}
	view.RecentEvents = events.Items
	return view, true, nil
}

// ListEventFeed returns a filtered, paginated event feed.
func ListEventFeed(ctx colony.Context, filter EventFilter) (EventFeedPage, error) {
	if filter.Limit <= 0 {
		filter.Limit = defaultEventFeedLimit
	}
	if filter.Limit > maxEventFeedLimit {
		filter.Limit = maxEventFeedLimit
	}

	var scanned []runs.ScannedEvent
	var err error
	if filter.TraceID != "" {
		events, err := runs.ReadTraceEvents(ctx.ColonyRoot, filter.TraceID)
		if err != nil {
			return EventFeedPage{}, err
		}
		producerCache := map[string]runs.EventProducer{}
		for _, ev := range events {
			scanned = append(scanned, scannedEventFor(ctx.ColonyRoot, filter.TraceID, ev, producerCache))
		}
		sortScannedEventsDesc(scanned)
	} else {
		scanned, err = runs.ScanRecentEvents(ctx.ColonyRoot, maxEventScanTraces, maxEventScanEvents)
		if err != nil {
			return EventFeedPage{}, err
		}
	}

	items := make([]EventFeedItem, 0, len(scanned))
	for _, row := range scanned {
		item := eventFeedItemFromScanned(row)
		if !matchesEventFilter(item, filter) {
			continue
		}
		items = append(items, item)
	}

	start := 0
	if filter.AfterCursor != "" {
		for i, item := range items {
			if item.ID == filter.AfterCursor {
				start = i + 1
				break
			}
		}
	}

	end := start + filter.Limit
	hasMore := false
	if end < len(items) {
		hasMore = true
	} else {
		end = len(items)
	}
	pageItems := items[start:end]

	var nextCursor string
	if hasMore && len(pageItems) > 0 {
		nextCursor = pageItems[len(pageItems)-1].ID
	}

	return EventFeedPage{
		Items:      pageItems,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// CollectRecentInsights returns narrative INSIGHT highlights for the dashboard.
func CollectRecentInsights(ctx colony.Context, limit int) ([]InsightHighlight, error) {
	if limit <= 0 {
		limit = 10
	}
	scanned, err := runs.ScanRecentEvents(ctx.ColonyRoot, maxEventScanTraces, maxEventScanEvents)
	if err != nil {
		return nil, err
	}
	// The highlights land on the dashboard as a `recentInsights` array, so a
	// colony that has published no INSIGHT yet reads as an empty list.
	out := []InsightHighlight{}
	for _, row := range scanned {
		ev := row.Event
		if ev.Type != protocol.EventInsight {
			continue
		}
		kind := protocol.PayloadKind(ev.Payload)
		if !protocol.IsPromptMemoryInsightKind(kind) {
			continue
		}
		summary, ok := RenderEventSummary(ev)
		if !ok || summary == "" {
			continue
		}
		out = append(out, InsightHighlight{
			CreatedAt:   ev.CreatedAt,
			TraceID:     ev.TraceID,
			AgentID:     ev.AgentID,
			Bee:         row.Bee,
			PayloadKind: kind,
			Summary:     summary,
			Severity:    eventSeverity(ev),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func TraceSummaryFromRuns(s runs.TraceSummary) TraceSummaryView {
	return TraceSummaryView{
		TraceID:        s.TraceID,
		LastActivityAt: s.LastActivityAt,
		RunCount:       s.RunCount,
		TaskCount:      s.TaskCount,
		EventCount:     s.EventCount,
		Bees:           append([]string(nil), s.Bees...),
		HasFailures:    s.HasFailures,
		HasActive:      s.HasActive,
		Usage:          s.Usage,
	}
}

// EnrichTraceEnergy fills honey reserve fields from the task ledger when available.
func EnrichTraceEnergy(ctx colony.Context, view *TraceSummaryView) {
	if view == nil || view.TraceID == "" {
		return
	}
	session, err := tasks.OpenLedger(ctx)
	if err != nil || session.Ledger == nil {
		return
	}
	defer session.Close()
	snap, err := session.Ledger.Snapshot(view.TraceID)
	if err != nil || snap.EnergyBudget == 0 {
		return
	}
	view.EnergyBudget = snap.EnergyBudget
	view.EnergyRemaining = snap.EnergyRemaining
	view.EnergyAdded = snap.EnergyAdded
	view.EnergyAllocated = taskledger.Allocated(snap.EnergyBudget, snap.EnergyAdded)
	view.LowEnergy = snap.EnergyRemaining <= snap.EnergyBudget/4
}

func EnrichTraceTitle(ctx colony.Context, view *TraceSummaryView) {
	if view == nil || view.TraceID == "" {
		return
	}
	title, err := runs.ResolveTraceTitle(ctx.ColonyRoot, view.TraceID)
	if err != nil || title == "" {
		return
	}
	view.Title = title
}

func EnrichTraceSummary(ctx colony.Context, view *TraceSummaryView) {
	if view == nil || view.TraceID == "" {
		return
	}
	summary, err := runs.ResolveTraceSummary(ctx.ColonyRoot, view.TraceID)
	if err != nil || summary == "" {
		return
	}
	view.Summary = summary
}

// loadTraceTasksOfflineFirst prefers the KV ledger when NATS is available.
// If OpenLedger or LoadTrace fails (NATS/KV unavailable mid-request), it falls
// back to filesystem projections so historical traces remain inspectable.
func loadTraceTasksOfflineFirst(ctx colony.Context, traceID string) (taskledger.TraceSnapshot, error) {
	session, err := tasks.OpenLedger(ctx)
	var ledger taskledger.Ledger
	if err == nil {
		defer session.Close()
		ledger = session.Ledger
	}
	return resolveTraceTasks(ctx, ledger, traceID)
}

// resolveTraceTasks tries the ledger first, then filesystem on any load error.
func resolveTraceTasks(ctx colony.Context, ledger taskledger.Ledger, traceID string) (taskledger.TraceSnapshot, error) {
	if ledger != nil {
		snap, _, err := tasks.LoadTrace(ctx, ledger, traceID)
		if err == nil {
			return snap, nil
		}
	}
	return runs.LoadTraceTasksFromFS(ctx.ColonyRoot, traceID)
}

// BuildEventFeedItems maps trace events to normalized feed rows, preserving input order
// (typically chronological ascending from runs.ReadTraceEvents).
func BuildEventFeedItems(colonyRoot, traceID string, events []protocol.Event) []EventFeedItem {
	producerCache := map[string]runs.EventProducer{}
	items := make([]EventFeedItem, 0, len(events))
	for _, ev := range events {
		items = append(items, eventFeedItemFromScanned(scannedEventFor(colonyRoot, traceID, ev, producerCache)))
	}
	return items
}

// scannedEventFor pairs an event with who produced it, cached per agent id.
func scannedEventFor(colonyRoot, traceID string, ev protocol.Event, cache map[string]runs.EventProducer) runs.ScannedEvent {
	producer := runs.ProducerForEvent(colonyRoot, traceID, ev.AgentID, cache)
	return runs.ScannedEvent{Event: ev, Bee: producer.Bee, Producer: producer.Producer}
}

func eventFeedItemFromScanned(row runs.ScannedEvent) EventFeedItem {
	ev := row.Event
	kind := protocol.PayloadKind(ev.Payload)
	summary, _ := RenderEventSummary(ev)
	if summary == "" {
		summary = defaultEventSummary(ev, kind)
	}
	item := EventFeedItem{
		ID:          eventFeedID(ev),
		CreatedAt:   ev.CreatedAt,
		TraceID:     ev.TraceID,
		AgentID:     ev.AgentID,
		Bee:         row.Bee,
		Producer:    row.Producer,
		Type:        ev.Type,
		PayloadKind: kind,
		TaskID:      eventTaskID(ev),
		Severity:    eventSeverity(ev),
		Summary:     summary,
		Link:        eventLinkFor(ev, row.Bee),
		Raw:         ev,
	}
	return item
}

// eventFeedID is the feed row's cursor key. A minted identity is the key: it is
// unique by construction, so paging through events that share an instant cannot
// repeat or skip a row. Records written before identities existed fall back to
// the previous composite key.
func eventFeedID(ev protocol.Event) string {
	if ev.ID != "" {
		return ev.ID
	}
	return fmt.Sprintf("%s|%s|%s|%d",
		ev.CreatedAt.UTC().Format(time.RFC3339Nano),
		ev.TraceID,
		ev.AgentID,
		ev.Seq,
	)
}

func eventLinkFor(ev protocol.Event, bee string) *EventLink {
	link := &EventLink{
		Kind:    "run",
		TraceID: ev.TraceID,
		AgentID: ev.AgentID,
	}
	if taskID := eventTaskID(ev); taskID != "" {
		link.Kind = "task"
		link.TaskID = taskID
	}
	_ = bee
	return link
}

func matchesEventFilter(item EventFeedItem, filter EventFilter) bool {
	if filter.TraceID != "" && item.TraceID != filter.TraceID {
		return false
	}
	if filter.TaskID != "" && item.TaskID != filter.TaskID {
		return false
	}
	if filter.AgentID != "" && item.AgentID != filter.AgentID {
		return false
	}
	if filter.Bee != "" && !strings.EqualFold(item.Bee, filter.Bee) && !strings.EqualFold(item.Producer, filter.Bee) {
		return false
	}
	if filter.EventType != "" && string(item.Type) != filter.EventType {
		return false
	}
	if filter.PayloadKind != "" && item.PayloadKind != filter.PayloadKind {
		return false
	}
	if filter.Severity != "" && !strings.EqualFold(item.Severity, filter.Severity) {
		return false
	}
	return true
}

func eventTaskID(ev protocol.Event) string {
	kind := protocol.PayloadKind(ev.Payload)
	switch kind {
	case string(protocol.TaskEventReady), string(protocol.TaskEventCompleted):
		var p struct {
			TaskID string `json:"taskId"`
		}
		_ = json.Unmarshal(ev.Payload, &p)
		return strings.TrimSpace(p.TaskID)
	case string(protocol.MutationCodeProposal):
		var p protocol.MutationPayload
		_ = json.Unmarshal(ev.Payload, &p)
		return strings.TrimSpace(p.TaskID)
	default:
		var meta struct {
			TaskID string `json:"taskId"`
		}
		_ = json.Unmarshal(ev.Payload, &meta)
		return strings.TrimSpace(meta.TaskID)
	}
}

func eventSeverity(ev protocol.Event) string {
	if ev.Type != protocol.EventInsight {
		return ""
	}
	var p protocol.NarrativeInsightPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.Severity)
}

// RenderEventSummary returns a human-readable summary for known event kinds.
func RenderEventSummary(ev protocol.Event) (string, bool) {
	if ev.Type == protocol.EventInsight {
		if line, ok := protocol.RenderInsightForPrompt(ev); ok {
			return line, true
		}
	}
	kind := protocol.PayloadKind(ev.Payload)
	switch kind {
	case string(protocol.InsightTraceTitle):
		var p protocol.TraceTitlePayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Title) == "" {
			return "", false
		}
		return fmt.Sprintf("Trail title: %s", strings.TrimSpace(p.Title)), true
	case string(protocol.InsightWorktreeBranch):
		var p protocol.WorktreeBranchPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Branch) == "" {
			return "", false
		}
		return fmt.Sprintf("Worktree branch: %s", strings.TrimSpace(p.Branch)), true
	case string(protocol.TaskEventReady):
		var p protocol.TaskReadyPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		title := p.Title
		if title == "" {
			title = p.TaskID
		}
		return fmt.Sprintf("Task ready: %s", title), true
	case string(protocol.TaskEventCompleted):
		var p protocol.TaskCompletedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		summary := p.Summary
		if summary == "" {
			summary = string(p.Status)
		}
		return fmt.Sprintf("Task %s: %s", p.TaskID, summary), true
	case string(protocol.MutationCodeProposal), string(protocol.MutationCodeProposalIsolated), string(protocol.MutationCodeProposalRoot):
		var p protocol.MutationPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		label := "Code proposal"
		switch protocol.NormalizeCodeProposalKind(p.Kind) {
		case protocol.MutationCodeProposalRoot:
			label = "Code proposal (root)"
		case protocol.MutationCodeProposalIsolated:
			label = "Code proposal (isolated)"
		}
		if p.Summary != "" {
			return label + ": " + p.Summary, true
		}
		return label + " submitted", true
	case string(protocol.VerificationSuccess):
		var p protocol.VerificationPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		if p.Summary != "" {
			return "Verification passed: " + p.Summary, true
		}
		return "Verification passed", true
	case string(protocol.VerificationFailed):
		var p protocol.VerificationPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		if p.Summary != "" {
			return "Verification failed: " + p.Summary, true
		}
		return "Verification failed", true
	case "spec.ready":
		var p struct {
			Ref   string `json:"ref"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		ref := strings.TrimSpace(p.Ref)
		if ref == "" {
			return "Spec ready", true
		}
		if title := strings.TrimSpace(p.Title); title != "" {
			return fmt.Sprintf("Spec ready: %s (%s)", title, ref), true
		}
		return "Spec ready: " + ref, true
	default:
		return "", false
	}
}

func defaultEventSummary(ev protocol.Event, kind string) string {
	if kind != "" {
		return fmt.Sprintf("%s / %s", ev.Type, kind)
	}
	return string(ev.Type)
}

// sortScannedEventsDesc orders the feed newest first. The identity is the last
// key, so the order is total rather than merely stable: nothing about the
// comparator's inputs decides the order on its own, and a page boundary can
// never fall between two rows it cannot tell apart.
func sortScannedEventsDesc(items []runs.ScannedEvent) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i].Event, items[j].Event
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
}
