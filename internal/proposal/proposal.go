// Package proposal owns runtime-generated code proposals: which runs may publish one,
// the diff collected for it, the archived patch record it points at, and the publication
// bookkeeping that says what the broker actually confirmed.
//
// It lives below both runtime paths on purpose. An AFK dispatch and an interactive session
// teardown run the same gate, the same collector, and the same writer, so neither has to
// call through the other to get there.
package proposal

import (
	"fmt"
	"path/filepath"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
)

// Plan is the answer to "may this bee auto-publish a proposal, and as which kind and
// workspace?". It is evaluated from the bee's own declarations plus its worktree setting.
type Plan struct {
	Kind      protocol.MutationKind
	Workspace protocol.ProposalWorkspace
	OK        bool
	Warn      string
}

// PlanAuto resolves the auto-proposal plan for a bee configuration.
func PlanAuto(bee colony.Bee) Plan {
	isolated := bee.DeclaresCodeProposalIsolated()
	root := bee.DeclaresCodeProposalRoot()

	if bee.Worktree {
		if isolated {
			return Plan{
				Kind:      protocol.MutationCodeProposalIsolated,
				Workspace: protocol.ProposalWorkspaceIsolated,
				OK:        true,
			}
		}
		if root {
			return Plan{
				Warn: fmt.Sprintf("bee %q: worktree true but publishes code.proposal.root; skipping auto mutation", bee.Role),
			}
		}
		return Plan{
			Warn: fmt.Sprintf("bee %q: worktree true without isolated code.proposal publish; skipping auto mutation", bee.Role),
		}
	}

	if root {
		return Plan{
			Kind:      protocol.MutationCodeProposalRoot,
			Workspace: protocol.ProposalWorkspaceRoot,
			OK:        true,
		}
	}
	if isolated {
		return Plan{
			Warn: fmt.Sprintf("bee %q: worktree false but publishes isolated code.proposal; skipping auto mutation", bee.Role),
		}
	}
	return Plan{
		Warn: fmt.Sprintf("bee %q: no matching code.proposal publish for worktree=false; skipping auto mutation", bee.Role),
	}
}

// ShouldAutoPublish reports whether a bee declares any code-proposal publication at all.
// Non-mutating bees leave no proposal records, so this fails closed on an empty publishes list.
func ShouldAutoPublish(bee colony.Bee) bool {
	if len(bee.Publishes) == 0 {
		return false
	}
	return bee.DeclaresCodeProposalIsolated() || bee.DeclaresCodeProposalRoot()
}

// WorktreeRelPath returns the colony-root-relative isolated worktree path for a trail.
func WorktreeRelPath(colonyRoot, traceID string) string {
	abs := filepath.Join(colonyRoot, ".paseka", "worktrees", traceID)
	if rel, err := filepath.Rel(colonyRoot, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}
