package review

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// DiscardSummary is the terminal note when the operator gives no reason.
const DiscardSummary = "discarded by human"

// DiscardInput describes a human discarding a review-gated task: close its
// result without merging and without asking the bee for rework.
type DiscardInput struct {
	TraceID string
	TaskID  string
	Reason  string
	AgentID string
}

// Discard closes a review-gated task in a terminal, non-completed state.
// It merges no worktree, publishes no pull request, emits no human.feedback,
// and plans no rework task — the operator is deliberately not doing this.
// The worktree is left in place so a discarded result can be revived later.
func Discard(ctx context.Context, pub bus.Publisher, ledger taskledger.Ledger, in DiscardInput) error {
	if err := EnsureDiscardable(ledger, in.TraceID, in.TaskID); err != nil {
		return err
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		reason = DiscardSummary
	}
	agentID := in.AgentID
	if agentID == "" {
		agentID = "human"
	}
	// task.status (not task.completed): cancelled is terminal but must not promote
	// the next planned task, and it carries the human summary onto the ledger row.
	ev, err := protocol.NewEvent(in.TraceID, agentID, 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind:    protocol.TaskEventStatus,
		TaskID:  in.TaskID,
		Status:  protocol.TaskStatusCancelled,
		Summary: reason,
	})
	if err != nil {
		return err
	}
	return WriteEvent(ctx, pub, ledger, ev, WriteOptions{})
}

// DiscardResponseMessage is the user-facing note after a discard.
func DiscardResponseMessage() string {
	return "Discarded — closed without merging and without rework."
}
