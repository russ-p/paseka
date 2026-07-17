package review

import (
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// IsRootProposalTask reports whether the task's opening proposal is on colony root (R1 path).
func IsRootProposalTask(task taskledger.TaskSnapshot, bees map[string]colony.Bee) bool {
	switch task.ProposalWorkspace {
	case protocol.ProposalWorkspaceRoot:
		return true
	case protocol.ProposalWorkspaceIsolated:
		return false
	}
	beeName := strings.TrimSpace(task.Bee)
	if beeName == "" {
		beeName = "builder"
	}
	bee, ok := bees[beeName]
	if !ok {
		return false
	}
	ws, ok := bee.ExpectedProposalWorkspace()
	return ok && ws == protocol.ProposalWorkspaceRoot
}

// ShouldMergeOnApprove reports whether approve may merge the trace worktree.
// Root proposals never merge (R1); isolated final merge gates keep existing behavior.
func ShouldMergeOnApprove(task taskledger.TaskSnapshot, bees map[string]colony.Bee) bool {
	if !taskledger.IsFinalReviewTask(task) {
		return false
	}
	return !IsRootProposalTask(task, bees)
}
