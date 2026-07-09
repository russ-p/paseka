package taskledger

import "github.com/paseka/paseka/internal/protocol"

// EnsureEnergySeeded initializes honey reserve fields when the trace has no budget yet.
func EnsureEnergySeeded(trace TraceSnapshot, budget int) (TraceSnapshot, bool) {
	if trace.EnergyBudget > 0 {
		return trace, false
	}
	if budget <= 0 {
		budget = protocol.DefaultEnergyBudget
	}
	// Preserve honey from energy.add that landed before formal seed.
	if trace.EnergyRemaining > 0 {
		trace.EnergyBudget = budget
		return trace, true
	}
	trace.EnergyBudget = budget
	trace.EnergyRemaining = budget
	return trace, true
}

// HasEnergy reports whether the trace has enough honey reserve for one dispatch.
func HasEnergy(trace TraceSnapshot, amount int) bool {
	if amount <= 0 {
		return true
	}
	return trace.EnergyRemaining >= amount
}

// IsEnergyBlockedTask reports whether a task was blocked due to exhausted honey reserve.
func IsEnergyBlockedTask(task TaskSnapshot) bool {
	return task.Status == protocol.TaskStatusBlocked &&
		task.Summary == protocol.HoneyReserveExhaustedSummary
}
