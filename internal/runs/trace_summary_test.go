package runs

import (
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

func TestResolveTraceSummaryLastWriteWins(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-summary-lww"
	base := time.Now().UTC()

	d1 := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d1.Prepare(); err != nil {
		t.Fatal(err)
	}
	first, err := protocol.NewEvent(traceID, "builder", 1, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "First summary",
	})
	if err != nil {
		t.Fatal(err)
	}
	first.CreatedAt = base
	if err := d1.AppendEvent(first); err != nil {
		t.Fatal(err)
	}

	d2 := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-2"}
	if err := d2.Prepare(); err != nil {
		t.Fatal(err)
	}
	otherKind, err := protocol.NewEvent(traceID, "scout", 1, protocol.EventInsight, protocol.TraceTitlePayload{
		Kind:  protocol.InsightTraceTitle,
		Title: "Trail title",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherKind.CreatedAt = base.Add(time.Minute)
	if err := d2.AppendEvent(otherKind); err != nil {
		t.Fatal(err)
	}

	later, err := protocol.NewEvent(traceID, "builder", 2, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "Latest summary",
	})
	if err != nil {
		t.Fatal(err)
	}
	later.CreatedAt = base.Add(2 * time.Minute)
	if err := d1.AppendEvent(later); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Latest summary" {
		t.Fatalf("summary = %q, want Latest summary", got)
	}
}

func TestResolveTraceSummarySeqTiebreak(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-summary-seq"
	at := time.Now().UTC()

	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	lowSeq, err := protocol.NewEvent(traceID, "builder", 1, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "Lower seq",
	})
	if err != nil {
		t.Fatal(err)
	}
	lowSeq.CreatedAt = at
	if err := d.AppendEvent(lowSeq); err != nil {
		t.Fatal(err)
	}

	highSeq, err := protocol.NewEvent(traceID, "builder", 2, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "Higher seq",
	})
	if err != nil {
		t.Fatal(err)
	}
	highSeq.CreatedAt = at
	if err := d.AppendEvent(highSeq); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Higher seq" {
		t.Fatalf("summary = %q, want Higher seq", got)
	}
}

func TestResolveTraceSummaryEmpty(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveTraceSummary(root, "trace-missing")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("summary = %q, want empty", got)
	}
}

func TestResolveTraceSummaryIgnoresRunSummary(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-no-run-fallback"
	at := time.Now().UTC()

	d := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	runSummary, err := protocol.NewEvent(traceID, "builder", 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "Task run outcome only",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	runSummary.CreatedAt = at
	if err := d.AppendEvent(runSummary); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveTraceSummary(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("summary = %q, want empty (no run.summary fallback)", got)
	}
}
