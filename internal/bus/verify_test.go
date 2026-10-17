package bus

import (
	"fmt"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// fakeStream is what the broker holds for one trace, and whether that read
// reached the end of the stream.
type fakeStream struct {
	events   []protocol.Event
	complete bool
	err      error
}

func (f fakeStream) ReplayTraceExhaustive(string) ([]protocol.Event, bool, error) {
	return f.events, f.complete, f.err
}

func verifyEvent(t *testing.T, root, traceID string, ev protocol.Event) {
	t.Helper()
	if err := (runs.Trail{ColonyRoot: root, TraceID: traceID}).AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyTraceSeparatesLocalFromBus(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-verify"
	local := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	localOnly := recordableEvent(t, traceID, "builder", protocol.EventMutation, "code.proposal.isolated")
	busOnly := recordableEvent(t, traceID, "guard", protocol.EventVerification, "verification.success")

	verifyEvent(t, root, traceID, local)
	verifyEvent(t, root, traceID, localOnly)

	report, err := VerifyTrace(root, traceID, fakeStream{events: []protocol.Event{local, busOnly}, complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Both) != 1 || report.Both[0].ID != local.ID {
		t.Fatalf("both = %+v, want the event in both records", report.Both)
	}
	if len(report.LocalOnly) != 1 || report.LocalOnly[0].ID != localOnly.ID {
		t.Fatalf("localOnly = %+v, want the event the broker never took", report.LocalOnly)
	}
	if len(report.BusOnly) != 1 || report.BusOnly[0].ID != busOnly.ID {
		t.Fatalf("busOnly = %+v, want the event missing from the local record", report.BusOnly)
	}
	if !report.Diverged() {
		t.Fatal("report must not claim agreement")
	}

	out := FormatVerifyReport(report)
	for _, want := range []string{"DIVERGED", "recorded locally but never broadcast", "broadcast but not in the local record", localOnly.ID, busOnly.ID} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestVerifyTraceAgreesWhenBothRecordsMatch(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-agree"
	local := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	verifyEvent(t, root, traceID, local)

	report, err := VerifyTrace(root, traceID, fakeStream{events: []protocol.Event{local}, complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Diverged() {
		t.Fatalf("report = %+v, want agreement", report)
	}
	if !strings.Contains(FormatVerifyReport(report), "agree") {
		t.Fatalf("output = %s", FormatVerifyReport(report))
	}
}

func TestVerifyTraceCountsDuplicatedLinesOnce(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-verify-dupes"

	// The record is append-only and its writer is at-least-once, so a re-flushed
	// event leaves two lines under one identity. That is one event, and a report
	// that calls it two turns an at-least-once bus into a divergence.
	once := recordableEvent(t, traceID, "builder", protocol.EventMutation, "code.proposal.isolated")
	twice := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	for _, ev := range []protocol.Event{once, once, twice, twice} {
		verifyEvent(t, root, traceID, ev)
	}

	raw, err := runs.ReadTrailEvents(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 {
		t.Fatalf("raw record holds %d lines, want 4 — the fixture must really duplicate", len(raw))
	}

	// Present on the bus, and one of them only locally.
	report, err := VerifyTrace(root, traceID, fakeStream{events: []protocol.Event{once}, complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.LocalCount != 2 {
		t.Fatalf("localCount = %d, want 2 distinct events", report.LocalCount)
	}
	if len(report.Both) != 1 {
		t.Fatalf("both = %+v, want one row for the duplicated event", report.Both)
	}
	if len(report.LocalOnly) != 1 {
		t.Fatalf("localOnly = %+v, want one row — a duplicated line is not a second event", report.LocalOnly)
	}
	if len(report.Both)+len(report.LocalOnly) != report.LocalCount {
		t.Fatalf("rows (%d + %d) do not add up to localCount %d", len(report.Both), len(report.LocalOnly), report.LocalCount)
	}
	if report.LocalOnly[0].ID != twice.ID {
		t.Fatalf("localOnly identity = %q, want %q", report.LocalOnly[0].ID, twice.ID)
	}
}

func TestVerifyTraceDuplicatedBusLinesCountOnce(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-verify-bus-dupes"
	ev := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	verifyEvent(t, root, traceID, ev)

	report, err := VerifyTrace(root, traceID, fakeStream{events: []protocol.Event{ev, ev}, complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.BusCount != 1 {
		t.Fatalf("busCount = %d, want 1 distinct event", report.BusCount)
	}
	if len(report.Both) != 1 || report.Diverged() {
		t.Fatalf("report = %+v, want one row and no divergence", report)
	}
}

func TestVerifyTraceMarksPreIdentityEventsUnverifiable(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-legacy-verify"
	legacy := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	legacy.ID = ""
	verifyEvent(t, root, traceID, legacy)

	report, err := VerifyTrace(root, traceID, fakeStream{events: []protocol.Event{legacy}, complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Unverifiable) != 2 {
		t.Fatalf("unverifiable = %+v, want both copies reported rather than guessed", report.Unverifiable)
	}
	if report.Diverged() {
		t.Fatal("an unmatchable record is not a divergence")
	}
}

func TestVerifyTraceRequiresReplayer(t *testing.T) {
	root := t.TempDir()
	if _, err := VerifyTrace(root, "trace-none", nil); err == nil {
		t.Fatal("verification without a broker must not report agreement")
	}
	var nilClient *Client
	if _, err := VerifyTrace(root, "trace-none", nilClient); err == nil {
		t.Fatal("a typed-nil replayer must not report agreement")
	}
}

func TestVerifyTraceEmptyTrail(t *testing.T) {
	root := t.TempDir()
	report, err := VerifyTrace(root, "trace-empty", fakeStream{complete: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.LocalCount != 0 || report.BusCount != 0 || report.Diverged() {
		t.Fatalf("report = %+v", report)
	}
	if !strings.Contains(FormatVerifyReport(report), "0 event(s)") {
		t.Fatalf("output = %s", FormatVerifyReport(report))
	}
}

func TestVerifyTraceNeedsTraceAndRoot(t *testing.T) {
	if _, err := VerifyTrace("", "trace-x", fakeStream{complete: true}); err == nil {
		t.Fatal("expected an error for a missing colony root")
	}
	if _, err := VerifyTrace(t.TempDir(), "", fakeStream{complete: true}); err == nil {
		t.Fatal("expected an error for a missing trace id")
	}
	if _, err := VerifyTrace(t.TempDir(), "trace-x", fakeStream{complete: true, err: fmt.Errorf("stream gone")}); err == nil {
		t.Fatal("a failing replayer must not report agreement")
	}
}

var _ = fmt.Sprint

func TestVerifyTraceTreatsAnIncompleteStreamAsUnknownNotDiverged(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-verify-incomplete"
	local := recordableEvent(t, traceID, "cli", protocol.EventSignal, "feature.requested")
	verifyEvent(t, root, traceID, local)

	// The broker read stopped early, so the event really did reach the bus but the
	// read never saw it. Reporting LocalOnly would invent a divergence out of a
	// slow stream.
	report, err := VerifyTrace(root, traceID, fakeStream{events: nil, complete: false})
	if err != nil {
		t.Fatal(err)
	}
	if report.StreamComplete {
		t.Fatal("report claims a complete read")
	}
	if report.Diverged() {
		t.Fatalf("report = %+v, want no divergence from an incomplete read", report)
	}
	out := FormatVerifyReport(report)
	if !strings.Contains(out, "INCONCLUSIVE") {
		t.Fatalf("output must say the comparison is inconclusive:\n%s", out)
	}
	if strings.Contains(out, "agree") {
		t.Fatalf("an incomplete read must not claim agreement:\n%s", out)
	}
	if !strings.Contains(out, "local record: 1 event(s)") {
		t.Fatalf("output lost the local count:\n%s", out)
	}
}
