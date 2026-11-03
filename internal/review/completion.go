package review

import (
	"errors"
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
)

// ensureWorkCompletionEvidence captures the work-completion checkpoint before a
// delivery side effect is allowed to remove its source worktree.
//
// Approval and reconcile are fast paths: a human can approve the final gate the
// instant the last task completes, before the reactor reaches its own completion
// boundary. A local merge deletes the trace worktree and a merge report from the
// forge removes it on reconcile, so the evidence must be durable first. The
// deterministic completion identity makes this capture converge with the
// reactor's: it fills only what is missing and never rewrites an announced
// snapshot.
//
// A trail with no retained run evidence has nothing to certify and the delivery
// proceeds unchanged. A capture that fails for any other reason is a visible
// finalization error: the caller must not deliver, and no success milestone may
// be announced for evidence that was never written.
func ensureWorkCompletionEvidence(colonyCtx colony.Context, trace taskledger.TraceSnapshot) error {
	if colonyCtx.ColonyRoot == "" || trace.TraceID == "" {
		return nil
	}
	if !taskledger.EvaluateWorkCompletion(trace).Complete {
		// Work is not at its completion boundary; acceptance of one gate is not
		// a trail completion.
		return nil
	}
	if trace.WorkRevision < 1 {
		// A legacy trail has no accepted-work revision to identify a completion
		// by. Delivery proceeds exactly as before; no revision is invented.
		return nil
	}
	if _, err := runs.CaptureWorkCompletion(colonyCtx.ColonyRoot, trace, time.Now().UTC()); err != nil {
		if errors.Is(err, runs.ErrTrailNotFound) {
			return nil
		}
		return fmt.Errorf("review: capture work completion before delivery: %w", err)
	}
	return nil
}
