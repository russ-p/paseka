package runs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestWriteWorkCompletionCapturesImmutableEvidence(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-work-1"
	completedAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

	checkpoint := WorkCompletionCheckpoint{
		CompletionID: "completion-r1-abc",
		WorkRevision: 1,
		CompletedAt:  completedAt,
		TaskIDs:      []string{"task-1"},
	}
	result := WorkCompletionResult{
		Tasks: []WorkCompletionTask{{TaskID: "task-1", Phase: "work", Status: "completed"}},
	}
	metrics := TrailMetrics{ObservedAt: completedAt}

	written, err := WriteWorkCompletion(root, traceID, checkpoint, result, metrics)
	if err != nil {
		t.Fatal(err)
	}
	if written.ResultRef == "" || written.MetricsRef == "" {
		t.Fatalf("refs = %q / %q", written.ResultRef, written.MetricsRef)
	}

	for _, ref := range []string{written.ResultRef, written.MetricsRef} {
		path, err := ResolveTrailRef(root, ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("ref %q does not resolve: %v", ref, err)
		}
	}
	if filepath.Clean(written.MetricsRef) == filepath.Clean(".paseka/runs/"+traceID+"/"+TrailMetricsFileName) {
		t.Fatalf("metricsRef points at the replaceable convenience file: %q", written.MetricsRef)
	}
	if filepath.Base(written.MetricsRef) != TrailMetricsFileName || filepath.Base(filepath.Dir(written.MetricsRef)) != checkpoint.CompletionID {
		t.Fatalf("metricsRef = %q, want the completion-owned snapshot", written.MetricsRef)
	}

	// A retry keeps the first capture's refs and does not rewrite evidence.
	before, err := os.ReadFile(WorkCompletionMetricsPath(root, traceID, checkpoint.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := WriteWorkCompletion(root, traceID, checkpoint, WorkCompletionResult{}, TrailMetrics{
		ObservedAt: completedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rewritten.ResultRef != written.ResultRef || rewritten.MetricsRef != written.MetricsRef {
		t.Fatalf("retry refs = %q / %q, want %q / %q", rewritten.ResultRef, rewritten.MetricsRef, written.ResultRef, written.MetricsRef)
	}
	after, err := os.ReadFile(WorkCompletionMetricsPath(root, traceID, checkpoint.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("a retry rewrote the immutable metrics snapshot")
	}

	if _, err := WriteWorkCompletion(root, traceID, WorkCompletionCheckpoint{
		CompletionID: checkpoint.CompletionID,
		WorkRevision: 2,
	}, result, metrics); err == nil {
		t.Fatal("a different revision reused an existing completion identity")
	}
}

func TestTrailWorkRecordFeedsMetrics(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-work-2"
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)

	task := taskledger.TaskSnapshot{
		TaskID: "task-1", Bee: "builder", Intent: "feature",
		Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at,
	}
	if err := WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}
	completion := MetricsCompletion{
		CompletionID: "completion-r1-abc",
		WorkRevision: 1,
		CompletedAt:  at,
		ResultRef:    ".paseka/runs/trace-work-2/completions/completion-r1-abc/result.json",
		MetricsRef:   ".paseka/runs/trace-work-2/completions/completion-r1-abc/metrics.json",
	}
	if err := WriteTrailWorkRecord(root, traceID, TrailWorkRecord{
		Revision: 1, Completion: &completion,
	}); err != nil {
		t.Fatal(err)
	}

	metrics, err := BuildTrailMetrics(root, traceID, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Work.Revision == nil || *metrics.Work.Revision != 1 {
		t.Fatalf("revision = %v, want 1", metrics.Work.Revision)
	}
	if metrics.Work.Completion == nil || metrics.Work.Completion.CompletionID != completion.CompletionID {
		t.Fatalf("completion = %+v", metrics.Work.Completion)
	}
	if metrics.Coverage.Work != CoverageComplete {
		t.Fatalf("work coverage = %q", metrics.Coverage.Work)
	}

	// A stale writer cannot lower the retained revision.
	if err := WriteTrailWorkRecord(root, traceID, TrailWorkRecord{Revision: 0}); err != nil {
		t.Fatal(err)
	}
	record, ok, err := ReadTrailWorkRecord(root, traceID)
	if err != nil || !ok {
		t.Fatalf("read work record: ok=%v err=%v", ok, err)
	}
	if record.Revision != 1 {
		t.Fatalf("revision = %d, want the recorded 1", record.Revision)
	}
}

func TestWorkRecordDoesNotClaimACompletionForNewerWork(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-work-3"
	at := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	task := taskledger.TaskSnapshot{
		TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at,
	}
	if err := WriteTaskSnapshot(root, traceID, task); err != nil {
		t.Fatal(err)
	}
	completion := MetricsCompletion{CompletionID: "completion-r1", WorkRevision: 1, CompletedAt: at}
	if err := WriteTrailWorkRecord(root, traceID, TrailWorkRecord{Revision: 1, Completion: &completion}); err != nil {
		t.Fatal(err)
	}
	if err := WriteTrailWorkRecord(root, traceID, TrailWorkRecord{Revision: 2}); err != nil {
		t.Fatal(err)
	}

	metrics, err := BuildTrailMetrics(root, traceID, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Work.Revision == nil || *metrics.Work.Revision != 2 {
		t.Fatalf("revision = %v, want 2", metrics.Work.Revision)
	}
	if metrics.Work.Completion != nil {
		t.Fatalf("an old completion described revision 2: %+v", metrics.Work.Completion)
	}
}
