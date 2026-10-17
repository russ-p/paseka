package runs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestReadTraceEventsIncludesConsoleAudit(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-console-events"
	beeDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "agent-1"}
	if err := beeDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	consoleDir := Dir{ColonyRoot: root, TraceID: traceID, AgentID: "console"}
	if err := consoleDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent(traceID, "console", 0, protocol.EventSignal, map[string]any{
		"kind": "artifact.written",
		"artifacts": []map[string]any{{
			"ref":          ".paseka/runs/" + traceID + "/artifacts/notes.md",
			"artifactKind": "notes",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := consoleDir.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 (console audit must not be skipped)", len(events))
	}
	if events[0].AgentID != "console" {
		t.Fatalf("agentId = %q", events[0].AgentID)
	}
}

// A proposal store is reserved and skipped: its sidecars retain a synthesized event
// envelope, but that envelope is the archive's own context and must not become a trail
// event, and the directory must never be read as a Bee run.
func TestProposalStoreIsNotARunOrATrailEventSource(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-proposals"

	storeDir := filepath.Join(root, ".paseka", "runs", traceID, "proposals")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A sidecar that names the directory as if it were a run, with an event envelope that
	// would show up in the trail if the skip set were wrong.
	sidecar := `{"schemaVersion":1,"traceId":"` + traceID + `","agentId":"agent-1","publication":"published",` +
		`"bytes":4,"event":{"id":"ev-1","type":"MUTATION","agentId":"agent-1","traceId":"` + traceID + `","payload":{"kind":"code.proposal.isolated"}}}`
	if err := os.WriteFile(filepath.Join(storeDir, "agent-1.json"), []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storeDir, "agent-1.patch"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("proposal sidecar leaked into the trail events: %+v", events)
	}

	if !IsReservedTraceSubdir("proposals") {
		t.Fatal("proposals must be a reserved trace subdirectory")
	}
}
