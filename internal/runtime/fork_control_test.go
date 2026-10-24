package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestReactorRecordsAgentForkRequestAsPending(t *testing.T) {
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{{
				EventRule: colony.EventRule{Type: "SIGNAL", Kind: string(protocol.SignalTrailFork)},
			}},
		},
	})
	ctxColony := colony.Context{ColonyRoot: root}
	if _, err := trailconfig.Ensure(ctxColony, "trace-source"); err != nil {
		t.Fatal(err)
	}
	if err := runs.WriteTaskSnapshot(root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "implement task-1",
		Bee:    "builder",
		Status: protocol.TaskStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.EnsureTaskAttempt(ctxColony, "trace-source", "task-1", "evt-1",
		trailconfig.NewTaskInputEvidence("task-1", "implement task-1", "body", "builder", "", "feature", string(protocol.TaskReviewNone))); err != nil {
		t.Fatal(err)
	}
	writeForkSenderRun(t, root, "trace-source", "agent-sender")

	ev, err := protocol.NewEvent("trace-source", "agent-sender", 1, protocol.EventSignal, protocol.ForkRequestPayload{
		Kind:        protocol.SignalTrailFork,
		RequestID:   "req-runtime",
		SourceTrace: "trace-source",
		Delta:       trailfork.DeltaNone,
		Title:       "runtime request",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	pending, err := trailfork.ListPending(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].RequestID != "req-runtime" || pending[0].State != trailfork.StatePending {
		t.Fatalf("pending = %+v", pending)
	}
	if pending[0].TraceID != "" || pending[0].Lineage != nil {
		t.Fatalf("runtime request minted runnable state: %+v", pending[0])
	}
}

func TestReactorRefusesAgentDecisionSignal(t *testing.T) {
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{{
				EventRule: colony.EventRule{Type: "SIGNAL", Kind: string(protocol.SignalTrailFork)},
			}},
		},
	})
	ctxColony := colony.Context{ColonyRoot: root}
	if _, err := trailconfig.Ensure(ctxColony, "trace-source"); err != nil {
		t.Fatal(err)
	}
	if err := runs.WriteTaskSnapshot(root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "implement task-1",
		Bee:    "builder",
		Status: protocol.TaskStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.EnsureTaskAttempt(ctxColony, "trace-source", "task-1", "evt-1",
		trailconfig.NewTaskInputEvidence("task-1", "implement task-1", "body", "builder", "", "feature", string(protocol.TaskReviewNone))); err != nil {
		t.Fatal(err)
	}
	writeForkSenderRun(t, root, "trace-source", "agent-sender")
	if _, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctxColony,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-runtime",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
	}); err != nil {
		t.Fatal(err)
	}

	ev, err := protocol.NewEvent("trace-source", "agent-sender", 1, protocol.EventSignal, protocol.ForkDecisionPayload{
		Kind:      protocol.SignalTrailForkApprove,
		RequestID: "req-runtime",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	pending, err := trailfork.ListPending(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].State != trailfork.StatePending {
		t.Fatalf("agent decision changed the request: %+v", pending)
	}
}

func TestReactorRefusesForkDecisionWithoutOperatorMarker(t *testing.T) {
	for _, sender := range []string{"", "unrecorded-agent"} {
		t.Run(sender, func(t *testing.T) {
			root := preparePendingFork(t)
			ev, err := protocol.NewEvent("trace-source", sender, 1, protocol.EventSignal, protocol.ForkDecisionPayload{
				Kind:      protocol.SignalTrailForkReject,
				RequestID: "req-runtime",
			})
			if err != nil {
				t.Fatal(err)
			}
			r := runtime.NewTestReactor(runtime.TestReactorOptions{
				ColonyRoot: root,
				Ledger:     taskledger.NewMemoryLedger(),
			})
			if err := r.ProcessEvent(context.Background(), ev); err != nil {
				t.Fatal(err)
			}
			pending, err := trailfork.ListPending(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(pending) != 1 || pending[0].State != trailfork.StatePending {
				t.Fatalf("sender %q changed the request: %+v", sender, pending)
			}
		})
	}
}

func TestReactorAcceptsForkDecisionFromOperatorMarker(t *testing.T) {
	root := preparePendingFork(t)
	ev, err := protocol.NewEvent("trace-source", protocol.OperatorAgentID, 1, protocol.EventSignal, protocol.ForkDecisionPayload{
		Kind:      protocol.SignalTrailForkReject,
		RequestID: "req-runtime",
		Reason:    "not now",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Ledger:     taskledger.NewMemoryLedger(),
	})
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	req, err := trailfork.LoadRequest(root, "req-runtime")
	if err != nil {
		t.Fatal(err)
	}
	if req.State != trailfork.StateRejected || req.DecidedBy != protocol.OperatorAgentID || req.Reason != "not now" {
		t.Fatalf("request = %+v", req)
	}
}

func preparePendingFork(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{{
				EventRule: colony.EventRule{Type: "SIGNAL", Kind: string(protocol.SignalTrailFork)},
			}},
		},
	})
	ctxColony := colony.Context{ColonyRoot: root}
	if _, err := trailconfig.Ensure(ctxColony, "trace-source"); err != nil {
		t.Fatal(err)
	}
	if err := runs.WriteTaskSnapshot(root, "trace-source", taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "implement task-1",
		Bee:    "builder",
		Status: protocol.TaskStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := trailconfig.EnsureTaskAttempt(ctxColony, "trace-source", "task-1", "evt-1",
		trailconfig.NewTaskInputEvidence("task-1", "implement task-1", "body", "builder", "", "feature", string(protocol.TaskReviewNone))); err != nil {
		t.Fatal(err)
	}
	writeForkSenderRun(t, root, "trace-source", "agent-sender")
	if _, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctxColony,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-runtime",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
	}); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeForkSenderRun(t *testing.T, root, traceID, agentID string) {
	t.Helper()
	dir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := dir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "builder",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          "task-1",
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}
