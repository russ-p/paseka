package protocol

// GateResult is the runtime-owned mechanical check verdict recorded for one run.
//
// It is written on the run record and copied onto the proposal archive record so
// an audit of a proposal outlives the run directory that produced it. A bee
// without a gate records no gate at all: absence stays distinguishable from a
// pass.
type GateResult struct {
	// Command is the argv as executed, after variable substitution.
	Command []string `json:"command,omitempty"`
	// ExitCode is the process exit status. It is -1 when the process never
	// started or was killed by a signal.
	ExitCode int `json:"exitCode"`
	// DurationMs is the wall-clock duration of the check process.
	DurationMs int64 `json:"durationMs"`
	// TimedOut reports that the gate exceeded checks.timeout and was stopped.
	TimedOut bool `json:"timedOut,omitempty"`
	// Truncated reports that captured output exceeded the hard byte cap.
	Truncated bool `json:"truncated,omitempty"`
	// LogPath is the colony-root-relative path to the captured output.
	LogPath string `json:"logPath,omitempty"`
	// Passed is the verdict: true only when the process started, was not
	// signalled or timed out, and exited zero.
	Passed bool `json:"passed"`
	// Reason classifies a failure: exit, signal, timeout, exec, or log.
	Reason string `json:"reason,omitempty"`
	// Signal names the signal that killed the check process, when one did.
	Signal string `json:"signal,omitempty"`
	// Summary is the one-line verdict carried by the rejection event and task
	// status. It never contains the captured log.
	Summary string `json:"summary,omitempty"`
}
