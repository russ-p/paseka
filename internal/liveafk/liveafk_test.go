package liveafk_test

import (
	"os"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/liveafk"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestScanColonyAndOnTraceSkipSessionsAndDeadRuns(t *testing.T) {
	root := t.TempDir()
	writeRun(t, root, "trail-daily-triage", "watch", os.Getpid(), protocol.StatusRunning, false)
	writeRun(t, root, "trail-daily-triage", "chat", os.Getpid(), protocol.StatusRunning, true)
	writeRun(t, root, "trace-bloom", "builder", os.Getpid(), protocol.StatusCompleted, false)

	all, err := liveafk.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || all[0].AgentID != "watch" {
		t.Fatalf("all = %+v", all)
	}

	onTrace, err := liveafk.OnTrace(root, "trail-daily-triage")
	if err != nil {
		t.Fatal(err)
	}
	if len(onTrace) != 1 || onTrace[0].Bee != "watch" || onTrace[0].PID != os.Getpid() {
		t.Fatalf("onTrace = %+v", onTrace)
	}

	other, err := liveafk.OnTrace(root, "trace-bloom")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Fatalf("completed run reported live: %+v", other)
	}

	if runsOnEmpty, err := liveafk.OnTrace(root, ""); err != nil || len(runsOnEmpty) != 0 {
		t.Fatalf("empty trace = %+v err=%v", runsOnEmpty, err)
	}
	if missing, err := liveafk.ScanColony(t.TempDir()); err != nil || len(missing) != 0 {
		t.Fatalf("colony without runs = %+v err=%v", missing, err)
	}
}

func writeRun(t *testing.T, root, traceID, agentID string, pid int, state protocol.RunStatus, session bool) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             agentID,
		Adapter:         "script",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		PID:             pid,
		StartedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if session {
		if err := os.WriteFile(d.SessionPath(), []byte(`{"sessionId":"sess-1"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
