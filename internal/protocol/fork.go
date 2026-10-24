package protocol

// ForkEventKind identifies platform trail-fork control payloads inside SIGNAL events.
type ForkEventKind string

// OperatorAgentID is the reserved sender identity for operator control-path
// actions. Decision events are admitted by this positive marker; a sender that
// does not carry it is not an operator, however it resolves on disk.
const OperatorAgentID = "operator"

const (
	// SignalTrailFork is an agent-originated request to fork a trail. It is
	// recorded as a durable pending request and creates no runnable resources.
	SignalTrailFork ForkEventKind = "trail.fork"
	// SignalTrailForkApprove advances a pending request and performs creation.
	// It is an operator control-path action.
	SignalTrailForkApprove ForkEventKind = "trail.fork.approve"
	// SignalTrailForkReject refuses a pending request without creating anything.
	SignalTrailForkReject ForkEventKind = "trail.fork.reject"
	// SignalTrailForkCancel cancels a pending request without creating anything.
	SignalTrailForkCancel ForkEventKind = "trail.fork.cancel"
)

// ForkRequestPayload is emitted as SIGNAL with payload.kind=trail.fork.
//
// Delta is an explicit selection: "none" for a control arm or "patch" with a
// proposal patch selector in Patch.
type ForkRequestPayload struct {
	Kind        ForkEventKind `json:"kind"`
	RequestID   string        `json:"requestId"`
	SourceTrace string        `json:"sourceTrace"`
	SourceTask  string        `json:"sourceTask,omitempty"`
	SourceRun   string        `json:"sourceRun,omitempty"`
	BaseSHA     string        `json:"baseSha,omitempty"`
	Delta       string        `json:"delta"`
	Patch       string        `json:"patch,omitempty"`
	Budget      int           `json:"budget,omitempty"`
	Title       string        `json:"title,omitempty"`
}

// ForkDecisionPayload is emitted as SIGNAL for approve/reject/cancel actions.
type ForkDecisionPayload struct {
	Kind      ForkEventKind `json:"kind"`
	RequestID string        `json:"requestId"`
	Reason    string        `json:"reason,omitempty"`
}
