package bus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func TestProcessEventInputValidateOnly(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"ok"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if result.Kind != "verification.success" {
		t.Fatalf("kind = %q", result.Kind)
	}
}

func TestProcessEventInputValidationFailure(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"VERIFICATION","payload":{"kind":"verification.success"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", false, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatal("expected validation failure")
	}
	if result.Error != "schema_validation_failed" {
		t.Fatalf("error = %q", result.Error)
	}
}

func TestWriteEventCLIResult(t *testing.T) {
	var buf strings.Builder
	result := protocol.EventCLIResult{OK: true, TraceID: "trace-1"}
	if err := WriteEventCLIResult(&buf, result); err != nil {
		t.Fatal(err)
	}
	var decoded protocol.EventCLIResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.OK || decoded.TraceID != "trace-1" {
		t.Fatalf("decoded = %#v", decoded)
	}
}

func TestAppendRunAuditLog(t *testing.T) {
	root := t.TempDir()
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-1", AgentID: "agent-1"}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runDir.RequestPath(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}

	path, err := appendRunAuditLog(root, ev)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, filepath.Join(".paseka", "runs", "trace-1", "agent-1", "events.ndjson")) {
		t.Fatalf("path = %q", path)
	}

	events, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != protocol.EventVerification {
		t.Fatalf("type = %q", events[0].Type)
	}
}

func TestProcessEventInputRefusesEmitWithoutRunAudit(t *testing.T) {
	// The run's own audit log is the emit's authority check. An emit from a
	// sender with no recorded run must fail before anything is published or
	// recorded on the trail.
	root := t.TempDir()
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"trail.fork.approve","requestId":"req-1"}}`)
	local := LocalPublisher{ColonyRoot: root}

	result, err := ProcessEventInput(context.Background(), local, "", raw, "cli", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "audit_log_failed" {
		t.Fatalf("result = %#v, want the per-run audit log to be required", result)
	}

	recorded, err := runs.ReadTrailEvents(root, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 0 {
		t.Fatalf("trail record holds %d events, want none before audit", len(recorded))
	}
	if result.Subject != "" {
		t.Fatalf("subject = %q, want empty — nothing was broadcast", result.Subject)
	}
}

func TestProcessEventInputNormalizesReviewNoteSeverity(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":" Medium "}}`)
	local := LocalPublisher{ColonyRoot: root}

	result, err := ProcessEventInput(context.Background(), local, "", raw, "agent-1", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}

	// The audit log and the published event are the same record: the stored
	// payload carries the canonical value, not the prompt's spelling.
	runEvents, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(runEvents) != 1 {
		t.Fatalf("run events = %d, want 1", len(runEvents))
	}
	if got := reviewNoteSeverity(t, runEvents[0]); got != "medium" {
		t.Fatalf("run audit severity = %q, want medium", got)
	}
	trailEvents, err := runs.ReadTraceEvents(root, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(trailEvents) != 1 {
		t.Fatalf("trail events = %d, want 1", len(trailEvents))
	}
	if got := reviewNoteSeverity(t, trailEvents[0]); got != "medium" {
		t.Fatalf("published severity = %q, want medium", got)
	}
}

func TestProcessEventInputRejectsUnknownReviewNoteSeverity(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"review.note","summary":"Missing retry handling","severity":"severe"}}`)
	local := LocalPublisher{ColonyRoot: root}

	result, err := ProcessEventInput(context.Background(), local, "", raw, "agent-1", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "schema_validation_failed" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Details) != 1 || result.Details[0].Path != "payload.severity" {
		t.Fatalf("details = %#v", result.Details)
	}
	for _, value := range protocol.ReviewSeverityValues() {
		if !strings.Contains(result.Details[0].Message, value) {
			t.Fatalf("message %q must name %q", result.Details[0].Message, value)
		}
	}
	runEvents, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(runEvents) != 0 {
		t.Fatalf("rejected emit recorded %d events", len(runEvents))
	}
}

func reviewNoteSeverity(t *testing.T, ev protocol.Event) string {
	t.Helper()
	var p protocol.NarrativeInsightPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		t.Fatal(err)
	}
	return p.Severity
}

func TestProcessEventInputRecordsLocallyFromARunDirectory(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	raw := []byte(`{"traceId":"trace-1","type":"SIGNAL","payload":{"kind":"feature.requested"}}`)
	local := LocalPublisher{ColonyRoot: root}

	result, err := ProcessEventInput(context.Background(), local, "", raw, "agent-1", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("result = %#v", result)
	}
	if result.EventLogPath == "" {
		t.Fatal("result must still name the run's event log")
	}

	// Both stores, one identity: the merged view shows the event once.
	merged, err := runs.ReadTraceEvents(root, "trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 1 {
		t.Fatalf("merged = %d events, want 1", len(merged))
	}
	runEvents, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(runEvents) != 1 || runEvents[0].ID != merged[0].ID {
		t.Fatalf("run log = %+v, trail row = %+v", runEvents, merged[0])
	}
}
