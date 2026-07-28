package protocol

// SystemEventKind identifies platform control payloads inside SIGNAL events.
type SystemEventKind string

const (
	SignalSystemKill SystemEventKind = "system.kill"

	// TraceKilledSummary is the default task summary when kill omits reason.
	TraceKilledSummary = "Trace killed by operator"
)

// SystemKillPayload is emitted as SIGNAL with payload.kind=system.kill.
type SystemKillPayload struct {
	Kind   SystemEventKind `json:"kind"`
	Reason string          `json:"reason,omitempty"`
}
