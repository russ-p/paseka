package runs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

func trailEvent(t *testing.T, traceID, agentID string, seq int, typ protocol.EventType, kind string, at time.Time) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, seq, typ, map[string]any{"kind": kind})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	return ev
}

func TestTrailAppendAndReadWithoutBroker(t *testing.T) {
	root := t.TempDir()
	traceID := "trail-only"
	trail := Trail{ColonyRoot: root, TraceID: traceID}
	at := time.Now().UTC()

	if err := trail.AppendEvent(trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", at)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", traceID, TrailEventsFileName)); err != nil {
		t.Fatalf("trail log not created: %v", err)
	}

	events, err := ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ID == "" {
		t.Fatalf("events = %+v, want one event carrying an identity", events)
	}
	if events[0].Seq != 0 {
		t.Fatalf("seq = %d, want 0 (the trail log allocates no sequence)", events[0].Seq)
	}
}

func TestTrailAppendCreatesRootForTrailWithNoRuns(t *testing.T) {
	root := t.TempDir()
	traceID := "trail-no-runs"
	trail := Trail{ColonyRoot: root, TraceID: traceID}
	if err := trail.AppendEvent(trailEvent(t, traceID, "telegram", 0, protocol.EventSignal, "feature.requested", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(trail.Root())
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range entries {
		if ent.IsDir() {
			t.Fatalf("trail root holds run directory %q, want only the record", ent.Name())
		}
	}
}

func TestTrailLogIsNotMistakenForARunDirectory(t *testing.T) {
	root := t.TempDir()
	traceID := "trail-not-a-run"
	trail := Trail{ColonyRoot: root, TraceID: traceID}
	if err := trail.AppendEvent(trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}

	metas, err := ListRunsForTrace(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 0 {
		t.Fatalf("runs = %+v, want none", metas)
	}

	summary, err := LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunCount != 0 || summary.EventCount != 1 {
		t.Fatalf("summary = %+v, want runCount 0 and eventCount 1", summary)
	}
}

func TestReadTraceEventsMergesRunLogsAndTrailLog(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-merge"
	at := time.Now().UTC()

	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	shared := trailEvent(t, traceID, "agent-1", 0, protocol.EventSignal, "feature.requested", at)
	if err := runDir.AppendEvent(shared); err != nil {
		t.Fatal(err)
	}
	runOnly := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "run.summary", at.Add(time.Second))
	if err := runDir.AppendEvent(runOnly); err != nil {
		t.Fatal(err)
	}

	trail := Trail{ColonyRoot: root, TraceID: traceID}
	trailOnly := trailEvent(t, traceID, "agent-1", 0, protocol.EventMutation, "code.proposal.isolated", at.Add(2*time.Second))
	if err := trail.AppendEvent(shared); err != nil {
		t.Fatal(err)
	}
	if err := trail.AppendEvent(trailOnly); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3 (the shared event appears once)", len(events))
	}
	seen := map[string]int{}
	for _, ev := range events {
		seen[ev.ID]++
	}
	if seen[shared.ID] != 1 {
		t.Fatalf("shared event appears %d times, want once", seen[shared.ID])
	}
	for i := 1; i < len(events); i++ {
		if events[i].CreatedAt.Before(events[i-1].CreatedAt) {
			t.Fatalf("events out of order: %+v", events)
		}
	}
}

func TestReadTraceEventsDedupesRetriedEvent(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-retry"
	at := time.Now().UTC()
	trail := Trail{ColonyRoot: root, TraceID: traceID}
	ev := trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", at)
	for i := 0; i < 3; i++ {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 — an at-least-once writer must not inflate the trail", len(events))
	}
}

func TestReadTraceEventsKeepsPreIdentityRecords(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-legacy"
	at := time.Now().UTC()

	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	legacy := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "trace.summary", at)
	legacy.ID = ""
	if err := runDir.AppendEvent(legacy); err != nil {
		t.Fatal(err)
	}
	if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(
		trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", at.Add(time.Second)),
	); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 — a record without identity is kept", len(events))
	}
	if events[0].ID != "" {
		t.Fatalf("legacy record gained an identity on read: %+v", events[0])
	}
}

