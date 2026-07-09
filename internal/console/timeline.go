package console

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/tasks"
)

const (
	defaultEventFeedLimit = 50
	maxEventFeedLimit     = 200
	maxEventScanTraces    = 50
	maxEventScanEvents    = 500
)

// EventLink points to a related console resource.
type EventLink struct {
	Kind      string `json:"kind"`
	TraceID   string `json:"traceId,omitempty"`
	AgentID   string `json:"agentId,omitempty"`
	TaskID    string `json:"taskId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// EventFeedItem is a normalized timeline row for the console UI.
type EventFeedItem struct {
	ID          string             `json:"id"`
	CreatedAt   time.Time          `json:"createdAt"`
	TraceID     string             `json:"traceId"`
	AgentID     string             `json:"agentId"`
	Bee         string             `json:"bee,omitempty"`
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

// TraceSummaryView is a console projection of one trace.
type TraceSummaryView struct {
	TraceID         string    `json:"traceId"`
	LastActivityAt  time.Time `json:"lastActivityAt"`
	RunCount        int       `json:"runCount"`
	TaskCount       int       `json:"taskCount"`
	Bees            []string  `json:"bees,omitempty"`
	HasFailures     bool      `json:"hasFailures"`
	HasActive       bool      `json:"hasActive"`
	EnergyBudget    int       `json:"energyBudget,omitempty"`
	EnergyRemaining int       `json:"energyRemaining,omitempty"`
	LowEnergy       bool      `json:"lowEnergy,omitempty"`
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

// TraceDetailView aggregates one trace for the console.
type TraceDetailView struct {
	TraceSummaryView
	Tasks        []TaskSummaryView `json:"tasks"`
	Runs         []RunView         `json:"runs"`
	Worktree     *WorktreeView     `json:"worktree,omitempty"`
	RecentEvents []EventFeedItem   `json:"recentEvents"`
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

// ListTraces returns recent trace summaries.
func ListTraces(ctx colony.Context, limit int) ([]TraceSummaryView, error) {
	if limit <= 0 {
		limit = 20
	}
	summaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TraceSummaryView, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, traceSummaryViewFromRuns(s))
	}
	return out, nil
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
		if summary.RunCount == 0 && summary.TaskCount == 0 {
			return TraceDetailView{}, false, nil
		}
	}

	view := TraceDetailView{
		TraceSummaryView: traceSummaryViewFromRuns(summary),
	}
	enrichTraceEnergy(ctx, &view.TraceSummaryView)

	taskSnap, err := runs.LoadTraceTasksFromFS(ctx.ColonyRoot, traceID)
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

	runMetas, err := runs.ScanRecentRuns(ctx.ColonyRoot, recentRunLimit*4)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	for _, meta := range runMetas {
		if meta.TraceID != traceID {
			continue
		}
		view.Runs = append(view.Runs, runViewFromMeta(meta))
	}
	sortRuns(view.Runs)

	st, err := colony.LoadState(ctx.Slug)
	if err != nil {
		return TraceDetailView{}, false, err
	}
	for _, wt := range st.Worktrees {
		if wt.TraceID == traceID {
			view.Worktree = &WorktreeView{
				TraceID:   wt.TraceID,
				Path:      wt.Path,
				BaseSHA:   wt.BaseSHA,
				Branch:    wt.Branch,
				CreatedAt: wt.CreatedAt,
			}
			break
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
		beeCache := map[string]string{}
		for _, ev := range events {
			scanned = append(scanned, runs.ScannedEvent{
				Event: ev,
				Bee:   runsBeeForEvent(ctx.ColonyRoot, filter.TraceID, ev.AgentID, beeCache),
			})
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
	var out []InsightHighlight
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

func traceSummaryViewFromRuns(s runs.TraceSummary) TraceSummaryView {
	return TraceSummaryView{
		TraceID:        s.TraceID,
		LastActivityAt: s.LastActivityAt,
		RunCount:       s.RunCount,
		TaskCount:      s.TaskCount,
		Bees:           append([]string(nil), s.Bees...),
		HasFailures:    s.HasFailures,
		HasActive:      s.HasActive,
	}
}

func enrichTraceEnergy(ctx colony.Context, view *TraceSummaryView) {
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
	view.LowEnergy = snap.EnergyRemaining <= snap.EnergyBudget/4
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

func eventFeedID(ev protocol.Event) string {
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
	if filter.Bee != "" && !strings.EqualFold(item.Bee, filter.Bee) {
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
	case string(protocol.MutationCodeProposal):
		var p protocol.MutationPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return "", false
		}
		if p.Summary != "" {
			return "Code proposal: " + p.Summary, true
		}
		return "Code proposal submitted", true
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
		return a.AgentID > b.AgentID
	})
}

func runsBeeForEvent(colonyRoot, traceID, agentID string, cache map[string]string) string {
	key := traceID + "/" + agentID
	if bee, ok := cache[key]; ok {
		return bee
	}
	meta, ok, err := runs.FindRun(colonyRoot, traceID, agentID)
	if err != nil || !ok {
		cache[key] = ""
		return ""
	}
	cache[key] = meta.Bee
	return meta.Bee
}
