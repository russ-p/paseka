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
