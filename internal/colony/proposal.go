package colony

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// ExpectedProposalWorkspace reports the sole declared code.proposal publish workspace for a bee.
func (b Bee) ExpectedProposalWorkspace() (protocol.ProposalWorkspace, bool) {
	root := b.DeclaresCodeProposalRoot()
	isolated := b.DeclaresCodeProposalIsolated()
	if root && !isolated {
		return protocol.ProposalWorkspaceRoot, true
	}
	if isolated && !root {
		return protocol.ProposalWorkspaceIsolated, true
	}
	return "", false
}

// ValidateCodeProposalWorktreeInvariants checks hard worktree ↔ proposal kind rules.
func (b Bee) ValidateCodeProposalWorktreeInvariants() error {
	isolated := b.DeclaresCodeProposalIsolated()
	root := b.DeclaresCodeProposalRoot()
	if b.Worktree {
		if root && !isolated {
			return fmt.Errorf("colony: bee %q: publishes code.proposal.root with worktree: true", b.Role)
		}
	} else {
		if isolated && !root {
			return fmt.Errorf("colony: bee %q: publishes isolated code.proposal with worktree: false", b.Role)
		}
	}
	return nil
}

// DiagnoseCodeProposalBees returns worktree ↔ proposal kind doctor findings for all bees.
func DiagnoseCodeProposalBees(bees map[string]Bee) []string {
	var issues []string
	for role, bee := range bees {
		bee.Role = role
		if err := bee.ValidateCodeProposalWorktreeInvariants(); err != nil {
			issues = append(issues, err.Error())
		}
	}
	return issues
}

// ValidateTaskReviewPolicy rejects review: final on tasks whose bee publishes root proposals.
func ValidateTaskReviewPolicy(spec protocol.TaskSpec, bees map[string]Bee) error {
	if protocol.NormalizeTaskReviewPolicy(spec.Review) != protocol.TaskReviewFinal {
		return nil
	}
	beeName := strings.TrimSpace(spec.Bee)
	if beeName == "" {
		beeName = "builder"
	}
	bee, ok := bees[beeName]
	if !ok {
		return nil
	}
	if ws, ok := bee.ExpectedProposalWorkspace(); ok && ws == protocol.ProposalWorkspaceRoot {
		return fmt.Errorf("colony: task %q: review:final is only valid for isolated proposals (bee %q publishes code.proposal.root)", spec.TaskID, beeName)
	}
	return nil
}