func TestReadTraceEventsOrderIsTotalWhenTimestampsCollide(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-tie"
	at := time.Now().UTC()
	trail := Trail{ColonyRoot: root, TraceID: traceID}

	var ids []string
	for i := 0; i < 5; i++ {
		ev := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "trace.summary", at)
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, ev.ID)
	}

	first, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(first); i++ {
		if first[i].ID <= first[i-1].ID {
			t.Fatalf("tie order is not total: %q before %q", first[i-1].ID, first[i].ID)
		}
	}
	second, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("read is not stable: %q vs %q at %d", first[i].ID, second[i].ID, i)
		}
	}
	if len(first) != len(ids) {
		t.Fatalf("events = %d, want %d", len(first), len(ids))
	}
}

func TestReadTraceEventsSkipsUnreadableLog(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-damaged"
	at := time.Now().UTC()

	broken := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-broken"}
	if err := broken.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken.EventsPath(), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	healthy := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-ok"}
	if err := healthy.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := healthy.AppendEvent(trailEvent(t, traceID, "agent-ok", 0, protocol.EventSignal, "task.ready", at)); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatalf("one damaged run log must not fail the trail read: %v", err)
	}
	if len(events) != 1 || events[0].AgentID != "agent-ok" {
		t.Fatalf("events = %+v, want the healthy run's event only", events)
	}
}

func TestReadTraceEventsSkipsUnreadableTrailLog(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-damaged-trail"
	at := time.Now().UTC()

	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendEvent(trailEvent(t, traceID, "agent-1", 0, protocol.EventSignal, "task.ready", at)); err != nil {
		t.Fatal(err)
	}
	trail := Trail{ColonyRoot: root, TraceID: traceID}
	if err := os.MkdirAll(trail.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trail.EventsPath(), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatalf("a damaged trail log must not fail the read: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v, want the run log's event", events)
	}
}

func TestScanRecentEventsReadsTrailLogOnly(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()

	runOnlyTrace := "trace-per-run-only"
	runDir := Dir{ColonyRoot: root, TraceID: runOnlyTrace, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendEvent(trailEvent(t, runOnlyTrace, "agent-1", 0, protocol.EventSignal, "task.ready", at)); err != nil {
		t.Fatal(err)
	}

	recordedTrace := "trace-recorded"
	trail := Trail{ColonyRoot: root, TraceID: recordedTrace}
	if err := trail.AppendEvent(trailEvent(t, recordedTrace, "cli", 0, protocol.EventSignal, "feature.requested", at)); err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanRecentEvents(root, 50, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range scanned {
		if row.Event.TraceID == runOnlyTrace {
			t.Fatalf("colony-wide read opened a per-run log: %+v", row.Event)
		}
	}
	found := 0
	for _, row := range scanned {
		if row.Event.TraceID == recordedTrace {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("recorded events in colony-wide read = %d, want 1", found)
	}
}

func TestTraceWithEventsAndNoRunsIsListed(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-intake-only"
	at := time.Now().UTC().Add(-time.Minute)
	if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(
		trailEvent(t, traceID, "telegram", 0, protocol.EventSignal, "feature.requested", at),
	); err != nil {
		t.Fatal(err)
	}

	summaries, err := ScanRecentTraces(root, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v, want the intake-only trail", summaries)
	}
	if !summaries[0].LastActivityAt.Equal(at) {
		t.Fatalf("lastActivityAt = %v, want %v", summaries[0].LastActivityAt, at)
	}
	if summaries[0].RunCount != 0 || summaries[0].EventCount != 1 {
		t.Fatalf("summary = %+v", summaries[0])
	}
}

func TestTrailEventActivityOutranksRunMetadata(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-activity"
	started := time.Now().UTC().Add(-time.Hour)
	charged := started.Add(30 * time.Minute)

	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Bee:             "builder",
		Adapter:         "cursor",
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := runDir.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(
		trailEvent(t, traceID, "console", 0, protocol.EventSignal, "energy.consume", charged),
	); err != nil {
		t.Fatal(err)
	}

	summary, err := LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.LastActivityAt.Equal(charged) {
		t.Fatalf("lastActivityAt = %v, want the honey charge at %v", summary.LastActivityAt, charged)
	}
	if summary.RunCount != 1 {
		t.Fatalf("runCount = %d, want 1 — run counts still count runs", summary.RunCount)
	}
}

func TestProducerForEventAttributesHumansAndRuntime(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-producers"

	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-1",
		Bee:             "builder",
		Adapter:         "cursor",
		ColonyRoot:      root,
	}); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		agentID  string
		bee      string
		producer string
	}{
		{agentID: "agent-1", bee: "builder"},
		{agentID: "cli", producer: ProducerBeekeeper},
		{agentID: "telegram", producer: ProducerBeekeeper},
		{agentID: "console", producer: ProducerBeekeeper},
		{agentID: "beekeeper", producer: ProducerBeekeeper},
		// The agent id the runtime publishes every synthesized event under.
		{agentID: "runtime", producer: ProducerRuntime},
		// An id nothing in the tree mints: attributing it would be a guess.
		{agentID: "agent-" + traceID},
		{agentID: "mystery"},
	}
	for _, tc := range cases {
		got := ProducerForEvent(root, traceID, tc.agentID, nil)
		if got.Bee != tc.bee || got.Producer != tc.producer {
			t.Fatalf("producer for %q = %+v, want bee %q producer %q", tc.agentID, got, tc.bee, tc.producer)
		}
	}
}

