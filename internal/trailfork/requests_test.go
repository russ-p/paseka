package trailfork_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestSubmitAgentRequestRecordsPendingWithoutRunnableState(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	req, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-1",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
			Title:       "explore control arm",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.State != trailfork.StatePending || req.Origin != "agent" || req.Sender.Bee != "builder" {
		t.Fatalf("request = %+v", req)
	}
	if req.TraceID != "" || req.ForkID != "" || req.Lineage != nil {
		t.Fatalf("pending request minted identity: %+v", req)
	}
	if req.Evidence.TraceID != "" || req.Evidence.ForkID != "" {
		t.Fatalf("pending evidence carries fork identity: %+v", req.Evidence)
	}
	path := trailfork.RequestPath(root, "trace-source", "req-1")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("request file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", "req-1")); !os.IsNotExist(err) {
		t.Fatalf("request created a trace dir: %v", err)
	}

	again, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-1",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
			Title:       "explore control arm",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !again.CreatedAt.Equal(req.CreatedAt) {
		t.Fatalf("retry rewrote request: %+v -> %+v", req, again)
	}

	_, err = trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-1",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
			Title:       "different",
		},
	})
	if err == nil || !strings.Contains(err.Error(), "already used with different contents") {
		t.Fatalf("digest mismatch error = %v", err)
	}

	pending, err := trailfork.ListPending(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].RequestID != "req-1" {
		t.Fatalf("pending = %+v", pending)
	}
}

func TestSubmitAgentRequestRefusesUngrantedAndUnrecordedSenders(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	_, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-ungranted",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "publication") {
		t.Fatalf("ungranted sender error = %v", err)
	}

	allowForkPublish(t, root)
	_, err = trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "missing"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-missing",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not a recorded bee run") {
		t.Fatalf("unrecorded sender error = %v", err)
	}
	if pending, err := trailfork.ListPending(root); err != nil || len(pending) != 0 {
		t.Fatalf("refused requests were retained: %+v %v", pending, err)
	}
}

func TestApproveRequestCreatesForkExactlyOnce(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-approve", "trace-source", "agent-sender")

	ledger := taskledger.NewMemoryLedger()
	pub := &capturePublisher{}
	result, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Actor:     "alice",
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.Plan.TraceID == "trace-source" || result.Plan.RequestID != req.RequestID {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(result.Worktree.Path); err != nil {
		t.Fatalf("fork worktree: %v", err)
	}
	if result.Plan.Task.TaskID == "" || result.Plan.ForkID == "" {
		t.Fatalf("approved plan lacks identity: %+v", result.Plan)
	}

	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateApproved || stored.DecidedBy != "alice" || stored.Lineage == nil {
		t.Fatalf("stored = %+v", stored)
	}
	if stored.TraceID != result.Plan.TraceID || stored.TaskID != result.Plan.Task.TaskID {
		t.Fatalf("stored identity = %+v, plan = %+v", stored, result.Plan)
	}

	again, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Actor:     "alice",
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Plan.TraceID != result.Plan.TraceID {
		t.Fatalf("repeat approval created a second fork: %s -> %s", result.Plan.TraceID, again.Plan.TraceID)
	}
	traceDirs, err := os.ReadDir(filepath.Join(root, ".paseka", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	forks := 0
	for _, entry := range traceDirs {
		if entry.Name() == result.Plan.TraceID {
			forks++
		}
	}
	if forks != 1 {
		t.Fatalf("fork trace dirs = %d", forks)
	}
}

func TestRejectAndCancelLeavePendingOnly(t *testing.T) {
	for _, tc := range []struct {
		name   string
		state  string
		decide func(trailfork.DecisionOptions) (trailfork.Request, error)
	}{
		{"reject", trailfork.StateRejected, trailfork.RejectRequest},
		{"cancel", trailfork.StateCancelled, trailfork.CancelRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, _ := setupGitRepo(t)
			ctx := testContext(root)
			allowForkPublish(t, root)
			writeSource(t, ctx, "trace-source", "task-1", nil)
			writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
			req := submitRequest(t, ctx, "req-"+tc.name, "trace-source", "agent-sender")

			decided, err := tc.decide(trailfork.DecisionOptions{
				Context:   ctx,
				RequestID: req.RequestID,
				Actor:     "alice",
				Reason:    "not useful",
			})
			if err != nil {
				t.Fatal(err)
			}
			if decided.State != tc.state || decided.Reason != "not useful" || decided.DecidedBy != "alice" {
				t.Fatalf("decided = %+v", decided)
			}
			if decided.TraceID != "" {
				t.Fatalf("decision minted a trace: %q", decided.TraceID)
			}
			if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", decided.RequestID)); !os.IsNotExist(err) {
				t.Fatalf("decision created a trace dir: %v", err)
			}
			pending, err := trailfork.ListPending(root)
			if err != nil || len(pending) != 0 {
				t.Fatalf("pending = %+v %v", pending, err)
			}
			if _, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
				Context:   ctx,
				RequestID: req.RequestID,
				Actor:     "alice",
			}); err == nil {
				t.Fatal("decided request was approved")
			}
		})
	}
}

