package colony

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// CodeProposalDiagnosis groups doctor findings for code proposal wiring.
type CodeProposalDiagnosis struct {
	Errors     []string
	Warnings   []string
	Advisories []string
}

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

// ValidateCodeProposalWorktreeInvariants checks hard worktree ↔ proposal publish kind rules.
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

// ValidateCodeProposalSubscriberInvariants checks hard worktree ↔ proposal subscribe kind rules.
func (b Bee) ValidateCodeProposalSubscriberInvariants() error {
	for _, sub := range b.Subscribes {
		kind := strings.TrimSpace(sub.Kind)
		if kind == "" {
			continue
		}
		if protocol.CodeProposalKindsMatch(kind, string(protocol.MutationCodeProposalIsolated)) && !b.Worktree {
			return fmt.Errorf("colony: bee %q: subscribes to isolated code.proposal with worktree: false", b.Role)
		}
		if protocol.CodeProposalKindsMatch(kind, string(protocol.MutationCodeProposalRoot)) && b.Worktree {
			return fmt.Errorf("colony: bee %q: subscribes to code.proposal.root with worktree: true", b.Role)
		}
	}
	return nil
}

// UsesBareCodeProposalAlias reports whether publishes or subscribes still use bare code.proposal.
func (b Bee) UsesBareCodeProposalAlias() bool {
	alias := string(protocol.MutationCodeProposal)
	for _, pub := range b.Publishes {
		if strings.TrimSpace(pub.Kind) == alias {
			return true
		}
	}
	for _, sub := range b.Subscribes {
		if strings.TrimSpace(sub.Kind) == alias {
			return true
		}
	}
	return false
}

// SubscribesToCodeProposalRoot reports whether a direct subscription matches root proposals.
func (b Bee) SubscribesToCodeProposalRoot() bool {
	for _, sub := range b.Subscribes {
		if sub.ResolvedDispatch() != DispatchDirect {
			continue
		}
		if sub.Matches(protocol.EventMutation, string(protocol.MutationCodeProposalRoot)) {
			return true
		}
	}
	return false
}

// DiagnoseCodeProposal returns doctor findings for code proposal colony wiring.
func DiagnoseCodeProposal(bees map[string]Bee) CodeProposalDiagnosis {
	var diag CodeProposalDiagnosis
	for role, bee := range bees {
		bee.Role = role
		if err := bee.ValidateCodeProposalWorktreeInvariants(); err != nil {
			diag.Errors = append(diag.Errors, err.Error())
		}
		if err := bee.ValidateCodeProposalSubscriberInvariants(); err != nil {
			diag.Errors = append(diag.Errors, err.Error())
		}
		if bee.UsesBareCodeProposalAlias() {
			diag.Warnings = append(diag.Warnings,
				fmt.Sprintf("colony: bee %q: prefers explicit code.proposal.isolated or code.proposal.root over bare code.proposal alias", role))
		}
	}

	for role, bee := range bees {
		if bee.DeclaresCodeProposalIsolated() {
			subs := DirectSubscribers(bees, protocol.EventMutation, string(protocol.MutationCodeProposalIsolated))
			if len(subs) == 0 {
				diag.Advisories = append(diag.Advisories,
					fmt.Sprintf("colony: bee %q: publishes isolated code.proposal with no matching direct subscriber", role))
			}
		}
		if bee.DeclaresCodeProposalRoot() {
			subs := DirectSubscribers(bees, protocol.EventMutation, string(protocol.MutationCodeProposalRoot))
			if len(subs) == 0 {
				diag.Advisories = append(diag.Advisories,
					fmt.Sprintf("colony: bee %q: publishes code.proposal.root with no matching direct subscriber", role))
			}
		}
	}

	for role, bee := range bees {
		if !bee.SubscribesToCodeProposalRoot() {
			continue
		}
		if !bee.ExplicitlyDeclaresPublish(protocol.EventVerification, string(protocol.VerificationSuccess)) ||
			!bee.ExplicitlyDeclaresPublish(protocol.EventVerification, string(protocol.VerificationFailed)) {
			diag.Advisories = append(diag.Advisories,
				fmt.Sprintf("colony: bee %q: subscribes to code.proposal.root but missing verification.success or verification.failed in publishes", role))
		}
	}

	return diag
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
