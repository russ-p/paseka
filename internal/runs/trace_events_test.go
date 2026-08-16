package runs

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
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
