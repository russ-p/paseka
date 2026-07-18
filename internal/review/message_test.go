package review_test

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/worktree"
)

func TestApproveMessageStashOutcomes(t *testing.T) {
	root := review.ApproveMessage(review.ApproveMessageOptions{
		ProposalWorkspace: protocol.ProposalWorkspaceRoot,
	})
	if root != "Task approved (root proposal — no worktree merge)." {
		t.Fatalf("root message = %q", root)
	}

	clean := review.ApproveMessage(review.ApproveMessageOptions{
		CommitSHA:    "abc",
		StashOutcome: worktree.StashOutcomeNone,
	})
	if clean != "Task approved and worktree merged." {
		t.Fatalf("clean merge message = %q", clean)
	}

	restored := review.ApproveMessage(review.ApproveMessageOptions{
		CommitSHA:    "abc",
		StashOutcome: worktree.StashOutcomeRestored,
	})
	if restored != "Task approved and worktree merged. Local changes were restored." {
		t.Fatalf("restored message = %q", restored)
	}
}
