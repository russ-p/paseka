package protocol

import (
	"strings"
	"time"
)

// SignalCueIngress is the payload kind of the platform observation a validated
// cue invocation publishes before its normal task or signal events.
const SignalCueIngress = "cue.ingress"

// CueIngressMode names how one cue invocation admits work: a task plan or a
// signal. It mirrors cue emit without importing the cue package.
type CueIngressMode string

const (
	CueIngressModeTask   CueIngressMode = "task"
	CueIngressModeSignal CueIngressMode = "signal"
)

// Known reports whether mode is one the cue subsystem can record.
func (m CueIngressMode) Known() bool {
	switch m {
	case CueIngressModeTask, CueIngressModeSignal:
		return true
	default:
		return false
	}
}

// CueIngressPayload is emitted as SIGNAL with payload.kind=cue.ingress.
//
// It records ingress intent: which invocation ran, through which channel, in
// which mode, and which normal ingress events that invocation owns. It is an
// observation, not a guarantee that the named events reached the broker, and it
// never creates a task, seeds honey, or dispatches a Bee.
type CueIngressPayload struct {
	Kind         string         `json:"kind"`
	InvocationID string         `json:"invocationId"`
	CueID        string         `json:"cueId"`
	Source       string         `json:"source,omitempty"`
	Mode         CueIngressMode `json:"mode"`
	InvokedAt    time.Time      `json:"invokedAt"`
	// EventIDs are the identities of the normal ingress events the invocation
	// publishes after this observation. A signal cue creates no task, so its
	// event list carries the signal event alone.
	EventIDs []string `json:"eventIds,omitempty"`
	// TaskIDs are the tasks the invocation creates, if any.
	TaskIDs []string `json:"taskIds,omitempty"`
}

// IsCueIngressObservation reports whether kind is the cue ingress observation.
func IsCueIngressObservation(kind string) bool {
	return strings.TrimSpace(kind) == SignalCueIngress
}

// IsPlatformObservation reports whether kind describes a platform decision
// rather than trail work. Platform observations are excluded from AFK direct
// routing, auto-invite matching, and other work admission: replaying one must
// never wake a Bee.
func IsPlatformObservation(kind string) bool {
	return IsEnergyObservation(kind) || IsCueIngressObservation(kind)
}
