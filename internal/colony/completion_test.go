package colony

import (
	"strings"
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

func TestCompletionContractForIntentDefaults(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      "INSIGHT",
			KindOneOf: []string{"context.note"},
		}},
	}
	for _, intent := range []string{"", "task", "unknown"} {
		got := contract.ForIntent(intent)
		if len(got.Required) != 1 || got.Required[0].KindOneOf[0] != "context.note" {
			t.Fatalf("ForIntent(%q) = %+v, want the default contract", intent, got)
		}
	}
}

func TestCompletionContractForIntentSelectsOverride(t *testing.T) {
	contract := CompletionContract{
		Required: []CompletionRule{{
			Type:      "INSIGHT",
			KindOneOf: []string{"context.note"},
		}},
		Intents: map[string]CompletionOverride{
			"skill": {Required: []CompletionRule{{
				Type:      "INSIGHT",
				KindOneOf: []string{"skill.proposed"},
			}}},
		},
	}
	got := contract.ForIntent("SKILL")
	if len(got.Required) != 1 || got.Required[0].KindOneOf[0] != "skill.proposed" {
		t.Fatalf("skill contract = %+v, want the override", got)
	}
	if got.Required[0].KindOneOf[0] == contract.Required[0].KindOneOf[0] {
		t.Fatal("override selection returned the default contract")
	}
}

func TestCompletionContractValidatesIntentOverrides(t *testing.T) {
	rule := CompletionRule{Type: "INSIGHT", KindOneOf: []string{"context.note"}}
	tests := []struct {
		name     string
		contract CompletionContract
		want     string
	}{
		{
			name: "empty intent key",
			contract: CompletionContract{Intents: map[string]CompletionOverride{
				" ": {Required: []CompletionRule{rule}},
			}},
			want: "intent key is required",
		},
		{
			name: "override without rules",
			contract: CompletionContract{Intents: map[string]CompletionOverride{
				"skill": {},
			}},
			want: "at least one rule is required",
		},
		{
			name: "override with invalid type",
			contract: CompletionContract{Intents: map[string]CompletionOverride{
				"skill": {Required: []CompletionRule{{Type: "BOGUS", KindOneOf: []string{"skill.proposed"}}}},
			}},
			want: "invalid type",
		},
		{
			name: "override without kinds",
			contract: CompletionContract{Intents: map[string]CompletionOverride{
				"skill": {Required: []CompletionRule{{Type: "INSIGHT"}}},
			}},
			want: "kind_one_of is required",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.contract.ValidateCompletionContract("archivist")
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCompletionContractWithoutOverridesValidates(t *testing.T) {
	contract := CompletionContract{Required: []CompletionRule{
		{Type: "INSIGHT", KindOneOf: []string{"context.note"}},
	}}
	if err := contract.ValidateCompletionContract("archivist"); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
