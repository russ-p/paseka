package colony

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
)

func TestCompletionContractValidateRunEvents(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      "VERIFICATION",
			KindOneOf: []string{"verification.success", "verification.failed"},
			Count:     1,
		}},
	}
	success, err := protocol.NewEvent("trace-1", "agent-1", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind:    protocol.VerificationSuccess,
		Summary: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateRunEvents([]protocol.Event{success}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompletionContractRejectsMissingEmit(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      "VERIFICATION",
			KindOneOf: []string{"verification.success", "verification.failed"},
			Count:     1,
		}},
	}
	if err := contract.ValidateRunEvents(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestCompletionContractRejectsDuplicateEmit(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      "VERIFICATION",
			KindOneOf: []string{"verification.success", "verification.failed"},
			Count:     1,
		}},
	}
	ev1, _ := protocol.NewEvent("t", "a", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	ev2, _ := protocol.NewEvent("t", "a", 2, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationFailed, Summary: "no",
	})
	if err := contract.ValidateRunEvents([]protocol.Event{ev1, ev2}); err == nil {
		t.Fatal("expected error for duplicate emits")
	}
}
