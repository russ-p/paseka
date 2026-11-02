package runtime_test

import (
	"context"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// TestReviewFeedbackVerdictBehaviour pins the reactor's reaction to
// human.feedback on an eligible waiting_review task: negative and the legacy
// absence keep the rework dispatch, while positive and neutral only record the
// feedback.
func TestReviewFeedbackVerdictBehaviour(t *testing.T) {
	tests := []struct {
		name         string
		verdict      string
		wantDispatch bool
	}{
		{name: "negative", verdict: protocol.FeedbackVerdictNegative, wantDispatch: true},
		{name: "positive", verdict: protocol.FeedbackVerdictPositive, wantDispatch: false},
		{name: "neutral", verdict: protocol.FeedbackVerdictNeutral, wantDispatch: false},
		{name: "legacy absent", verdict: "", wantDispatch: true},
		{name: "legacy whitespace", verdict: "   ", wantDispatch: true},
		// A historical value outside the vocabulary was always read as a
		// request to rework, so it keeps that behaviour rather than silently
		// becoming observational.
		{name: "historical unknown", verdict: "blocking", wantDispatch: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ledger := taskledger.NewMemoryLedger()
			seedWaitingReviewTask(t, ledger, "task-1")
			r := newPendingDispatchReactor(t, ledger, fakeReplayer{}, taskDispatchBees())
			probe := &taskProbeAdapter{ledger: ledger, traceID: "trace-1"}
			r.Dispatcher().RegisterAdapter("cursor", probe)

			feedback, err := protocol.NewEvent("trace-1", "console", 0, protocol.EventInsight, protocol.HumanFeedbackPayload{
				Kind:    protocol.InsightHumanFeedback,
				TaskID:  "task-1",
				Message: "review feedback",
				Verdict: tc.verdict,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.ProcessEvent(context.Background(), feedback); err != nil {
				t.Fatal(err)
			}

			if tc.wantDispatch {
				if probe.calls != 1 {
					t.Fatalf("adapter calls = %d, want one rework dispatch", probe.calls)
				}
				if !probe.running[0] {
					t.Fatal("rework dispatch must run the task")
				}
				// The probe completes without a new proposal, so the required
				// gate stops at waiting_review again.
				assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
				return
			}

			if probe.calls != 0 {
				t.Fatalf("adapter calls = %d, want no dispatch for %s feedback", probe.calls, tc.name)
			}
			// Observational feedback records the event but neither reopens the
			// gate nor approves the task.
			assertTaskStatus(t, ledger, "task-1", protocol.TaskStatusWaitingReview)
		})
	}
}
