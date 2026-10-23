package bus

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestDeferredEventKeepsRunAttemptProvenance(t *testing.T) {
	root := t.TempDir()
	runDir := prepareRun(t, root, "trace-1", "agent-1")
	want := protocol.Provenance{
		AttemptID:            "attempt-1",
		ConfigRef:            "sha256:0000000000000000000000000000000000000000000000000000000000000000",
		ConfigRevision:       2,
		InitialCodeBaseSHA:   "abc123",
		InitialCodeBaseDirty: true,
	}
	if err := runDir.WriteRequest(protocol.Request{
		ProtocolVersion:      protocol.Version,
		TraceID:              "trace-1",
		AgentID:              "agent-1",
		Bee:                  "builder",
		Adapter:              "cursor",
		AttemptID:            want.AttemptID,
		ConfigRef:            want.ConfigRef,
		ConfigRevision:       want.ConfigRevision,
		InitialCodeBaseSHA:   want.InitialCodeBaseSHA,
		InitialCodeBaseDirty: want.InitialCodeBaseDirty,
	}); err != nil {
		t.Fatal(err)
	}

	raw := []byte(`{"traceId":"trace-1","agentId":"agent-1","type":"INSIGHT","payload":{"kind":"context.note","summary":"queued"}}`)
	result, err := ProcessEventInput(context.Background(), nil, "", raw, "agent-1", true, true, root)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.Deferred {
		t.Fatalf("result = %#v", result)
	}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d", len(pending))
	}
	if pending[0].Provenance != want {
		t.Fatalf("pending provenance = %+v, want %+v", pending[0].Provenance, want)
	}
}
