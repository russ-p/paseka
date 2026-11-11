package runs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTrailRejectionRecordAndSummary(t *testing.T) {
	root := t.TempDir()
	trail := Trail{ColonyRoot: root, TraceID: "trace-a"}
	first := time.Now().UTC().Add(-time.Minute)
	if err := trail.AppendRejection(RejectedEvent{
		EventID:    "evt-1",
		TraceID:    "trace-a",
		Type:       "SIGNAL",
		Kind:       "task.status",
		AgentID:    "console",
		Subject:    "paseka.test.events.SIGNAL.task.status",
		Attempts:   5,
		Error:      "first failure",
		RejectedAt: first,
	}); err != nil {
		t.Fatal(err)
	}
	if err := trail.AppendRejection(RejectedEvent{
		EventID:    "evt-2",
		TraceID:    "trace-a",
		Type:       "SIGNAL",
		Kind:       "task.status",
		Subject:    "paseka.test.events.SIGNAL.task.status",
		Attempts:   5,
		Error:      "newest failure",
		RejectedAt: first.Add(30 * time.Second),
	}); err != nil {
		t.Fatal(err)
	}
	// A torn append must not hide the parseable rows before it.
	f, err := os.OpenFile(trail.RejectionsPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"eventId\":\"torn\"\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListTrailRejections(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v, want one trail", summaries)
	}
	got := summaries[0]
	if got.TraceID != "trace-a" || got.Count != 2 {
		t.Fatalf("summary = %+v, want trace-a with 2 records", got)
	}
	if got.LastError != "newest failure" {
		t.Fatalf("last error = %q, want newest failure", got.LastError)
	}
	if !got.NewestAt.Equal(first.Add(30 * time.Second)) {
		t.Fatalf("newest at = %s", got.NewestAt)
	}
}

func TestListTrailRejectionsMissingTree(t *testing.T) {
	root := t.TempDir()
	summaries, err := ListTrailRejections(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Fatalf("summaries = %+v, want none", summaries)
	}
	if _, err := ListTrailRejections(""); err == nil {
		t.Fatal("expected an error for an empty colony root")
	}
}

func TestRejectionsPathSitsBesideTrailEvents(t *testing.T) {
	trail := Trail{ColonyRoot: "/colony", TraceID: "trace-1"}
	want := filepath.Join("/colony", ".paseka", "runs", "trace-1", RejectedEventsFileName)
	if got := trail.RejectionsPath(); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
