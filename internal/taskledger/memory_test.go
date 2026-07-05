package taskledger_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestMemoryLedgerApplyTaskPlanAndReady(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	plan := mustEvent(t, "trace-1", protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Build", Bee: "builder"},
		},
	})
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}

	ready := mustEvent(t, "trace-1", protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
	})
	res, err := ledger.Apply(ready)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ready) != 1 || res.Ready[0].TaskID != "task-1" {
		t.Fatalf("ready = %+v", res.Ready)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks["task-1"].Status != protocol.TaskStatusReady {
		t.Fatalf("status = %q", snap.Tasks["task-1"].Status)
	}
}

func mustEvent(t *testing.T, traceID string, typ protocol.EventType, payload any) protocol.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		Type:            typ,
		CreatedAt:       time.Now().UTC(),
		Payload:         raw,
	}
}
