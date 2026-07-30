package runs

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestPendingQueueFIFO(t *testing.T) {
	root := t.TempDir()
	runDir := Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}

	first, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(first); err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(second); err != nil {
		t.Fatal(err)
	}

	events, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("pending = %d", len(events))
	}

	if err := runDir.WritePending(events[1:]); err != nil {
		t.Fatal(err)
	}
	remaining, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining = %d", len(remaining))
	}

	if err := runDir.ClearPending(); err != nil {
		t.Fatal(err)
	}
	summary, err := runDir.PendingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 0 {
		t.Fatalf("count = %d", summary.Count)
	}
}
