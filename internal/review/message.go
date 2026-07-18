package review

import (
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/worktree"
)

// ApproveMessageOptions configures the human-readable approve success message.
type ApproveMessageOptions struct {
	ProposalWorkspace protocol.ProposalWorkspace
	CommitSHA         string
	StashOutcome      worktree.StashOutcome
}

// ApproveMessage returns a user-facing message after a successful approve.
func ApproveMessage(opts ApproveMessageOptions) string {
	if opts.ProposalWorkspace == protocol.ProposalWorkspaceRoot {
		return "Task approved (root proposal — no worktree merge)."
	}
	if opts.CommitSHA == "" {
		return "Task approved."
	}
	switch opts.StashOutcome {
	case worktree.StashOutcomeRestored:
		return "Task approved and worktree merged. Local changes were restored."
	case worktree.StashOutcomeRestoreConflicted:
		return "Task approved and worktree merged. Warning: restoring local changes conflicted — resolve stash/working-tree conflicts manually."
	default:
		return "Task approved and worktree merged."
	}
}

// CLIApproveMessage returns the CLI variant of the approve success message.
func CLIApproveMessage(opts ApproveMessageOptions) string {
	if opts.ProposalWorkspace == protocol.ProposalWorkspaceRoot {
		return "Approved (root proposal — no worktree merge)."
	}
	if opts.CommitSHA == "" {
		return "Approved."
	}
	switch opts.StashOutcome {
	case worktree.StashOutcomeRestored:
		return "Approved and worktree merged. Local changes were restored."
	case worktree.StashOutcomeRestoreConflicted:
		return "Approved and worktree merged. Warning: restoring local changes conflicted — resolve stash/working-tree conflicts manually."
	default:
		return "Approved and worktree merged."
	}
}
