package taskledger

import (
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/ids"
	"github.com/russ-p/paseka/internal/protocol"
)

// ActivePendingCount is the number of deferred dispatch entries that still
// await admission. Observations and closed entries are not counted.
func ActivePendingCount(trace TraceSnapshot) int {
	count := 0
	for _, dispatch := range trace.PendingDispatches {
		if dispatch.Status.Active() {
			count++
		}
	}
	return count
}

// requiresEnergyDeferralReason reports whether a deferral waited on the trail's
// honey reserve rather than on a configuration transition. A configuration
// barrier hold is not energy waiting: its own boundary releases or cancels it,
// so it neither opens nor closes a pause episode.
func requiresEnergyDeferralReason(reason string) bool {
	return reason != protocol.ConfigBarrierHoldReason
}

// OpenEnergyPause starts the episode the first energy-caused deferral opens. It
// is called inside the same ledger mutation that records the deferral, so the
// episode and the work that caused it can never disagree. An existing episode is
// kept: the first pause owns the whole interval until an increase closes it.
func OpenEnergyPause(trace TraceSnapshot, reason string, now time.Time) (TraceSnapshot, bool) {
	if trace.EnergyPause != nil || trace.Killed {
		return trace, false
	}
	episodeID, err := ids.Prefixed("pause-")
	if err != nil {
		episodeID = fmt.Sprintf("pause-%s-%d", trace.TraceID, now.UnixNano())
	}
	trace.EnergyPause = &EnergyPauseSnapshot{
		EpisodeID: episodeID,
		Reason:    reason,
		StartedAt: now,
		Remaining: trace.EnergyRemaining,
		Budget:    Allocated(trace.EnergyBudget, trace.EnergyAdded),
		Pending:   ActivePendingCount(trace),
	}
	return trace, true
}

// ClaimEnergyPausePublished marks the open episode published and returns it. The
// compare-and-set that persists the marking is the dedupe: only the caller that
// moves the flag from false to true may send the paused observation, so a
// replay, a restart, or a second reactor cannot send it again. It reports false
// when there is no unpublished episode to claim.
func ClaimEnergyPausePublished(trace TraceSnapshot) (TraceSnapshot, EnergyPauseSnapshot, bool) {
	if trace.EnergyPause == nil || trace.EnergyPause.Published {
		return trace, EnergyPauseSnapshot{}, false
	}
	pause := *trace.EnergyPause
	pause.Published = true
	// Refresh the count so the observation reports the queue as sent, not only
	// as it stood when the episode opened.
	pause.Pending = ActivePendingCount(trace)
	trace.EnergyPause = &pause
	return trace, pause, true
}

// CloseEnergyPause clears the open episode and returns it, so the runtime sends
// at most one resumed observation after the field is already gone. It reports
// false when no episode is open.
func CloseEnergyPause(trace TraceSnapshot) (TraceSnapshot, EnergyPauseSnapshot, bool) {
	if trace.EnergyPause == nil {
		return trace, EnergyPauseSnapshot{}, false
	}
	pause := *trace.EnergyPause
	pause.Pending = ActivePendingCount(trace)
	trace.EnergyPause = nil
	return trace, pause, true
}