func TestScanTracesRetainsTheRecordItRankedWith(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	for i, traceID := range []string{"trace-reuse-a", "trace-reuse-b", "trace-reuse-c"} {
		for k := range 2 {
			if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(
				trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "seam.note",
					at.Add(time.Duration(i*10+k)*time.Second)),
			); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Retaining the record is what lets the colony-wide feed avoid a second read
	// per trail. The contract this pins: a walk asked to retain returns the very
	// events its summaries were derived from, not a re-read of them.
	walk, err := scanTraces(root, walkOptions{keep: 2, retainRecords: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.order) != 2 {
		t.Fatalf("walk kept %d trails, want 2", len(walk.order))
	}
	for _, trail := range walk.order {
		if len(trail.events) != 2 {
			t.Fatalf("trail %s retained %d events, want 2", trail.summary.TraceID, len(trail.events))
		}
		if trail.summary.EventCount != len(trail.events) {
			t.Fatalf("trail %s summary counts %d events, retained %d",
				trail.summary.TraceID, trail.summary.EventCount, len(trail.events))
		}
	}
	// The newest two by activity: the last trail's events are the latest.
	if walk.order[0].summary.TraceID != "trace-reuse-c" {
		t.Fatalf("first trail = %q, want the most recent one", walk.order[0].summary.TraceID)
	}

	// A summary-only walk must not hold records: a list read has no use for
	// payloads, and the walk would otherwise keep them alive for nothing.
	summaryOnly, err := scanTraces(root, walkOptions{keep: 2})
	if err != nil {
		t.Fatal(err)
	}
	for _, trail := range summaryOnly.order {
		if trail.events != nil {
			t.Fatalf("trail %s retained a record in a summary-only walk", trail.summary.TraceID)
		}
		if trail.summary.EventCount != 2 {
			t.Fatalf("trail %s summary counts %d events, want 2", trail.summary.TraceID, trail.summary.EventCount)
		}
	}
}

func TestScanRecentEventsSkipsUnreadableTrailLog(t *testing.T) {
	root := t.TempDir()
	at := time.Now().UTC()
	damaged := "trace-damaged-scan"
	healthy := "trace-healthy-scan"

	for _, traceID := range []string{damaged, healthy} {
		if err := (Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(
			trailEvent(t, traceID, "agent-1", 0, protocol.EventSignal, "feature.requested", at),
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(Trail{ColonyRoot: root, TraceID: damaged}.EventsPath(), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	scanned, err := ScanRecentEvents(root, 50, 100)
	if err != nil {
		t.Fatalf("a damaged trail record must not fail the colony-wide read: %v", err)
	}
	for _, row := range scanned {
		if row.Event.TraceID == damaged {
			t.Fatalf("damaged trail contributed an event: %+v", row.Event)
		}
	}
	found := 0
	for _, row := range scanned {
		if row.Event.TraceID == healthy {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("healthy trail events = %d, want 1", found)
	}
}

func TestColonyWideFeedDeduplicatesRecordLines(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-record-at-least-once"
	at := time.Now().UTC()
	trail := Trail{ColonyRoot: root, TraceID: traceID}

	// The seam records before it publishes, so a refused publish leaves the event
	// in the record and the retry appends it again. The record is what an
	// operator reads; a repeated line there is not a second event.
	retried := trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", at)
	traffic := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "seam.note", at.Add(time.Second))
	for _, ev := range []protocol.Event{retried, retried, traffic, retried} {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}

	raw, err := ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 {
		t.Fatalf("record holds %d lines, want the 4 the writer actually wrote", len(raw))
	}

	scanned, err := ScanRecentEvents(root, 50, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != 2 {
		t.Fatalf("colony-wide feed holds %d rows, want 2 distinct events", len(scanned))
	}
	seen := map[string]int{}
	for _, row := range scanned {
		seen[row.Event.ID]++
	}
	if seen[retried.ID] != 1 {
		t.Fatalf("re-flushed event appears %d times in the colony-wide feed", seen[retried.ID])
	}

	// The listing agrees: a trail's event count is its history, not its appends.
	summary, err := LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.EventCount != 2 {
		t.Fatalf("eventCount = %d, want 2 distinct events", summary.EventCount)
	}
	if !summary.LastActivityAt.Equal(at.Add(time.Second)) {
		t.Fatalf("lastActivityAt = %v, want %v", summary.LastActivityAt, at.Add(time.Second))
	}
}

func TestDedupeTrailEventsKeepsPreIdentityRecords(t *testing.T) {
	withID := trailEvent(t, "trace-dedupe", "cli", 0, protocol.EventSignal, "feature.requested", time.Now().UTC())
	legacy := trailEvent(t, "trace-dedupe", "builder", 0, protocol.EventSignal, "task.ready", time.Now().UTC())
	legacy.ID = ""

	got := DedupeTrailEvents([]protocol.Event{withID, legacy, withID, legacy})
	// One copy of the identified event. Both identity-less lines survive: without
	// an identity two records cannot be told apart from one event twice, so
	// collapsing them would be a guess that can delete a real event.
	if len(got) != 3 {
		t.Fatalf("deduped = %d events, want 3 — only an identified event may be collapsed", len(got))
	}
	if got[0].ID != withID.ID || got[1].ID != "" || got[2].ID != "" {
		t.Fatalf("deduped = %+v, want first-occurrence order", got)
	}
	identified := 0
	for _, ev := range got {
		if ev.ID == withID.ID {
			identified++
		}
	}
	if identified != 1 {
		t.Fatalf("identified event kept %d times, want once", identified)
	}
}

func TestTornLastLineCostsOneEventNotTheWholeRecord(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-torn"
	at := time.Now().UTC()
	trail := Trail{ColonyRoot: root, TraceID: traceID}

	first := trailEvent(t, traceID, "cli", 0, protocol.EventSignal, "feature.requested", at)
	second := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "seam.note", at.Add(time.Second))
	for _, ev := range []protocol.Event{first, second} {
		if err := trail.AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
	}
	// A process that died mid-append leaves half a line behind. Everything
	// before it is history the colony actually has.
	appendRaw(t, trail.EventsPath(), `{"protocolVersion":"agent-runtime.v1","id":"evt-tor`)

	events, err := ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatalf("a torn line must not fail the read: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("read %d events, want the 2 complete ones", len(events))
	}
	if events[0].ID != first.ID || events[1].ID != second.ID {
		t.Fatalf("events = %+v, want both complete records", events)
	}

	// The trail surface and the listing agree on what survived.
	merged, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := LoadTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != summary.EventCount {
		t.Fatalf("history holds %d events, listing counts %d", len(merged), summary.EventCount)
	}
}

func TestRunLogAlsoSurvivesATornLine(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-torn-run"
	at := time.Now().UTC()
	runDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	kept := trailEvent(t, traceID, "agent-1", 0, protocol.EventInsight, "seam.note", at)
	if err := runDir.AppendEvent(kept); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, runDir.EventsPath(), "{oops")

	events, err := runDir.ReadEvents()
	if err != nil {
		t.Fatalf("a torn line must not fail the run log: %v", err)
	}
	if len(events) != 1 || events[0].ID != kept.ID {
		t.Fatalf("events = %+v, want the one complete record", events)
	}
}

// appendRaw appends bytes the way a crash mid-write would: without the trailing
// newline that makes a record complete.
func appendRaw(t *testing.T, path, raw string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(raw); err != nil {
		t.Fatal(err)
	}
}
