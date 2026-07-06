package runtime

import (
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestEnforceCompletionContractMarksFailed(t *testing.T) {
	d := NewDispatcher()
	bee := colony.Bee{
		Role: "guard",
		CompletionContract: colony.CompletionContract{
			Required: []colony.CompletionRule{{
				Type:      "VERIFICATION",
				KindOneOf: []string{"verification.success", "verification.failed"},
				Count:     1,
			}},
		},
	}
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	d.enforceCompletionContract(bee, nil, result)
	if result.Status != string(protocol.StatusFailed) {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected warning")
	}
}

func TestEnforceCompletionContractPasses(t *testing.T) {
	d := NewDispatcher()
	bee := colony.Bee{
		Role: "guard",
		CompletionContract: colony.CompletionContract{
			Required: []colony.CompletionRule{{
				Type:      "VERIFICATION",
				KindOneOf: []string{"verification.success", "verification.failed"},
				Count:     1,
			}},
		},
	}
	ev, err := protocol.NewEvent("t", "a", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	result := &adapters.RunResult{Status: string(protocol.StatusCompleted)}
	d.enforceCompletionContract(bee, []protocol.Event{ev}, result)
	if result.Status != string(protocol.StatusCompleted) {
		t.Fatalf("status = %q, want completed", result.Status)
	}
}
