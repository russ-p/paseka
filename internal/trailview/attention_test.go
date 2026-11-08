package trailview_test

import (
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailview"
)

// writeTrailRun writes one run directory with a full outcome pair, so a trail
// fixture can describe exactly which task each attempt belongs to.
func writeTrailRun(
	t *testing.T,
	root, traceID, agentID, taskID string,
	state protocol.RunStatus,
	started, finished time.Time,
) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          taskID,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		StartedAt:       started,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          state,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
}

// writeConflictingTrailRun writes one run whose status.json says failed and
// whose result.json says completed, so the pair is a record problem.
func writeConflictingTrailRun(
	t *testing.T,
	root, traceID, agentID, taskID string,
	started, finished time.Time,
) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          taskID,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusFailed,
		StartedAt:       started,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          protocol.StatusCompleted,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
}

// The shared trail collection is the one projection every surface reads: the
// open-failure count and its instant survive enrichment, and the collection row
// and the single-trail detail agree because they are built from the same walk.
func TestTrailPassesOpenFailureProjectionThrough(t *testing.T) {
	root := writeColony(t, "slug: test\nmodel_aliases:\n  builder: vendor/builder\n")
	setupHome(t, "test")
	ctx := testContext(root)
	traceID := "trace-attention"
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }

	// task-a failed and then completed, so it is healed; task-b failed last, so
	// it is the one open failure. task-c has a conflicting pair followed by a
	// success: the record problem survives the success and is not counted as
	// another open failure.
	writeTrailRun(t, root, traceID, "agent-1", "task-a", protocol.StatusFailed, at(0), at(1))
	writeTrailRun(t, root, traceID, "agent-2", "task-a", protocol.StatusCompleted, at(2), at(3))
	writeTrailRun(t, root, traceID, "agent-3", "task-b", protocol.StatusFailed, at(4), at(5))
	writeConflictingTrailRun(t, root, traceID, "agent-4", "task-c", at(6), at(7))
	writeTrailRun(t, root, traceID, "agent-5", "task-c", protocol.StatusCompleted, at(8), at(9))

	collection, err := trailview.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	row, ok := collection.Find(traceID)
	if !ok {
		t.Fatal("trail missing from the collection")
	}
	if !row.HasFailures {
		t.Fatalf("row = %+v, want the historical failure flag kept", row)
	}
	if row.OpenFailures != 1 {
		t.Fatalf("openFailures = %d, want 1", row.OpenFailures)
	}
	if row.NewestOpenFailureAt == nil || !row.NewestOpenFailureAt.Equal(at(5)) {
		t.Fatalf("newestOpenFailureAt = %v, want %s", row.NewestOpenFailureAt, at(5))
	}
	if !row.RecordProblem {
		t.Fatalf("row = %+v, want the record problem carried through", row)
	}

	detail, ok, err := trailview.LoadTrail(ctx, traceID)
	if err != nil || !ok {
		t.Fatalf("LoadTrail() = ok=%v err=%v", ok, err)
	}
	if detail.HasFailures != row.HasFailures ||
		detail.OpenFailures != row.OpenFailures ||
		detail.RecordProblem != row.RecordProblem ||
		detail.NewestOpenFailureAt == nil || row.NewestOpenFailureAt == nil ||
		!detail.NewestOpenFailureAt.Equal(*row.NewestOpenFailureAt) {
		t.Fatalf("detail = %+v, want it to agree with the collection row %+v", detail, row)
	}
}
