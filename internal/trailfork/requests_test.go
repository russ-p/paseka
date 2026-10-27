package trailfork_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
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

func TestApproveUsesRecordedAttemptAfterSourceRetry(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-retried", "trace-source", "agent-sender")
	if req.Evidence.SourceAttempt == "" || req.Evidence.SourceRun != "" {
		t.Fatalf("recorded evidence = %+v", req.Evidence)
	}
	recorded := req.Evidence.SourceAttempt

	// Retry the source task after the request recorded its decision. Two
	// attempts now exist, so resolving "the latest attempt" would be ambiguous
	// and must not invalidate the request.
	if err := trailconfig.CloseTaskAttempts(root, "trace-source", "task-1", "retry"); err != nil {
		t.Fatal(err)
	}
	retryInput := trailconfig.NewTaskInputEvidence("task-1", "retried task", "retried body", "builder", "", "feature", string(protocol.TaskReviewNone))
	retried, err := trailconfig.EnsureTaskAttempt(ctx, "trace-source", "task-1", "evt-retry", retryInput)
	if err != nil {
		t.Fatal(err)
	}
	if retried.AttemptID == recorded {
		t.Fatalf("source retry reused attempt %s", recorded)
	}

	result, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Actor:     "alice",
		Publisher: &capturePublisher{},
		Ledger:    taskledger.NewMemoryLedger(),
	})
	if err != nil {
		t.Fatalf("approval after source retry: %v", err)
	}
	if result.Plan.SourceAttempt != recorded {
		t.Fatalf("fork inherited attempt %s, want recorded %s", result.Plan.SourceAttempt, recorded)
	}
	if result.Plan.SourceRun != "" {
		t.Fatalf("recorded source run was rewritten to %q", result.Plan.SourceRun)
	}
	if result.Plan.Task.Title != "implement task-1" || result.Plan.Task.Body != "body for task-1" {
		t.Fatalf("fork carried retry evidence: %+v", result.Plan.Task)
	}
	if result.Plan.Task.SourceTaskDigest != req.Evidence.Task.SourceTaskDigest {
		t.Fatalf("fork task digest = %q, want recorded %q", result.Plan.Task.SourceTaskDigest, req.Evidence.Task.SourceTaskDigest)
	}
}

func TestApproveRefusesRewrittenSourceAttempt(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-rewritten-attempt", "trace-source", "agent-sender")

	// A missing recorded attempt must not silently fall back to the recorded
	// run or to the only remaining attempt.
	req.Evidence.SourceAttempt = "attempt-missing"
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trailfork.RequestPath(root, req.Evidence.SourceTrace, req.RequestID), data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: req.RequestID,
		Actor:     "alice",
		Publisher: &capturePublisher{},
		Ledger:    taskledger.NewMemoryLedger(),
	})
	if err == nil || !strings.Contains(err.Error(), "attempt-missing") || !strings.Contains(err.Error(), "is not recorded") {
		t.Fatalf("approve rewritten attempt error = %v", err)
	}
	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateRefused {
		t.Fatalf("state = %s, want refused", stored.State)
	}
	if stored.TraceID != "" {
		t.Fatalf("refused approval minted a fork trace %s", stored.TraceID)
	}
}

