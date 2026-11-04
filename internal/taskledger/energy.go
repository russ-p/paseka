package taskledger

import (
	"fmt"
	"strconv"
	"time"

	"github.com/russ-p/paseka/internal/ids"
	"github.com/russ-p/paseka/internal/protocol"
)

// EnsureEnergySeeded initializes honey reserve fields when the trace has no budget yet.
func EnsureEnergySeeded(trace TraceSnapshot, budget int) (TraceSnapshot, bool) {
	updated, _, changed := EnsureEnergySeed(trace, budget)
	return updated, changed
}

// EnsureEnergySeed is the authoritative seed transition. It allocates the
// reserve once, records the winning seed identity, effective budget, and actual
// seed time, and reports whether this call performed the seed.
//
// A competing request observes the recorded winner instead of allocating a
// second budget. A ledger written before seed provenance existed keeps its
// positive budget but has no record: it stays unknown rather than being given
// an invented identity, budget, or timestamp.
func EnsureEnergySeed(trace TraceSnapshot, budget int) (TraceSnapshot, EnergySeedSnapshot, bool) {
	if trace.EnergySeed != nil {
		return trace, *trace.EnergySeed, false
	}
	if trace.EnergyBudget > 0 {
		// A positive budget without a record predates seed provenance. It stays
		// unknown: no identity, budget, or timestamp is invented for it.
		return trace, EnergySeedSnapshot{}, false
	}
	if budget <= 0 {
		budget = protocol.DefaultEnergyBudget
	}
	now := time.Now().UTC()
	seed := EnergySeedSnapshot{
		SeedID:   newSeedID(trace.TraceID, now),
		Budget:   budget,
		SeededAt: now,
	}
	trace.EnergySeed = &seed
	trace.EnergyAdded = 0
	// Preserve honey from energy.add that landed before formal seed.
	trace.EnergyBudget = budget
	if trace.EnergyRemaining <= 0 {
		trace.EnergyRemaining = budget
	}
	return trace, seed, true
}

// InitialEnergyBudget returns the winning initial allocation recorded by the
// seed transition. ok is false for an unseeded trail and for a legacy ledger
// that has a budget but no provenance, so a reader never substitutes today's
// default for a missing seed record.
func InitialEnergyBudget(trace TraceSnapshot) (int, bool) {
	if trace.EnergySeed == nil || trace.EnergySeed.Budget <= 0 {
		return 0, false
	}
	return trace.EnergySeed.Budget, true
}

// MarkEnergySeedPublished records that seedID's observation reached the
// publisher. It reports whether the flag changed; a second call is a no-op and
// an identity that does not match the recorded winner is refused.
func MarkEnergySeedPublished(trace TraceSnapshot, seedID string) (TraceSnapshot, bool, error) {
	if trace.EnergySeed == nil {
		return trace, false, fmt.Errorf("taskledger: no seed record to publish")
	}
	if trace.EnergySeed.SeedID != seedID {
		return trace, false, fmt.Errorf("taskledger: seed %q does not match recorded seed %q", seedID, trace.EnergySeed.SeedID)
	}
	if trace.EnergySeed.Published {
		return trace, false, nil
	}
	seed := *trace.EnergySeed
	seed.Published = true
	trace.EnergySeed = &seed
	return trace, true, nil
}

func newSeedID(traceID string, now time.Time) string {
	if id, err := ids.Prefixed("seed-"); err == nil {
		return id
	}
	return fmt.Sprintf("seed-%s-%d", traceID, now.UnixNano())
}

// Allocated is the display denominator after seed: seed plus post-seed top-ups.
// Returns 0 when the trail is not yet seeded.
func Allocated(budget, added int) int {
	if budget <= 0 {
		return 0
	}
	return budget + added
}

// formatHoneyFraction is the shared core for the fraction formatters:
// "remaining<sep>allocated<suffix>" once seeded; bare remaining before seed (never "N/0").
func formatHoneyFraction(remaining, budget, added int, sep, suffix string) string {
	den := Allocated(budget, added)
	if den <= 0 {
		return strconv.Itoa(remaining)
	}
	return fmt.Sprintf("%d%s%d%s", remaining, sep, den, suffix)
}

// FormatHoneyPrimary is "remaining / allocated" after seed; remaining only when unseeded (never "N / 0").
func FormatHoneyPrimary(remaining, budget, added int) string {
	return formatHoneyFraction(remaining, budget, added, " / ", "")
}

// FormatHoneyCompact is remaining/allocated without spaces, or remaining alone when unseeded.
func FormatHoneyCompact(remaining, budget, added int) string {
	return formatHoneyFraction(remaining, budget, added, "/", "")
}

// FormatHoneyRemaining is "n/m remaining" after seed — the reading `paseka status` prints —
// and bare remaining when unseeded (never "n/0").
func FormatHoneyRemaining(remaining, budget, added int) string {
	return formatHoneyFraction(remaining, budget, added, "/", " remaining")
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
		if Allocated(budget, added) > 0 {
			lines = append(lines, FormatHoneyPrimary(remaining, budget, added))
			if extra := FormatHoneySecondary(budget, added); extra != "" {
				lines = append(lines, extra)
			}
		}
	}
	return lines
}

// FormatPendingReport reports how many deferred dispatches still await
// admission. The count is always printed so an empty queue reads as an explicit
// zero rather than a missing field.
func FormatPendingReport(active int) []string {
	return []string{fmt.Sprintf("pending:   %d", active)}
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
