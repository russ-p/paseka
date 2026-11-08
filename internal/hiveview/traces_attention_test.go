package hiveview

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// writeAttentionViewRun writes one run with a full outcome pair so a Console
// fixture can describe which task each attempt belongs to.
func writeAttentionViewRun(
	t *testing.T,
	repo, traceID, agentID, taskID string,
	state protocol.RunStatus,
	started, finished time.Time,
) {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
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

// writeAttentionAnalysisRun writes one analysis attempt: the admission-time
// purpose is what excludes it from attention, never its Bee name or task id.
func writeAttentionAnalysisRun(
	t *testing.T,
	repo, traceID, agentID, taskID string,
	state protocol.RunStatus,
	started, finished time.Time,
) {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		TaskID:          taskID,
		Purpose:         protocol.PurposeAnalysis,
		AnalysisOf:      "completion-r1",
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

// writeAttentionConflictRun writes one run whose outcome pair conflicts, so the
// trail carries a record problem.
func writeAttentionConflictRun(
	t *testing.T,
	repo, traceID, agentID, taskID string,
	started, finished time.Time,
) {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
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

func attentionViewContext(t *testing.T, repo string) colony.Context {
	t.Helper()
	slug := "trace-attention"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("colony_root: "+repo+"\nslug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return colony.Context{ColonyRoot: repo, Slug: slug}
}

// The Console trace summary and the detail are projections of the same walk, so
// a healed trail reads hasFailures true with no open failure on both, and an
// open task survives with its count and instant.
func TestTraceViewsCarryOpenFailureProjection(t *testing.T) {
	repo := t.TempDir()
	ctx := attentionViewContext(t, repo)
	traceID := "trace-attention"
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }

	writeAttentionViewRun(t, repo, traceID, "agent-1", "task-a", protocol.StatusFailed, at(0), at(1))
	writeAttentionViewRun(t, repo, traceID, "agent-2", "task-a", protocol.StatusCompleted, at(2), at(3))
	writeAttentionViewRun(t, repo, traceID, "agent-3", "task-b", protocol.StatusFailed, at(4), at(5))
	// A cancelled attempt is unfinished work and reads as open here too.
	writeAttentionViewRun(t, repo, traceID, "agent-4", "task-c", protocol.StatusCancelled, at(6), at(7))
	// Analysis is filtered before grouping: a successful analysis on the same
	// task does not clear the work failure it studied.
	writeAttentionAnalysisRun(t, repo, traceID, "agent-5", "task-b", protocol.StatusCompleted, at(8), at(9))
	// A conflicting pair is a record problem: a later success of the same task
	// leaves it open and does not add to the work-failure count.
	writeAttentionConflictRun(t, repo, traceID, "agent-6", "task-d", at(10), at(11))
	writeAttentionViewRun(t, repo, traceID, "agent-7", "task-d", protocol.StatusCompleted, at(12), at(13))

	page, err := ListTraces(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var row *TraceSummaryView
	for i := range page {
		if page[i].TraceID == traceID {
			row = &page[i]
			break
		}
	}
	if row == nil {
		t.Fatalf("trail missing from %+v", page)
	}
	if !row.HasFailures {
		t.Fatalf("row = %+v, want the historical failure flag kept", row)
	}
	if row.OpenFailures != 2 {
		t.Fatalf("openFailures = %d, want 2 (the failed and the cancelled work attempts)", row.OpenFailures)
	}
	if row.NewestOpenFailureAt == nil || !row.NewestOpenFailureAt.Equal(at(7)) {
		t.Fatalf("newestOpenFailureAt = %v, want %s", row.NewestOpenFailureAt, at(7))
	}
	if !row.RecordProblem {
		t.Fatalf("row = %+v, want the record problem carried through", row)
	}

	detail, ok, err := GetTrace(ctx, traceID)
	if err != nil || !ok {
		t.Fatalf("GetTrace() = ok=%v err=%v", ok, err)
	}
	if detail.HasFailures != row.HasFailures || detail.OpenFailures != row.OpenFailures ||
		detail.RecordProblem != row.RecordProblem {
		t.Fatalf("detail = %+v, want it to agree with the list row %+v", detail, row)
	}
	if detail.NewestOpenFailureAt == nil || !detail.NewestOpenFailureAt.Equal(*row.NewestOpenFailureAt) {
		t.Fatalf("detail newest = %v, want %v", detail.NewestOpenFailureAt, row.NewestOpenFailureAt)
	}
}