func TestSourceRunMustAgreeWithRecordedAttempt(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	first, ok, err := trailconfig.LatestTaskAttempt(root, "trace-source", "task-1")
	if err != nil || !ok {
		t.Fatalf("first attempt: %v %v", ok, err)
	}
	if err := trailconfig.CloseTaskAttempts(root, "trace-source", "task-1", "retry"); err != nil {
		t.Fatal(err)
	}
	second := addAttempt(t, ctx, "trace-source", "task-1", "evt-2")
	if err := trailconfig.RecordAttemptAgent(root, "trace-source", second.AttemptID, "agent-second"); err != nil {
		t.Fatal(err)
	}

	_, err = trailfork.Preview(trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		SourceRun:     "agent-second",
		SourceAttempt: first.AttemptID,
	})
	if err == nil || !strings.Contains(err.Error(), "recorded source attempt") {
		t.Fatalf("disagreeing run/attempt error = %v", err)
	}

	matched, err := trailfork.Preview(trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		SourceRun:     "agent-second",
		SourceAttempt: second.AttemptID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if matched.Plan.SourceAttempt != second.AttemptID || matched.Plan.SourceRun != "agent-second" {
		t.Fatalf("agreeing selection = %+v", matched.Plan)
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

func TestListOpenIncludesCreatingRequests(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	pending := submitRequest(t, ctx, "req-open-pending", "trace-source", "agent-sender")
	creating := submitRequest(t, ctx, "req-open-creating", "trace-source", "agent-sender")

	if _, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: creating.RequestID,
		Actor:     "alice",
		Publisher: &dropProbePublisher{failPublish: true},
		Ledger:    taskledger.NewMemoryLedger(),
	}); err == nil {
		t.Fatal("expected the initial publish to fail")
	}
	stored, err := trailfork.LoadRequest(root, creating.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateCreating || stored.LastError == "" {
		t.Fatalf("interrupted request = %+v", stored)
	}

	open, err := trailfork.ListOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for i, req := range open {
		ids[req.RequestID] = true
		if i > 0 && req.CreatedAt.Before(open[i-1].CreatedAt) {
			t.Fatalf("ListOpen is not oldest first: %+v", open)
		}
	}
	if len(open) != 2 || !ids[pending.RequestID] || !ids[creating.RequestID] {
		t.Fatalf("open requests = %+v", open)
	}

	pendingOnly, err := trailfork.ListPending(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingOnly) != 1 || pendingOnly[0].RequestID != pending.RequestID {
		t.Fatalf("ListPending = %+v", pendingOnly)
	}
}

// blockingPublisher blocks the first publish until the test releases it,
// keeping an approval inside the creation path while a duplicate arrives.
type blockingPublisher struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once

	mu     sync.Mutex
	events []protocol.Event
}

func newBlockingPublisher() *blockingPublisher {
	return &blockingPublisher{started: make(chan struct{}), release: make(chan struct{})}
}

func (p *blockingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.once.Do(func() {
		close(p.started)
		<-p.release
	})
	p.mu.Lock()
	p.events = append(p.events, ev)
	p.mu.Unlock()
	return nil
}

