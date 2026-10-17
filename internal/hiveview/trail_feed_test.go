package hiveview

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func feedEvent(t *testing.T, traceID, agentID string, typ protocol.EventType, kind string, at time.Time) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, typ, map[string]any{"kind": kind})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	return ev
}

func TestEventFeedCarriesTrailRecordAndIdentityCursor(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-feed"
	at := time.Now().UTC()
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	intake := feedEvent(t, traceID, "telegram", protocol.EventSignal, "feature.requested", at)
	proposal := feedEvent(t, traceID, "agent-1", protocol.EventMutation, "code.proposal.isolated", at.Add(time.Second))
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	for _, ev := range []protocol.Event{intake, proposal} {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	page, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want both recorded events", len(page.Items))
	}
	byID := map[string]EventFeedItem{}
	for _, item := range page.Items {
		byID[item.ID] = item
	}
	if _, ok := byID[proposal.ID]; !ok {
		t.Fatalf("feed cursor does not use the event identity: %+v", page.Items)
	}
	if _, ok := byID[intake.ID]; !ok {
		t.Fatalf("feed is missing the intake request: %+v", page.Items)
	}

	// The operator's own signal is attributed to a beekeeper rather than
	// appearing as an unknown bee.
	var intakeItem EventFeedItem
	for _, item := range page.Items {
		if item.AgentID == "telegram" {
			intakeItem = item
		}
	}
	if intakeItem.Producer != runs.ProducerBeekeeper {
		t.Fatalf("producer = %q, want %q", intakeItem.Producer, runs.ProducerBeekeeper)
	}
	if intakeItem.Bee != "" {
		t.Fatalf("bee = %q, want empty for a human producer", intakeItem.Bee)
	}
}

func TestEventFeedPagingNeverRepeatsOrSkipsARow(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-paging"
	at := time.Now().UTC()
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	// Five events sharing one instant: only identity can order them.
	var ids []string
	for i := 0; i < 5; i++ {
		ev := feedEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note", at)
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ev.ID)
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}
	seen := map[string]int{}
	cursor := ""
	for page := 0; page < 5; page++ {
		filter := EventFilter{TraceID: traceID, Limit: 2, AfterCursor: cursor}
		got, err := ListEventFeed(ctx, filter)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Items) == 0 {
			break
		}
		for _, item := range got.Items {
			seen[item.ID]++
		}
		if !got.HasMore {
			break
		}
		cursor = got.NextCursor
	}
	if len(seen) != len(ids) {
		t.Fatalf("paged over %d rows, want %d", len(seen), len(ids))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("row %s paged %d times", id, n)
		}
	}
}

func TestEventFeedFilterMatchesProducer(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-filter"
	at := time.Now().UTC()
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	if err := trail.AppendEvent(feedEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested", at)); err != nil {
		t.Fatal(err)
	}
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	page, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Bee: runs.ProducerBeekeeper, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want the beekeeper's event", len(page.Items))
	}
}

func TestEventFeedOrderIsTotalWhenEverythingElseCollides(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-total-order"
	at := time.Now().UTC()
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}

	// Four events that share a creation instant, a sequence of 0, a trail id,
	// and a producer — only the identity can separate them, and nothing about the
	// order they were appended in may leak into the order they are read back in.
	ids := map[string]struct{}{}
	for range 4 {
		ev := feedEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note", at)
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
		ids[ev.ID] = struct{}{}
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}
	var firstOrder []string
	for round := range 3 {
		page, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != len(ids) {
			t.Fatalf("round %d: items = %d, want %d", round, len(page.Items), len(ids))
		}
		order := make([]string, 0, len(page.Items))
		for i, item := range page.Items {
			order = append(order, item.ID)
			if _, ok := ids[item.ID]; !ok {
				t.Fatalf("round %d: unexpected row %q", round, item.ID)
			}
			// Newest first: the identity is the only key left, so it decides, and
			// it decides the same way every time.
			if i > 0 && item.ID >= order[i-1] {
				t.Fatalf("round %d: %q does not sort before %q", round, item.ID, order[i-1])
			}
		}
		if round == 0 {
			firstOrder = order
			continue
		}
		for i := range order {
			if order[i] != firstOrder[i] {
				t.Fatalf("round %d: row %d = %q, want %q — a total order must not depend on read order", round, i, order[i], firstOrder[i])
			}
		}
	}
}

