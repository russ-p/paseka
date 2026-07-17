package runtime

import (
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestRunOpenedCodeProposalExplicitPublishOnly(t *testing.T) {
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{
		"scout": {Role: "scout"},
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
			},
		},
	})
	result := &adapters.RunResult{
		Artifacts: []adapters.Artifact{{Kind: "diff", Content: "+line"}},
	}
	if runOpenedCodeProposal(reg, "scout", result) {
		t.Fatal("scout with empty publishes must not open code.proposal gate")
	}
	if !runOpenedCodeProposal(reg, "builder", result) {
		t.Fatal("builder with explicit code.proposal publish and diff should open gate")
	}
}

func TestRunOpenedCodeProposalFromEmittedEvent(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposal,
		Diff: "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{"builder": {Role: "builder"}})
	result := &adapters.RunResult{Events: []protocol.Event{ev}}
	if !runOpenedCodeProposal(reg, "builder", result) {
		t.Fatal("emitted code.proposal should open gate even without diff artifact")
	}
}

func TestShouldDeferAFKCompletion(t *testing.T) {
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
			},
		},
		"receiver": {
			Role: "receiver",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: string(protocol.TaskEventCompleted)}},
			},
		},
	})
	result := &adapters.RunResult{
		Artifacts: []adapters.Artifact{{Kind: "diff", Content: "+line"}},
	}
	if !shouldDeferAFKCompletion(reg, "builder", result) {
		t.Fatal("expected defer when receiver declares task.completed and builder opened proposal")
	}
	if shouldDeferAFKCompletion(reg, "scout", result) {
		t.Fatal("scout must not defer without explicit code.proposal publish on dispatched bee")
	}
}

func TestRunOpenedCodeProposalIsolatedExplicitKind(t *testing.T) {
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{
		"builder": {
			Role: "builder",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
			},
		},
	})
	result := &adapters.RunResult{
		Artifacts: []adapters.Artifact{{Kind: "diff", Content: "+line"}},
	}
	if !runOpenedCodeProposal(reg, "builder", result) {
		t.Fatal("builder with explicit isolated publish and diff should open gate")
	}
}

func TestRunOpenedCodeProposalRootDoesNotOpenGate(t *testing.T) {
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{
		"hivewright": {
			Role: "hivewright",
			Publishes: []colony.PublicationRule{
				{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
			},
		},
	})
	result := &adapters.RunResult{
		Artifacts: []adapters.Artifact{{Kind: "diff", Content: "+line"}},
	}
	if runOpenedCodeProposal(reg, "hivewright", result) {
		t.Fatal("root proposal publish must not open AFK merge/commit-gate")
	}
}

func TestRunOpenedCodeProposalRootEmittedEvent(t *testing.T) {
	ev, err := protocol.NewEvent("trace-1", "hivewright-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind: protocol.MutationCodeProposalRoot,
		Diff: "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	reg := NewBeeRegistryFromBees(map[string]colony.Bee{"hivewright": {Role: "hivewright"}})
	result := &adapters.RunResult{Events: []protocol.Event{ev}}
	if runOpenedCodeProposal(reg, "hivewright", result) {
		t.Fatal("emitted code.proposal.root must not open gate")
	}
}

func TestShouldDeferAFKCompletionMatrix(t *testing.T) {
	receiver := colony.Bee{
		Role: "receiver",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "VERIFICATION", Kind: string(protocol.TaskEventCompleted)}},
		},
	}
	builderIsolated := colony.Bee{
		Role: "builder",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.isolated"}},
		},
	}
	hivewrightRoot := colony.Bee{
		Role: "hivewright",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal.root"}},
		},
	}
	scout := colony.Bee{Role: "scout"}
	diffResult := &adapters.RunResult{
		Artifacts: []adapters.Artifact{{Kind: "diff", Content: "+line"}},
	}

	tests := []struct {
		name    string
		bees    map[string]colony.Bee
		role    string
		result  *adapters.RunResult
		deferAF bool
	}{
		{
			name:    "isolated alias with receiver defers",
			bees:    map[string]colony.Bee{"builder": builderBeeAlias(), "receiver": receiver},
			role:    "builder",
			result:  diffResult,
			deferAF: true,
		},
		{
			name:    "isolated explicit with receiver defers",
			bees:    map[string]colony.Bee{"builder": builderIsolated, "receiver": receiver},
			role:    "builder",
			result:  diffResult,
			deferAF: true,
		},
		{
			name:    "root with receiver does not defer",
			bees:    map[string]colony.Bee{"hivewright": hivewrightRoot, "receiver": receiver},
			role:    "hivewright",
			result:  diffResult,
			deferAF: false,
		},
		{
			name:    "no proposal publish does not defer",
			bees:    map[string]colony.Bee{"scout": scout, "receiver": receiver},
			role:    "scout",
			result:  diffResult,
			deferAF: false,
		},
		{
			name:    "isolated without receiver does not defer",
			bees:    map[string]colony.Bee{"builder": builderIsolated},
			role:    "builder",
			result:  diffResult,
			deferAF: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewBeeRegistryFromBees(tc.bees)
			got := shouldDeferAFKCompletion(reg, tc.role, tc.result)
			if got != tc.deferAF {
				t.Fatalf("shouldDeferAFKCompletion() = %v, want %v", got, tc.deferAF)
			}
		})
	}
}

func builderBeeAlias() colony.Bee {
	return colony.Bee{
		Role: "builder",
		Publishes: []colony.PublicationRule{
			{EventRule: colony.EventRule{Type: "MUTATION", Kind: "code.proposal"}},
		},
	}
}
