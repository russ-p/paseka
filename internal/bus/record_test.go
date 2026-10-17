package bus

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// refusingPublisher models a broker that refuses the event after the seam has
// recorded it locally.
type refusingPublisher struct {
	root  string
	calls int
}

func (p *refusingPublisher) PublishEvent(ctx context.Context, ev protocol.Event) error {
	_ = LocalPublisher{ColonyRoot: p.root}.PublishEvent(ctx, ev)
	p.calls++
	return errors.New("publish boom")
}

func recordableEvent(t *testing.T, traceID, agentID string, typ protocol.EventType, kind string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, typ, map[string]any{"kind": kind})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestLocalPublisherRecordsDomainEventsWithoutBus(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-local"
	pub := LocalPublisher{ColonyRoot: root}
	ctx := context.Background()

	if err := pub.PublishEvent(ctx, recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")); err != nil {
		t.Fatal(err)
	}
	if err := pub.PublishEvent(ctx, recordableEvent(t, traceID, "builder", protocol.EventMutation, "code.proposal.isolated")); err != nil {
		t.Fatal(err)
	}

	events, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	for _, ev := range events {
		if ev.ID == "" {
			t.Fatalf("event recorded without identity: %+v", ev)
		}
	}
}

func TestLocalPublisherSkipsRunLifecycleRecords(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-lifecycle"
	pub := LocalPublisher{ColonyRoot: root}

	for _, typ := range []protocol.EventType{protocol.EventToolCall, protocol.EventAssistantText, protocol.EventProgress, protocol.EventLog} {
		if err := pub.PublishEvent(context.Background(), recordableEvent(t, traceID, "builder", typ, "noise")); err != nil {
			t.Fatal(err)
		}
	}

	events, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %+v, want none — the record holds decisions, not elapsed time", events)
	}
}

func TestLocalPublisherWithoutColonyRootRecordsNothing(t *testing.T) {
	pub := LocalPublisher{}
	if err := pub.PublishEvent(context.Background(), recordableEvent(t, "trace-x", "cli", protocol.EventSignal, "feature.requested")); err != nil {
		t.Fatal(err)
	}
}

func TestPublisherSeamRecordsIdentityForEveryStore(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-identity"
	runDir := prepareRun(t, root, traceID, "agent-1")

	ev := recordableEvent(t, traceID, "agent-1", protocol.EventSignal, "task.ready")
	local := LocalPublisher{ColonyRoot: root}
	if err := local.PublishEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	merged, err := runs.ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged = %d events, want 1 — one identity, one row", len(merged))
	}
	if merged[0].ID != ev.ID {
		t.Fatalf("identity = %q, want %q", merged[0].ID, ev.ID)
	}
}

func TestFlushPendingRecordsLocallyWhenPublishFails(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-flush-fail"
	runDir := prepareRun(t, root, traceID, "agent-1")

	ev := recordableEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note")
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}

	result, err := FlushPending(context.Background(), &refusingPublisher{root: root}, root, traceID, "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "publish_failed" {
		t.Fatalf("result = %+v, want a publish failure", result)
	}

	recorded, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].ID != ev.ID {
		t.Fatalf("recorded = %+v, want the event the broker refused", recorded)
	}
}

func TestFlushPendingTwiceLandsOnce(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-flush-twice"
	runDir := prepareRun(t, root, traceID, "agent-1")

	ev := recordableEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note")
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}
	// A re-flush of a queue that failed midway appends the record again; the
	// merge on read is what keeps the trail from being inflated.
	for i := 0; i < 2; i++ {
		if err := runDir.AppendPending(ev); err != nil {
			t.Fatal(err)
		}
		if _, err := FlushPending(context.Background(), LocalPublisher{ColonyRoot: root}, root, traceID, "agent-1", false); err != nil {
			t.Fatal(err)
		}
		if err := runDir.WritePending(nil); err != nil {
			t.Fatal(err)
		}
	}

	events, err := runs.ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestFlushPendingMintsIdentityForPreIdentityQueue(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-flush-legacy"
	runDir := prepareRun(t, root, traceID, "agent-1")

	ev := recordableEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note")
	ev.ID = ""
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}
	if _, err := FlushPending(context.Background(), LocalPublisher{ColonyRoot: root}, root, traceID, "agent-1", false); err != nil {
		t.Fatal(err)
	}

	recorded, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0].ID == "" {
		t.Fatalf("recorded = %+v, want one event carrying a minted identity", recorded)
	}
}

func TestFlushPendingKeepsMintedIdentityAcrossAFailedPublish(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-flush-retry"
	runDir := prepareRun(t, root, traceID, "agent-1")

	ev := recordableEvent(t, traceID, "agent-1", protocol.EventInsight, "seam.note")
	ev.ID = ""
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}

	// The broker refuses, so the queue is not drained — and the local record
	// already holds the event under the identity the flush minted.
	if _, err := FlushPending(context.Background(), &refusingPublisher{root: root}, root, traceID, "agent-1", false); err != nil {
		t.Fatal(err)
	}
	queued, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 {
		t.Fatalf("queued = %d events, want the undrained one", len(queued))
	}
	if queued[0].ID == "" {
		t.Fatal("the minted identity must go back into the queue, or the retry mints a second one")
	}
	firstID := queued[0].ID

	// The retry lands the same event. An append-only record written by an
	// at-least-once producer may hold the line twice, but never under two
	// identities — that is what the merge could not collapse.
	if _, err := FlushPending(context.Background(), LocalPublisher{ColonyRoot: root}, root, traceID, "agent-1", false); err != nil {
		t.Fatal(err)
	}
	recorded, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	for i, ev := range recorded {
		if ev.ID != firstID {
			t.Fatalf("line %d carries identity %q, want the one the failed attempt used (%q)", i, ev.ID, firstID)
		}
	}
	merged, err := runs.ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 {
		t.Fatalf("trail history holds %d rows, want 1 — a retry must not inflate the trail", len(merged))
	}
	if merged[0].ID != firstID {
		t.Fatalf("merged identity = %q, want %q", merged[0].ID, firstID)
	}
}

func TestClientRecordsBeforeTheBusAttempt(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-client-config"
	c := &Client{cfg: Config{SubjectPrefix: "paseka.test", Slug: "test", ColonyRoot: root}}

	// A client with no live connection fails the publish. The record is written
	// first, which is the whole point of putting the local write before the bus.
	err := c.PublishEvent(context.Background(), recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested"))
	if err == nil {
		t.Fatal("publish without a connection must report the failure")
	}
	events, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %+v, want the event recorded before the bus attempt", events)
	}
}

func TestClientWithoutColonyRootKeepsBusOnlyBehavior(t *testing.T) {
	root := t.TempDir()
	c := &Client{cfg: Config{SubjectPrefix: "paseka.test", Slug: "test"}}
	_ = c.PublishEvent(context.Background(), recordableEvent(t, "trace-bare", "cli", protocol.EventSignal, "feature.requested"))
	if entries, err := os.ReadDir(filepath.Join(root, ".paseka")); err == nil && len(entries) > 0 {
		t.Fatalf("runs tree = %+v, want nothing written", entries)
	}
}

func TestRecordTrailEventReportsUnwritableTrail(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-blocked"
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file where the trail directory belongs: the record cannot be written.
	if err := os.WriteFile(filepath.Join(root, ".paseka", "runs", traceID), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	pub := LocalPublisher{ColonyRoot: root}
	if err := pub.PublishEvent(context.Background(), recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")); err == nil {
		t.Fatal("recording is the whole job of a local publisher, so an unwritable record must report")
	}
}
