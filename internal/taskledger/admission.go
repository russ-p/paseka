package taskledger

import "fmt"

// Admission is the trail-level admission state every operator surface reads:
// whether the shared honey reserve has paused dispatch, the reason recorded when
// the pause episode opened, and how much deferred work waits behind it.
//
// It is a projection of one snapshot, never a task status. A task keeps its
// lifecycle state while the trail cannot admit it. A snapshot written before the
// pause or pending fields existed reads as admitting with an empty queue, so an
// older trail never has to be migrated to be readable.
type Admission struct {
	Paused       bool   `json:"paused"`
	PauseReason  string `json:"pauseReason,omitempty"`
	PendingCount int    `json:"pendingCount"`
}

// AdmissionOf derives the admission state from one trail snapshot.
func AdmissionOf(trace TraceSnapshot) Admission {
	admission := Admission{PendingCount: ActivePendingCount(trace)}
	if trace.EnergyPause != nil {
		admission.Paused = true
		admission.PauseReason = trace.EnergyPause.Reason
	}
	return admission
}

// PendingTaskIDs is the set of tasks whose dispatch is recorded in the active
// pending queue. It is a per-task admission view, not a lifecycle status: the
// task itself stays ready while the trail cannot admit the work.
func PendingTaskIDs(trace TraceSnapshot) map[string]bool {
	out := make(map[string]bool)
	for _, dispatch := range ActivePendingDispatches(trace) {
		if dispatch.Kind != PendingDispatchTask || dispatch.TaskID == "" {
			continue
		}
		out[dispatch.TaskID] = true
	}
	return out
}

// FormatAdmissionReport renders admission as the labeled fields `paseka energy
// show` prints. `paused` and `pending` are always present so an operator can
// tell "not paused" from "unknown"; the reason appears when an episode is open.
func FormatAdmissionReport(admission Admission) []string {
	lines := []string{fmt.Sprintf("paused:    %t", admission.Paused)}
	if admission.PauseReason != "" {
		lines = append(lines, fmt.Sprintf("reason:    %s", admission.PauseReason))
	}
	return append(lines, FormatPendingReport(admission.PendingCount)...)
}
