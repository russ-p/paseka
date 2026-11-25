package runs

import (
	"fmt"

	"github.com/russ-p/paseka/internal/protocol"
)

// RunInterruption explains why a run whose outcome records still read as in
// flight was projected as failed: the process its status record names is not
// running on this host. It is presentational as well as diagnostic — an
// operator sees the pid and the reason instead of a silent failed badge.
type RunInterruption struct {
	// Reason is the machine-readable classifier written to the projected run.
	Reason string `json:"reason"`
	// PID is the recorded process identity that was checked.
	PID int `json:"pid,omitempty"`
	// Detail is the human-readable sentence stating what was found.
	Detail string `json:"detail,omitempty"`
}

// projectInterruptedRun reports whether a projected state is still in flight
// while its recorded process is gone.
//
// Only a non-terminal state carries an interruption: a terminal record already
// owns the outcome, and a recorded pair that fail-closes into failed keeps its
// own record-integrity marker. An interactive session is skipped because
// session.json is its own liveness record and a run is never failed just for
// lacking a pid. The state is not rewritten here; the caller decides how to
// project it, and the read path never writes.
func projectInterruptedRun(state string, pid int, session bool) (RunInterruption, bool) {
	if session || pid <= 0 {
		return RunInterruption{}, false
	}
	switch protocol.RunStatus(state) {
	case protocol.StatusRunning, protocol.StatusQueued:
	default:
		return RunInterruption{}, false
	}
	if PIDAlive(pid) {
		return RunInterruption{}, false
	}
	return RunInterruption{
		Reason: protocol.ReasonProcessGone,
		PID:    pid,
		Detail: InterruptionDetail(protocol.RunStatus(state), pid),
	}, true
}

// InterruptionDetail is the operator-facing sentence for a run whose recorded
// process is gone, shared by the projection and the reconciler so the Console
// and the written record say the same thing.
func InterruptionDetail(state protocol.RunStatus, pid int) string {
	name := string(state)
	if name == "" {
		name = "in flight"
	}
	return fmt.Sprintf(
		"status.json said %s with pid %d, but that process is not running on this host; the run is recorded as failed",
		name, pid,
	)
}