func TestDuplicateApprovalDuringSlowCreationReturnsRecordedFork(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-duplicate-approval", "trace-source", "agent-sender")

	ledger := taskledger.NewMemoryLedger()
	pub := newBlockingPublisher()
	type outcome struct {
		result trailfork.Result
		err    error
	}
	decide := func() outcome {
		result, err := trailfork.ApproveRequest(context.Background(), trailfork.DecisionOptions{
			Context:   ctx,
			RequestID: req.RequestID,
			Actor:     "alice",
			Publisher: pub,
			Ledger:    ledger,
		})
		return outcome{result: result, err: err}
	}
	wait := func(name string, ch <-chan outcome) outcome {
		t.Helper()
		select {
		case out := <-ch:
			return out
		case <-time.After(30 * time.Second):
			t.Fatalf("%s approval did not return", name)
			return outcome{}
		}
	}

	first := make(chan outcome, 1)
	go func() { first <- decide() }()
	select {
	case <-pub.started:
	case <-time.After(30 * time.Second):
		t.Fatal("first approval never reached publication")
	}

	second := make(chan outcome, 1)
	go func() { second <- decide() }()
	close(pub.release)

	firstOut := wait("first", first)
	secondOut := wait("second", second)
	if firstOut.err != nil {
		t.Fatalf("first approval: %v", firstOut.err)
	}
	if secondOut.err != nil {
		t.Fatalf("second approval: %v", secondOut.err)
	}
	if firstOut.result.Plan.TraceID != secondOut.result.Plan.TraceID {
		t.Fatalf("duplicate approval forked twice: %s -> %s",
			firstOut.result.Plan.TraceID, secondOut.result.Plan.TraceID)
	}

	requests, err := trailfork.ListRequests(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].TraceID != firstOut.result.Plan.TraceID {
		t.Fatalf("requests = %+v", requests)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	forks := 0
	for _, entry := range entries {
		if entry.Name() == firstOut.result.Plan.TraceID {
			forks++
		}
	}
	if forks != 1 {
		t.Fatalf("fork trace dirs = %d, want 1", forks)
	}
}

func TestCreateFailureNamesTheRecordedRequest(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	_, err := trailfork.Create(context.Background(), &dropProbePublisher{failPublish: true}, taskledger.NewMemoryLedger(), trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
	})
	if err == nil {
		t.Fatal("expected the initial publish to fail")
	}
	requests, listErr := trailfork.ListRequests(root)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %+v", requests)
	}
	requestID := requests[0].RequestID
	if requestID == "" {
		t.Fatal("failed creation recorded no request identity")
	}
	if !strings.Contains(err.Error(), requestID) {
		t.Fatalf("error %q does not name request %s", err, requestID)
	}
	if _, err := trailfork.LoadRequest(root, requestID); err != nil {
		t.Fatalf("recorded request %s is not loadable: %v", requestID, err)
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

func TestRecoverCreatingResumesInterruptedApprovalWithoutDuplication(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")
	req := submitRequest(t, ctx, "req-recover", "trace-source", "agent-sender")

	// A fresh process reads the durable pending request without creating
	// anything.
	pending, err := trailfork.ListPending(root)
	if err != nil || len(pending) != 1 || pending[0].RequestID != req.RequestID {
		t.Fatalf("pending after restart = %+v %v", pending, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", req.RequestID)); !os.IsNotExist(err) {
		t.Fatalf("pending request created a trace dir: %v", err)
	}

	// Simulate a crash after approval minted identities and recorded the
	// creating state but before the on-disk creation finished.
	req.State = trailfork.StateCreating
	req.TraceID = "trace-recovered"
	req.TaskID = "task-recovered"
	req.ForkID = "fork-recovered"
	req.DecidedBy = "alice"
	req.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(root, req.Evidence.SourceTrace, req.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ledger := taskledger.NewMemoryLedger()
	pub := &capturePublisher{}
	recovered, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Plan.TraceID != "trace-recovered" || recovered[0].Plan.Task.TaskID != "task-recovered" {
		t.Fatalf("recovered = %+v", recovered)
	}

	// A second restart finds nothing to recover and does not create a duplicate.
	again, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("second recovery = %+v %v", again, err)
	}
	tasksSnapshot, err := runs.LoadTraceTasksFromFS(root, "trace-recovered")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasksSnapshot.Tasks) != 1 {
		t.Fatalf("recovered fork tasks = %+v", tasksSnapshot.Tasks)
	}
	snap, err := ledger.Snapshot("trace-recovered")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 8 || snap.EnergyRemaining != 8 {
		t.Fatalf("recovered fork honey = budget %d remaining %d", snap.EnergyBudget, snap.EnergyRemaining)
	}

	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateApproved || stored.TraceID != "trace-recovered" || stored.Lineage == nil {
		t.Fatalf("stored request = %+v", stored)
	}
}

func TestCancelCreatingRequestRemovesPartialFork(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	opts := trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		RequestID:     "req-cancel-creating",
	}
	if _, err := trailfork.Create(context.Background(), &dropProbePublisher{failPublish: true}, taskledger.NewMemoryLedger(), opts); err == nil {
		t.Fatal("expected the initial publish to fail")
	}
	req, err := trailfork.LoadRequest(root, opts.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if req.State != trailfork.StateCreating || req.TraceID == "" {
		t.Fatalf("request = %+v", req)
	}
	lineage, err := trailfork.LoadLineage(root, req.TraceID)
	if err != nil {
		t.Fatal(err)
	}

	cancelled, err := trailfork.CancelRequest(trailfork.DecisionOptions{
		Context:   ctx,
		RequestID: opts.RequestID,
		Actor:     "operator",
		Reason:    "abandon failed creation",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != trailfork.StateCancelled {
		t.Fatalf("state = %q", cancelled.State)
	}
	if _, err := trailfork.LoadLineage(root, req.TraceID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lineage survived cancellation: %v", err)
	}
	if _, err := trailconfig.Load(root, req.TraceID); !errors.Is(err, trailconfig.ErrNoBinding) {
		t.Fatalf("binding survived cancellation: %v", err)
	}
	if _, err := os.Stat(lineage.Workspace); !os.IsNotExist(err) {
		t.Fatalf("worktree survived cancellation: %v", err)
	}
	if gitroot.LocalBranchExists(root, lineage.Branch) {
		t.Fatalf("branch %q survived cancellation", lineage.Branch)
	}
}

func TestRecoverCreatingSkipsRecordedFailure(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	opts := trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		RequestID:     "req-failed-creation",
	}
	ledger := taskledger.NewMemoryLedger()
	if _, err := trailfork.Create(context.Background(), &dropProbePublisher{failPublish: true}, ledger, opts); err == nil {
		t.Fatal("expected the initial publish to fail")
	}
	recovered, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: &capturePublisher{},
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovered failed creation: %+v", recovered)
	}
	req, err := trailfork.LoadRequest(root, opts.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if req.State != trailfork.StateCreating || req.LastError == "" {
		t.Fatalf("request = %+v", req)
	}
}

func TestRecoverCreatingFinishesAPlanWhoseReadyCrashed(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	opts := trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		RequestID:     "req-half",
		ForkTraceID:   "trace-half",
		ForkTaskID:    "task-half",
	}
	plan, err := trailfork.Preview(opts)
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	// A crash between task.plan and task.ready leaves the ledger holding the
	// carried task as planned while the request stays creating.
	if _, err := trailfork.Create(context.Background(), &readyCrashPublisher{ledger: ledger}, ledger, opts); err == nil {
		t.Fatal("expected the task.ready publish to fail")
	}
	snap, err := ledger.Snapshot(plan.Plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks[plan.Plan.Task.TaskID].Status; got != protocol.TaskStatusPlanned {
		t.Fatalf("task status before recovery = %q", got)
	}
	lineage, err := trailfork.LoadLineage(root, plan.Plan.TraceID)
	if err != nil {
		t.Fatalf("partial creation lost its lineage: %v", err)
	}

	req := trailfork.Request{
		Version:       1,
		RequestID:     "req-half",
		State:         trailfork.StateCreating,
		Origin:        "cli",
		Evidence:      lineage,
		ContentDigest: "half",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		TraceID:       plan.Plan.TraceID,
		TaskID:        plan.Plan.Task.TaskID,
		ForkID:        plan.Plan.ForkID,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(root, lineage.SourceTrace, req.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	pub := &capturePublisher{}
	recovered, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Plan.TraceID != plan.Plan.TraceID {
		t.Fatalf("recovered = %+v", recovered)
	}
	kinds := map[string]int{}
	for _, ev := range pub.events {
		kinds[protocol.PayloadKind(ev.Payload)]++
	}
	if kinds[string(protocol.TaskEventPlan)] != 0 || kinds[string(protocol.TaskEventReady)] != 1 {
		t.Fatalf("recovered publication = %+v", kinds)
	}

	// The published ready promotes the carried task out of planned.
	for _, ev := range pub.events {
		if _, err := ledger.Apply(ev); err != nil {
			t.Fatal(err)
		}
	}
	snap, err = ledger.Snapshot(plan.Plan.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks[plan.Plan.Task.TaskID].Status; got != protocol.TaskStatusReady {
		t.Fatalf("task status after recovery = %q", got)
	}

	// A later restart finds nothing to publish again.
	again, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("second recovery = %+v %v", again, err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("recovery republished events: %d", len(pub.events))
	}
}

func TestRecoverCreatingUsesLineageAfterSourcePurge(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	writeSource(t, ctx, "trace-source", "task-1", nil)

	ledger := taskledger.NewMemoryLedger()
	opts := trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		RequestID:     "req-source-purged",
	}
	if _, err := trailfork.Create(context.Background(), &readyCrashPublisher{ledger: ledger}, ledger, opts); err == nil {
		t.Fatal("expected the task.ready publish to fail")
	}
	req, err := trailfork.LoadRequest(root, opts.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if req.State != trailfork.StateCreating || req.TraceID == "" {
		t.Fatalf("request = %+v", req)
	}
	// A crash leaves no recorded failure; clear the error a completed publish
	// attempt would have written so recovery sees an interrupted creation.
	req.LastError = ""
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trailfork.RequestPath(root, req.Evidence.SourceTrace, req.RequestID), data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(root, ".paseka", "runs", "trace-source", "config")); err != nil {
		t.Fatal(err)
	}
	pub := &capturePublisher{}
	recovered, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Plan.TraceID != req.TraceID {
		t.Fatalf("recovered = %+v", recovered)
	}
	stored, err := trailfork.LoadRequest(root, opts.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateApproved || stored.Lineage == nil {
		t.Fatalf("stored request = %+v", stored)
	}
}

// readyCrashPublisher applies everything it publishes to the ledger and fails
// the first task.ready, reproducing a crash between task.plan and task.ready.
type readyCrashPublisher struct {
	ledger taskledger.Ledger
}

func (p *readyCrashPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	if protocol.PayloadKind(ev.Payload) == string(protocol.TaskEventReady) {
		return errors.New("bus unavailable before task.ready")
	}
	_, err := p.ledger.Apply(ev)
	return err
}

func TestRecoverCreatingFinishesAnUnpublishedInitialPlan(t *testing.T) {
	root, _ := setupGitRepo(t)
	ctx := testContext(root)
	allowForkPublish(t, root)
	writeSource(t, ctx, "trace-source", "task-1", nil)
	writeSenderRun(t, root, "trace-source", "agent-sender", "builder")

	opts := trailfork.Options{
		Context:       ctx,
		SourceTraceID: "trace-source",
		RequestID:     "req-partial",
		ForkTraceID:   "trace-partial",
		ForkTaskID:    "task-partial",
	}
	plan, err := trailfork.Preview(opts)
	if err != nil {
		t.Fatal(err)
	}
	ledger := taskledger.NewMemoryLedger()
	if _, err := trailfork.Create(context.Background(), &dropProbePublisher{failPublish: true}, ledger, opts); err == nil {
		t.Fatal("expected the initial publish to fail")
	}
	lineage, err := trailfork.LoadLineage(root, "trace-partial")
	if err != nil {
		t.Fatalf("partial creation lost its lineage: %v", err)
	}

	// An approval that crashed after writing the binding and lineage but before
	// its plan reached the ledger is recorded as creating.
	req := trailfork.Request{
		Version:       1,
		RequestID:     "req-partial",
		State:         trailfork.StateCreating,
		Origin:        "cli",
		Evidence:      lineage,
		ContentDigest: "partial",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		TraceID:       plan.Plan.TraceID,
		TaskID:        plan.Plan.Task.TaskID,
		ForkID:        plan.Plan.ForkID,
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.RequestPath(root, lineage.SourceTrace, req.RequestID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	pub := &capturePublisher{}
	recovered, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].Plan.TraceID != "trace-partial" {
		t.Fatalf("recovered = %+v", recovered)
	}
	kinds := map[string]int{}
	for _, ev := range pub.events {
		kinds[protocol.PayloadKind(ev.Payload)]++
	}
	if kinds[string(protocol.TaskEventPlan)] != 1 || kinds[string(protocol.TaskEventReady)] != 1 {
		t.Fatalf("recovered publication = %+v", kinds)
	}

	// The completed recovery leaves nothing to publish again.
	again, err := trailfork.RecoverCreating(context.Background(), trailfork.DecisionOptions{
		Context:   ctx,
		Publisher: pub,
		Ledger:    ledger,
	})
	if err != nil || len(again) != 0 {
		t.Fatalf("second recovery = %+v %v", again, err)
	}
	if len(pub.events) != 2 {
		t.Fatalf("recovery republished events: %d", len(pub.events))
	}
	stored, err := trailfork.LoadRequest(root, req.RequestID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != trailfork.StateApproved {
		t.Fatalf("stored state = %q", stored.State)
	}
}
