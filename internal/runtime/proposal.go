package runtime

import (
	"fmt"
	"path/filepath"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
)

type autoProposalPlan struct {
	kind      protocol.MutationKind
	workspace protocol.ProposalWorkspace
	ok        bool
	warn      string
}

func planAutoProposal(bee colony.Bee) autoProposalPlan {
	isolated := bee.DeclaresCodeProposalIsolated()
	root := bee.DeclaresCodeProposalRoot()

	if bee.Worktree {
		if isolated {
			return autoProposalPlan{
				kind:      protocol.MutationCodeProposalIsolated,
				workspace: protocol.ProposalWorkspaceIsolated,
				ok:        true,
			}
		}
		if root {
			return autoProposalPlan{
				warn: fmt.Sprintf("bee %q: worktree true but publishes code.proposal.root; skipping auto mutation", bee.Role),
			}
		}
		return autoProposalPlan{
			warn: fmt.Sprintf("bee %q: worktree true without isolated code.proposal publish; skipping auto mutation", bee.Role),
		}
	}

	if root {
		return autoProposalPlan{
			kind:      protocol.MutationCodeProposalRoot,
			workspace: protocol.ProposalWorkspaceRoot,
			ok:        true,
		}
	}
	if isolated {
		return autoProposalPlan{
			warn: fmt.Sprintf("bee %q: worktree false but publishes isolated code.proposal; skipping auto mutation", bee.Role),
		}
	}
	return autoProposalPlan{
		warn: fmt.Sprintf("bee %q: no matching code.proposal publish for worktree=false; skipping auto mutation", bee.Role),
	}
}

func shouldAutoPublishProposal(bee colony.Bee) bool {
	if len(bee.Publishes) == 0 {
		return false
	}
	return bee.DeclaresCodeProposalIsolated() || bee.DeclaresCodeProposalRoot()
}

func worktreeRelPath(colonyRoot, traceID string) string {
	abs := filepath.Join(colonyRoot, ".paseka", "worktrees", traceID)
	if rel, err := filepath.Rel(colonyRoot, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

func appendAutoProposalSkipWarning(bee colony.Bee, plan autoProposalPlan, result *adapters.RunResult) {
	if plan.warn == "" {
		return
	}
	runtimeLog.Warn("auto mutation skipped", logging.F("bee", bee.Role), logging.F("reason", plan.warn))
	if result != nil {
		result.Warnings = append(result.Warnings, plan.warn)
	}
}
