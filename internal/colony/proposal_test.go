package colony_test

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestValidateTaskReviewPolicyRejectsFinalOnRootBee(t *testing.T) {
	bees := map[string]colony.Bee{
		"hivewright": {
			Role:     "hivewright",
			Worktree: false,
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
			},
		},
	}
	err := colony.ValidateTaskReviewPolicy(protocol.TaskSpec{
		TaskID: "task-1",
		Bee:    "hivewright",
		Review: protocol.TaskReviewFinal,
	}, bees)
	if err == nil || !strings.Contains(err.Error(), "review:final") {
		t.Fatalf("ValidateTaskReviewPolicy() err = %v, want review:final rejection", err)
	}
}

func TestValidateCodeProposalWorktreeInvariants(t *testing.T) {
	cases := []struct {
		name string
		bee  colony.Bee
		want string
	}{
		{
			name: "root publish on worktree bee",
			bee: colony.Bee{
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			want: "code.proposal.root with worktree: true",
		},
		{
			name: "isolated publish on root bee",
			bee: colony.Bee{
				Role:     "hivewright",
				Worktree: false,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
				},
			},
			want: "isolated code.proposal with worktree: false",
		},
		{
			name: "alias publish on root bee",
			bee: colony.Bee{
				Role:     "hivewright",
				Worktree: false,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
				},
			},
			want: "isolated code.proposal with worktree: false",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.bee.ValidateCodeProposalWorktreeInvariants()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateCodeProposalWorktreeInvariants() err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateCodeProposalSubscriberInvariants(t *testing.T) {
	cases := []struct {
		name string
		bee  colony.Bee
		want string
	}{
		{
			name: "isolated subscribe on root bee",
			bee: colony.Bee{
				Role:     "main-guard",
				Worktree: false,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}, Dispatch: colony.DispatchDirect},
				},
			},
			want: "subscribes to isolated code.proposal with worktree: false",
		},
		{
			name: "root subscribe on worktree bee",
			bee: colony.Bee{
				Role:     "guard",
				Worktree: true,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
				},
			},
			want: "subscribes to code.proposal.root with worktree: true",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.bee.ValidateCodeProposalSubscriberInvariants()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateCodeProposalSubscriberInvariants() err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDiagnoseCodeProposal(t *testing.T) {
	t.Run("errors on publish and subscribe mismatches", func(t *testing.T) {
		bees := map[string]colony.Bee{
			"builder": {
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			"guard": {
				Role:     "guard",
				Worktree: true,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
				},
			},
		}
		diag := colony.DiagnoseCodeProposal(bees)
		if len(diag.Errors) < 2 {
			t.Fatalf("errors = %v, want publish and subscribe mismatch errors", diag.Errors)
		}
	})

	t.Run("warns on bare alias", func(t *testing.T) {
		bees := map[string]colony.Bee{
			"builder": {
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
				},
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}, Dispatch: colony.DispatchDirect},
				},
			},
		}
		diag := colony.DiagnoseCodeProposal(bees)
		if len(diag.Warnings) == 0 || !strings.Contains(diag.Warnings[0], "bare code.proposal alias") {
			t.Fatalf("warnings = %v, want bare alias warning", diag.Warnings)
		}
	})

	t.Run("advises on publisher without subscriber", func(t *testing.T) {
		bees := map[string]colony.Bee{
			"builder": {
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
				},
			},
		}
		diag := colony.DiagnoseCodeProposal(bees)
		if len(diag.Advisories) == 0 || !strings.Contains(diag.Advisories[0], "no matching direct subscriber") {
			t.Fatalf("advisories = %v, want missing subscriber advisory", diag.Advisories)
		}
	})

	t.Run("advises when root subscriber missing verification publishes", func(t *testing.T) {
		bees := map[string]colony.Bee{
			"hivewright": {
				Role:     "hivewright",
				Worktree: false,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			"main-guard": {
				Role:     "main-guard",
				Worktree: false,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
				},
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "INSIGHT", Kind: "review.note"}},
				},
			},
		}
		diag := colony.DiagnoseCodeProposal(bees)
		found := false
		for _, adv := range diag.Advisories {
			if strings.Contains(adv, "missing verification.success or verification.failed") {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("advisories = %v, want missing verification advisory", diag.Advisories)
		}
	})

	t.Run("clean wired colony has no findings", func(t *testing.T) {
		bees := map[string]colony.Bee{
			"builder": {
				Role:     "builder",
				Worktree: true,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
				},
			},
			"guard": {
				Role:     "guard",
				Worktree: true,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}, Dispatch: colony.DispatchDirect},
				},
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}},
					{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}},
				},
			},
			"hivewright": {
				Role:     "hivewright",
				Worktree: false,
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
				},
			},
			"main-guard": {
				Role:     "main-guard",
				Worktree: false,
				Subscribes: []colony.SubscriptionRule{
					{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}, Dispatch: colony.DispatchDirect},
				},
				Publishes: []colony.PublicationRule{
					{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.success"}},
					{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: "verification.failed"}},
				},
			},
		}
		diag := colony.DiagnoseCodeProposal(bees)
		if len(diag.Errors) != 0 || len(diag.Warnings) != 0 || len(diag.Advisories) != 0 {
			t.Fatalf("unexpected findings: errors=%v warnings=%v advisories=%v", diag.Errors, diag.Warnings, diag.Advisories)
		}
	})
}

func TestExpectedProposalWorkspace(t *testing.T) {
	root := colony.Bee{
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
		},
	}
	ws, ok := root.ExpectedProposalWorkspace()
	if !ok || ws != protocol.ProposalWorkspaceRoot {
		t.Fatalf("root bee workspace = %q ok=%v", ws, ok)
	}
}

func TestLoadBeeRejectsProposalMismatches(t *testing.T) {
	root := t.TempDir()
	writeBeeYAML(t, root, "hivewright", `role: hivewright
adapter: cursor
worktree: false
publishes:
  - type: MUTATION
    kind: code.proposal.isolated
`)
	_, _, err := colony.LoadBee(root, "hivewright")
	if err == nil || !strings.Contains(err.Error(), "isolated code.proposal with worktree: false") {
		t.Fatalf("LoadBee() err = %v, want publish mismatch error", err)
	}
}
