package runs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func bigMutationEvent(t *testing.T, traceID, agentID string, diffBytes int) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:    protocol.MutationCodeProposalIsolated,
		Summary: "large proposal",
		Diff:    strings.Repeat("x", diffBytes),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

// A MUTATION with an inline diff past bufio's 64 KiB default is a valid event:
// protocol validation requires only one of diff/ref/summary and sets no size
// bound. It used to make the run's whole event log unreadable, and because the
// failure was returned rather than skipped, it failed every surface that merges
// run logs — the colony-wide Console timeline included.
func TestReadEventsAcceptsOversizedInlineDiff(t *testing.T) {
	root := t.TempDir()
	d := Dir{ColonyRoot: root, TraceID: "trace-big", AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(bigMutationEvent(t, "trace-big", "builder-1", 200*1024)); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(d.EventsPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= 64*1024 {
		t.Fatalf("fixture is %d bytes, which does not cross the old 64 KiB limit", info.Size())
	}

	events, err := d.ReadEvents()
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if got := protocol.PayloadKind(events[0].Payload); got != string(protocol.MutationCodeProposalIsolated) {
		t.Fatalf("payload kind = %q, want %q", got, protocol.MutationCodeProposalIsolated)
	}
}

func TestReadTraceEventsReadsOversizedInlineDiff(t *testing.T) {
	root := t.TempDir()
	d := Dir{ColonyRoot: root, TraceID: "trace-big", AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(bigMutationEvent(t, "trace-big", "builder-1", 512*1024)); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, "trace-big")
	if err != nil {
		t.Fatalf("ReadTraceEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

// An unreadable run log costs its own trail its events and nothing else. The
// timeline, export, and prompt memory all read through this function, so a hard
// failure here used to blank every trail in the colony along with the broken one.
func TestReadTraceEventsSkipsUnreadableRunLog(t *testing.T) {
	root := t.TempDir()
	broken := Dir{ColonyRoot: root, TraceID: "trace-mixed", AgentID: "builder-broken"}
	if err := broken.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(broken.EventsPath(), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	healthy := Dir{ColonyRoot: root, TraceID: "trace-mixed", AgentID: "builder-ok"}
	if err := healthy.Prepare(); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trace-mixed", "builder-ok", 0, protocol.EventSignal, map[string]any{
		"kind":   "task.ready",
		"taskId": "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := healthy.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	events, err := ReadTraceEvents(root, "trace-mixed")
	if err != nil {
		t.Fatalf("ReadTraceEvents: %v", err)
	}
	if len(events) != 1 || events[0].AgentID != "builder-ok" {
		t.Fatalf("events = %+v, want the one readable run's event", events)
	}
}

func TestReadPendingAcceptsOversizedLine(t *testing.T) {
	root := t.TempDir()
	d := Dir{ColonyRoot: root, TraceID: "trace-pending", AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendPending(bigMutationEvent(t, "trace-pending", "builder-1", 200*1024)); err != nil {
		t.Fatal(err)
	}

	events, err := d.ReadPending()
	if err != nil {
		t.Fatalf("ReadPending: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
}

func TestReadTranscriptAcceptsOversizedContent(t *testing.T) {
	root := t.TempDir()
	d := Dir{ColonyRoot: root, TraceID: "trace-tty", AgentID: "session-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendTranscript(TranscriptEntry{Content: strings.Repeat("y", 300*1024)}); err != nil {
		t.Fatal(err)
	}

	entries, err := d.ReadTranscript()
	if err != nil {
		t.Fatalf("ReadTranscript: %v", err)
	}
	if len(entries) != 1 || len(entries[0].Content) != 300*1024 {
		t.Fatalf("entries = %d entries, content len %d, want 1 entry of 300 KiB", len(entries), len(entries[0].Content))
	}
}

// The bound is a ceiling, not a license: a line past it is still a read error
// rather than unbounded allocation.
func TestReadEventsRejectsLinePastMaxLineBytes(t *testing.T) {
	root := t.TempDir()
	d := Dir{ColonyRoot: root, TraceID: "trace-huge", AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	huge := filepath.Join(d.Root(), EventsFileName)
	if err := os.WriteFile(huge, []byte(strings.Repeat("z", maxLineBytes+16)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ReadEvents(); err == nil {
		t.Fatal("ReadEvents accepted a line past maxLineBytes, want an error")
	}
}
