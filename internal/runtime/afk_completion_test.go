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
