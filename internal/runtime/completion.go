package runtime

import (
	"fmt"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// enforceCompletionContract evaluates the bee's declared completion contract
// and reconciles both outcome records with the verdict.
//
// Finalization owns the persisted outcome: this runs after the check gate and
// before a successful result is exposed to the caller or published, so the run
// directory cannot keep saying completed about a run the runtime failed.
//
// The gate verdict outranks the contract verdict. A run the gate rejected (or an
// adapter that failed, timed out, or was cancelled) keeps its own state and
// reason; the contract verdict is still recorded, but it never relabels that
// primary failure. A contract changes only an otherwise completed run.
func (d *Dispatcher) enforceCompletionContract(runDir runs.Dir, bee colony.Bee, runEvents []protocol.Event, readErr error, result *adapters.RunResult) error {
	record, message := contractVerdict(bee, runEvents, readErr)
	if record == nil || result == nil {
		return nil
	}
	result.Contract = record

	update := runs.RunRecordReconciliation{Contract: record}
	if record.Status != protocol.ContractPassed && result.Status == string(protocol.StatusCompleted) {
		result.Status = string(protocol.StatusFailed)
		result.Reason = protocol.ReasonCompletionContract
		if result.Err == nil {
			result.Err = fmt.Errorf("%s", message)
		}
		result.Warnings = append(result.Warnings, message)
		update.State = protocol.StatusFailed
		update.Reason = protocol.ReasonCompletionContract
		update.Note = message
	}
	if err := runDir.ReconcileRunRecords(update); err != nil {
		return fmt.Errorf("runtime: reconcile run records: %w", err)
	}
	return nil
}

// contractVerdict builds the persisted contract verdict for a bee that declares
// one, together with the human-readable message a failed or unavailable verdict
// carries onto the outcome records. It returns a nil record when the bee
// declares no contract, so "no declared contract" stays distinguishable from
// "contract passed".
//
// An unreadable event record yields unavailable — never a pass — with the
// evaluation error, because an absent verdict must not be readable as success.
func contractVerdict(bee colony.Bee, runEvents []protocol.Event, readErr error) (*protocol.ContractRecord, string) {
	if len(bee.CompletionContract.Required) == 0 {
		return nil, ""
	}
	record := &protocol.ContractRecord{
		Rules: effectiveContractRules(bee.CompletionContract),
	}
	if readErr != nil {
		message := fmt.Sprintf("runtime: completion contract unavailable: %s", readErr.Error())
		record.Status = protocol.ContractUnavailable
		record.Error = message
		return record, message
	}

	domain := contractEvents(runEvents)
	for _, obs := range bee.CompletionContract.ObserveRunEvents(domain) {
		record.Observations = append(record.Observations, protocol.ContractObservation{
			Type:      obs.Type,
			KindOneOf: obs.KindOneOf,
			Want:      obs.Want,
			Got:       obs.Got,
			Satisfied: obs.Satisfied(),
		})
	}

	if err := bee.CompletionContract.ValidateRunEvents(domain); err != nil {
		record.Status = protocol.ContractFailed
		return record, fmt.Sprintf("runtime: %s", err.Error())
	}
	record.Status = protocol.ContractPassed
	return record, ""
}

// effectiveContractRules renders the declared rules with the defaults the
// runtime actually applied, so the record says what was checked rather than
// what was typed.
func effectiveContractRules(contract colony.CompletionContract) []protocol.ContractRule {
	declared := contract.EffectiveRules()
	rules := make([]protocol.ContractRule, 0, len(declared))
	for _, rule := range declared {
		rules = append(rules, protocol.ContractRule{
			Type:      rule.Type,
			KindOneOf: rule.KindOneOf,
			Count:     rule.Count,
		})
	}
	return rules
}

// contractEvents is what the completion contract is evaluated against: the
// domain events the run's own bee emitted. Runtime-authored events are excluded
// because the platform's own records — above all the gate rejection this
// finalizer appends — must never satisfy a bee's declared contract.
func contractEvents(events []protocol.Event) []protocol.Event {
	var out []protocol.Event
	for _, ev := range events {
		if ev.AgentID == protocol.RuntimeAgentID {
			continue
		}
		if !protocol.IsDomainEvent(ev.Type) {
			continue
		}
		out = append(out, ev)
	}
	return out
}
