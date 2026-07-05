package runs_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestDirPaths(t *testing.T) {
	d := runs.Dir{ColonyRoot: "/colony", TraceID: "trace-1", AgentID: "agent-a"}

	wantRoot := filepath.Join("/colony", ".paseka", "runs", "trace-1", "agent-a")
	if d.Root() != wantRoot {
		t.Fatalf("Root() = %q, want %q", d.Root(), wantRoot)
	}
	if d.ResultPath() != filepath.Join(wantRoot, "result.txt") {
		t.Fatal("unexpected ResultPath")
	}
	if d.RequestPath() != filepath.Join(wantRoot, "request.json") {
		t.Fatal("unexpected RequestPath")
	}
	if d.EventsPath() != filepath.Join(wantRoot, "events.ndjson") {
		t.Fatal("unexpected EventsPath")
	}
	if d.SessionPath() != filepath.Join(wantRoot, "session.json") {
		t.Fatal("unexpected SessionPath")
	}
	if d.TranscriptPath() != filepath.Join(wantRoot, "transcript.ndjson") {
		t.Fatal("unexpected TranscriptPath")
	}
}

func TestPrepareAndResult(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "t1", AgentID: "a1"}

	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WritePrompt("hello"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(d.ResultPath(), []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := d.ReadResult()
	if err != nil || got != "done" {
		t.Fatalf("ReadResult() = %q, %v", got, err)
	}
}

func TestRequestEventsResultProtocol(t *testing.T) {
	root := t.TempDir()
	d := runs.Dir{ColonyRoot: root, TraceID: "t1", AgentID: "a1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	req := protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "t1",
		AgentID:         "a1",
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          "task-1",
		Task:            "ship feature",
		ResultPath:      d.ResultPath(),
		EventLogPath:    d.EventsPath(),
		CreatedAt:       now,
	}
	if err := d.WriteRequest(req); err != nil {
		t.Fatal(err)
	}
	gotReq, err := d.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if gotReq.Task != "ship feature" {
		t.Fatalf("task = %q", gotReq.Task)
	}
	if gotReq.TaskID != "task-1" {
		t.Fatalf("taskId = %q", gotReq.TaskID)
	}

	ev, err := protocol.NewEvent("t1", "a1", 0, protocol.EventLog, map[string]string{"line": "start"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	ev2, err := protocol.NewEvent("t1", "a1", 0, protocol.EventProgress, map[string]string{"pct": "50"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev2); err != nil {
		t.Fatal(err)
	}
	events, err := d.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("events = %+v", events)
	}

	res := protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         "t1",
		AgentID:         "a1",
		Status:          protocol.StatusCompleted,
		Summary:         "done",
		FinishedAt:      now,
	}
	if err := d.WriteResult(res); err != nil {
		t.Fatal(err)
	}
	gotRes, err := d.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if gotRes.Summary != "done" {
		t.Fatalf("summary = %q", gotRes.Summary)
	}

	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		FinishedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusCompleted {
		t.Fatalf("state = %q", snap.State)
	}
}
