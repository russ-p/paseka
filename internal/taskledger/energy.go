package taskledger

import (
	"fmt"
	"strconv"

	"github.com/paseka/paseka/internal/protocol"
)

// EnsureEnergySeeded initializes honey reserve fields when the trace has no budget yet.
func EnsureEnergySeeded(trace TraceSnapshot, budget int) (TraceSnapshot, bool) {
	if trace.EnergyBudget > 0 {
		return trace, false
	}
	if budget <= 0 {
		budget = protocol.DefaultEnergyBudget
	}
	trace.EnergyAdded = 0
	// Preserve honey from energy.add that landed before formal seed.
	if trace.EnergyRemaining > 0 {
		trace.EnergyBudget = budget
		return trace, true
	}
	trace.EnergyBudget = budget
	trace.EnergyRemaining = budget
	return trace, true
}

// Allocated is the display denominator after seed: seed plus post-seed top-ups.
// Returns 0 when the trail is not yet seeded.
func Allocated(budget, added int) int {
	if budget <= 0 {
		return 0
	}
	return budget + added
}

// FormatHoneyPrimary is "remaining / allocated" after seed; remaining only when unseeded (never "N / 0").
func FormatHoneyPrimary(remaining, budget, added int) string {
	den := Allocated(budget, added)
	if den <= 0 {
		return strconv.Itoa(remaining)
	}
	return fmt.Sprintf("%d / %d", remaining, den)
}

// FormatHoneyCompact is remaining/allocated without spaces, or remaining alone when unseeded.
func FormatHoneyCompact(remaining, budget, added int) string {
	den := Allocated(budget, added)
	if den <= 0 {
		return strconv.Itoa(remaining)
	}
	return fmt.Sprintf("%d/%d", remaining, den)
}

// FormatHoneySecondary is omitted when there are no post-seed top-ups.
func FormatHoneySecondary(budget, added int) string {
	if added <= 0 {
		return ""
	}
	return fmt.Sprintf("seed %d · topped %d", budget, added)
}

// FormatHoneyReport is labeled CLI fields plus remaining/allocated when topped up after seed.
func FormatHoneyReport(remaining, budget, added int) []string {
	lines := []string{
		fmt.Sprintf("budget:    %d", budget),
		fmt.Sprintf("remaining: %d", remaining),
	}
	if added > 0 {
		lines = append(lines, fmt.Sprintf("added:     %d", added))
	}
	if Allocated(budget, added) > 0 && added > 0 {
		lines = append(lines, FormatHoneyPrimary(remaining, budget, added))
		if extra := FormatHoneySecondary(budget, added); extra != "" {
			lines = append(lines, extra)
		}
	}
	return lines
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
