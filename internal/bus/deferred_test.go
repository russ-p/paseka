package bus

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

type failingPublisher struct {
	failOn int
	calls  int
	events []protocol.Event
}

func (p *failingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.calls++
	p.events = append(p.events, ev)
	if p.calls == p.failOn {
		return errors.New("publish boom")
	}
	return nil
}

func prepareRun(t *testing.T, root, traceID, agentID string) runs.Dir {
	t.Helper()
	runDir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := runDir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runDir.RequestPath(), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return runDir
}

func TestProcessEventInputDeferQueuesWithoutPublish(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")

	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"context.note","summary":"queued"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", true, true, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.Deferred {
		t.Fatalf("result = %#v", result)
	}
	if result.Kind != "context.note" {
		t.Fatalf("kind = %q", result.Kind)
	}

	summary, err := runDir.PendingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 1 || summary.Kinds[0] != "context.note" {
		t.Fatalf("pending = %#v", summary)
	}
	events, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("audit events = %d, want 0", len(events))
	}
}

func TestProcessEventInputDeferDenyList(t *testing.T) {
	root := t.TempDir()
	prepareRun(t, root, "trace-1", "agent-1")

	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"SIGNAL","payload":{"kind":"task.status","taskId":"t1","status":"blocked"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", true, true, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "defer_denied" {
		t.Fatalf("result = %#v", result)
	}
}

func TestProcessEventInputDeferRequiresRunDir(t *testing.T) {
	root := t.TempDir()
	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"context.note","summary":"x"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", true, true, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "defer_failed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestProcessEventInputLiveEmitUnchanged(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")

	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"VERIFICATION","payload":{"kind":"verification.success","summary":"ok"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", true, false, root)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("expected nats_not_configured without client, got %#v", result)
	}
	if result.Error != "nats_not_configured" || result.Deferred {
		t.Fatalf("result = %#v", result)
	}
	events, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("pending = %d", len(events))
	}
}

func TestFlushPendingFIFOAndAudit(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")

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

	pub := &failingPublisher{}
	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Published != 2 {
		t.Fatalf("result = %#v", result)
	}
	if len(pub.events) != 2 {
		t.Fatalf("published = %d", len(pub.events))
	}
	if protocol.PayloadKind(pub.events[0].Payload) != "context.note" {
		t.Fatalf("first kind = %q", protocol.PayloadKind(pub.events[0].Payload))
	}

	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d", len(pending))
	}
	audit, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 2 {
		t.Fatalf("audit = %d", len(audit))
	}
}

func TestFlushPendingDiscard(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}

	pub := &failingPublisher{}
	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Discarded != 1 || result.Published != 0 {
		t.Fatalf("result = %#v", result)
	}
	if pub.calls != 0 {
		t.Fatal("expected no publish on discard")
	}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d", len(pending))
	}
}

func TestFlushPendingEmptyNoOp(t *testing.T) {
	root := t.TempDir()
	prepareRun(t, root, "trace-1", "agent-1")
	pub := &failingPublisher{}

	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Published != 0 {
		t.Fatalf("result = %#v", result)
	}

	result, err = FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("second flush = %#v", result)
	}
}

func TestFlushPendingMidFlushFailureResumes(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")

	for i, summary := range []string{"one", "two", "three"} {
		ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, map[string]any{
			"kind":    "context.note",
			"summary": summary,
		})
		if err != nil {
			t.Fatal(err)
		}
		ev.Seq = i + 1
		if err := runDir.AppendPending(ev); err != nil {
			t.Fatal(err)
		}
	}

	pub := &failingPublisher{failOn: 2}
	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Published != 1 || result.Error != "publish_failed" {
		t.Fatalf("result = %#v", result)
	}

	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending = %d, want 2", len(pending))
	}
	audit, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 {
		t.Fatalf("audit = %d, want 1", len(audit))
	}

	pub = &failingPublisher{}
	result, err = FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Published != 2 {
		t.Fatalf("retry result = %#v", result)
	}
}

func TestFlushPendingFileRefMissing(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")

	payload, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/missing.md",
	})
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventSignal, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}

	pub := &failingPublisher{}
	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Error != "file_ref_missing" {
		t.Fatalf("result = %#v", result)
	}
	if pub.calls != 0 {
		t.Fatal("expected no publish when file missing")
	}
}

func TestFlushPendingFileRefExists(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	specPath := filepath.Join(root, "docs", "specs", "ready.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(specPath, []byte("# ready"), 0o644); err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{
		"kind": "spec.ready",
		"ref":  "docs/specs/ready.md",
	})
	ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventSignal, json.RawMessage(payload))
	if err != nil {
		t.Fatal(err)
	}
	if err := runDir.AppendPending(ev); err != nil {
		t.Fatal(err)
	}

	pub := &failingPublisher{}
	result, err := FlushPending(context.Background(), pub, root, "trace-1", "agent-1", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Published != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestInspectPending(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	for _, kind := range []string{"context.note", "task.plan"} {
		payload, _ := json.Marshal(map[string]any{"kind": kind, "summary": "x", "tasks": []map[string]string{{"taskId": "t1", "title": "T"}}})
		ev, err := protocol.NewEvent("trace-1", "agent-1", 0, protocol.EventInsight, json.RawMessage(payload))
		if err != nil {
			t.Fatal(err)
		}
		if err := runDir.AppendPending(ev); err != nil {
			t.Fatal(err)
		}
	}

	result, err := InspectPending(root, "trace-1", "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 2 {
		t.Fatalf("result = %#v", result)
	}
	if strings.Join(result.Kinds, ",") != "context.note,task.plan" {
		t.Fatalf("kinds = %#v", result.Kinds)
	}
}

func TestIsDeferDenied(t *testing.T) {
	if !IsDeferDenied("system.kill") {
		t.Fatal("expected deny")
	}
	if IsDeferDenied("context.note") {
		t.Fatal("expected allow")
	}
}
