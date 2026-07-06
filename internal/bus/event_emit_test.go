package bus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func TestProcessEventInputValidateOnly(t *testing.T) {
	raw := []byte(`{"traceId":"trace-1","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"ok"}}`)
	result, err := ProcessEventInput(context.Background(), nil, raw, "agent-1", false, "")
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
	result, err := ProcessEventInput(context.Background(), nil, raw, "agent-1", false, "")
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
