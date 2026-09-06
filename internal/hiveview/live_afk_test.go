package hiveview_test

import (
	"os"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestLiveAFKOnTraceIgnoresSessionsAndOtherTrails(t *testing.T) {
	root := t.TempDir()
	writeHiveLiveAFK(t, root, "trail-daily-triage", "watch", os.Getpid(), false)
	writeHiveLiveAFK(t, root, "trail-other", "scout", os.Getpid(), false)
	writeHiveLiveAFK(t, root, "trail-daily-triage", "chat", os.Getpid(), true)

	items, err := hiveview.LiveAFKOnTrace(root, "trail-daily-triage")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Bee != "watch" || items[0].TraceID != "trail-daily-triage" {
		t.Fatalf("items = %+v", items)
	}

	missing, err := hiveview.LiveAFKOnTrace(root, "trail-missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %+v", missing)
	}
}

func writeHiveLiveAFK(t *testing.T, root, traceID, agentID string, pid int, session bool) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             agentID,
		Adapter:         "script",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		PID:             pid,
		StartedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if session {
		if err := os.WriteFile(d.SessionPath(), []byte(`{"sessionId":"sess-1"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