func TestExpiredRequestIsNotApprovable(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	req, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-expired",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
		Now: time.Now().Add(-2 * time.Hour),
		TTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := trailfork.ListPending(root)
	if err != nil || len(pending) != 0 {
		t.Fatalf("expired request listed: %+v %v", pending, err)
	}
	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateExpired {
		t.Fatalf("state = %s", stored.State)
	}
	if _, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Publisher: &capturePublisher{},
		Ledger:    taskledger.NewMemoryLedger(),
	}); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("approve expired error = %v", err)
	}
}

func TestApproveRefusesChangedEvidence(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-changed", "trace-source", "agent-sender")

	attemptsPath := trailconfig.AttemptsPath(root, "trace-source")
	data, err := os.ReadFile(attemptsPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SchemaVersion int                   `json:"schemaVersion"`
		Attempts      []trailconfig.Attempt `json:"attempts"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for i := range doc.Attempts {
		if doc.Attempts[i].TaskInput != nil {
			doc.Attempts[i].TaskInput.Title = "changed after the request"
		}
	}
	data, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(attemptsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Publisher: &capturePublisher{},
		Ledger:    taskledger.NewMemoryLedger(),
	})
	if err == nil || !strings.Contains(err.Error(), "retained evidence changed") {
		t.Fatalf("approve changed evidence error = %v", err)
	}
	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateRefused {
		t.Fatalf("state = %s", stored.State)
	}
	if stored.TraceID == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", stored.TraceID)); !os.IsNotExist(err) {
		t.Fatalf("refused request created trace state: %v", err)
	}
}

func TestPendingQuotaBoundsRequests(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	if _, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-quota-a",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
		Quota: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   "req-quota-over",
			SourceTrace: "trace-source",
			Delta:       trailfork.DeltaNone,
		},
		Quota: 1,
	}); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("quota error = %v", err)
	}
}

func submitRequest(t *testing.T, ctx colony.Context, requestID, sourceTrace, agentID string) trailfork.Request {
	t.Helper()
	req, err := trailfork.SubmitAgentRequest(trailfork.RequestOptions{
		Context: ctx,
		Sender:  trailfork.Sender{TraceID: sourceTrace, AgentID: agentID},
		Payload: protocol.ForkRequestPayload{
			Kind:        protocol.SignalTrailFork,
			RequestID:   requestID,
			SourceTrace: sourceTrace,
			Delta:       trailfork.DeltaNone,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func allowForkPublish(t *testing.T, root string) {
	t.Helper()
	writeFile(t, root, ".paseka/bees/builder.yaml", `role: builder
adapter: script
prompt_template: builder.md
command: [echo]
publishes:
  - type: SIGNAL
    kind: trail.fork
`)
	runGit(t, root, "add", ".paseka/bees/builder.yaml")
	runGit(t, root, "commit", "-q", "-m", "grant fork publication")
}

func writeSenderRun(t *testing.T, root, traceID, agentID, bee string) {
	t.Helper()
	dir := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := dir.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := dir.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             bee,
		Adapter:         "script",
		Workspace:       root,
		ColonyRoot:      root,
		TaskID:          "task-1",
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}