func TestEventFeedAttributesRuntimeProducedEvents(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-runtime-producer"
	at := time.Now().UTC()
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	// The agent id the runtime publishes every synthesized event under: task
	// status and completion, energy charges, the final review gate.
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	charge := feedEvent(t, traceID, "runtime", protocol.EventSignal, "energy.consume", at)
	verdict := feedEvent(t, traceID, "runtime", protocol.EventVerification, "task.completed", at.Add(time.Second))
	for _, ev := range []protocol.Event{charge, verdict} {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	page, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(page.Items))
	}
	for _, item := range page.Items {
		if item.Producer != runs.ProducerRuntime {
			t.Fatalf("producer for %s = %q, want %q", item.AgentID, item.Producer, runs.ProducerRuntime)
		}
		if item.Bee != "" {
			t.Fatalf("bee = %q, want empty for a runtime producer", item.Bee)
		}
	}

	filtered, err := ListEventFeed(ctx, EventFilter{TraceID: traceID, Bee: runs.ProducerRuntime, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 2 {
		t.Fatalf("filtered by runtime = %d items, want 2", len(filtered.Items))
	}
}

func TestGetTraceFindsTrailWithEventsAndNoRuns(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-events-only"
	at := time.Now().UTC()
	if err := (runs.Trail{ColonyRoot: repo, TraceID: traceID}).AppendEvent(
		feedEvent(t, traceID, "telegram", protocol.EventSignal, "feature.requested", at),
	); err != nil {
		t.Fatal(err)
	}
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	view, ok, err := GetTrace(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("a trail that received an intake request must not 404")
	}
	if view.RunCount != 0 || view.EventCount != 1 {
		t.Fatalf("view = %+v", view.TraceSummaryView)
	}
	if len(view.RecentEvents) != 1 {
		t.Fatalf("recentEvents = %d, want 1", len(view.RecentEvents))
	}
	if view.Title != "" {
		t.Logf("title = %q", view.Title)
	}
}

func TestListTracesListsEventsOnlyTrailWithActivity(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-list-events"
	charged := time.Now().UTC().Add(-time.Minute)
	if err := (runs.Trail{ColonyRoot: repo, TraceID: traceID}).AppendEvent(
		feedEvent(t, traceID, "console", protocol.EventSignal, "energy.consume", charged),
	); err != nil {
		t.Fatal(err)
	}
	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}

	views, err := ListTraces(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].TraceID != traceID {
		t.Fatalf("views = %+v", views)
	}
	if !views[0].LastActivityAt.Equal(charged) {
		t.Fatalf("lastActivityAt = %v, want the honey charge at %v", views[0].LastActivityAt, charged)
	}
}

func TestColonyWideFeedDeduplicatesAndPagesWithoutRepeating(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-colony-wide-dupes"
	at := time.Now().UTC()
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}

	// The record holds one event three times, because the seam records before it
	// publishes and the retry appends again.
	retried := feedEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested", at)
	second := feedEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note", at.Add(time.Second))
	for _, ev := range []protocol.Event{retried, retried, second, retried} {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	ctx := colony.Context{ColonyRoot: repo, Slug: "test"}
	first, err := ListEventFeed(ctx, EventFilter{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || !first.HasMore {
		t.Fatalf("first page = %+v, want one row and more to come", first)
	}

	// Duplicate rows would share a cursor key, so the page after this one would
	// hand back the row it was built from — forever.
	seen := map[string]int{}
	cursor := ""
	for range 5 {
		got, err := ListEventFeed(ctx, EventFilter{Limit: 1, AfterCursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Items) == 0 {
			break
		}
		for _, item := range got.Items {
			seen[item.ID]++
		}
		if !got.HasMore {
			break
		}
		cursor = got.NextCursor
	}
	if len(seen) != 2 {
		t.Fatalf("paged over %d distinct rows, want 2", len(seen))
	}
	for id, n := range seen {
		if n != 1 {
			t.Fatalf("row %s paged %d times — a repeated cursor key never terminates", id, n)
		}
	}
}
