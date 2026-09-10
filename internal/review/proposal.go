package review

import (
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// IsRootProposalTask reports whether the task's opening proposal is on colony root (R1 path).
func IsRootProposalTask(task taskledger.TaskSnapshot, bees map[string]colony.Bee, defaults colony.Defaults) bool {
	switch task.ProposalWorkspace {
	case protocol.ProposalWorkspaceRoot:
		return true
	case protocol.ProposalWorkspaceIsolated:
		return false
	}
	beeName := colony.EffectiveTaskBee(task.Bee, defaults)
	bee, ok := bees[beeName]
	if !ok {
		return false
	}
	ws, ok := bee.ExpectedProposalWorkspace()
	return ok && ws == protocol.ProposalWorkspaceRoot
}

// ShouldMergeOnApprove reports whether approve may merge the trace worktree.
// Root proposals never merge (R1). pull_request delivery publishes instead of merging.
func ShouldMergeOnApprove(task taskledger.TaskSnapshot, bees map[string]colony.Bee, defaults colony.Defaults) bool {
	if !taskledger.IsFinalReviewTask(task) {
		return false
	}
	if IsRootProposalTask(task, bees, defaults) {
		return false
	}
	return defaults.ResolvedDelivery() != colony.DeliveryPullRequest
}

// ShouldPublishOnApprove reports whether approve should push the worktree head and upsert a PR.
func ShouldPublishOnApprove(task taskledger.TaskSnapshot, bees map[string]colony.Bee, defaults colony.Defaults) bool {
	if !taskledger.IsFinalReviewTask(task) {
		return false
	}
	if IsRootProposalTask(task, bees, defaults) {
		return false
	}
	return defaults.ResolvedDelivery() == colony.DeliveryPullRequest
}
