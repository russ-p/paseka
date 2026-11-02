package colony

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
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

func TestCompletionContractEffectiveRulesApplyCountDefault(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      " VERIFICATION ",
			KindOneOf: []string{" verification.success "},
		}},
	}
	rules := contract.EffectiveRules()
	if len(rules) != 1 {
		t.Fatalf("rules = %+v, want one", rules)
	}
	if rules[0].Type != "VERIFICATION" || rules[0].Count != 1 {
		t.Fatalf("effective rule = %+v, want trimmed type and default count 1", rules[0])
	}
	if len(rules[0].KindOneOf) != 1 || rules[0].KindOneOf[0] != "verification.success" {
		t.Fatalf("effective kinds = %+v, want trimmed", rules[0].KindOneOf)
	}
}

func TestCompletionContractObserveRunEventsReportsEveryRule(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{
			{Type: "VERIFICATION", KindOneOf: []string{"verification.success"}, Count: 1},
			{Type: "MUTATION", KindOneOf: []string{"code.proposal.isolated"}, Count: 2},
		},
	}
	success, _ := protocol.NewEvent("t", "a", 1, protocol.EventVerification, protocol.VerificationPayload{
		Kind: protocol.VerificationSuccess, Summary: "ok",
	})
	observations := contract.ObserveRunEvents([]protocol.Event{success})
	if len(observations) != 2 {
		t.Fatalf("observations = %+v, want one per rule", observations)
	}
	if !observations[0].Satisfied() || observations[0].Got != 1 || observations[0].Want != 1 {
		t.Fatalf("first observation = %+v, want satisfied", observations[0])
	}
	if observations[1].Satisfied() || observations[1].Got != 0 || observations[1].Want != 2 {
		t.Fatalf("second observation = %+v, want unsatisfied want=2 got=0", observations[1])
	}
}
