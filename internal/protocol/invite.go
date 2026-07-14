package protocol

// InviteEventKind identifies Human Gateway payloads inside SIGNAL events.
type InviteEventKind string

const (
	SignalSessionInvite  InviteEventKind = "session.invite"
	SignalBeekeeperReady InviteEventKind = "beekeeper.ready"
)

// InviteStatus is the lifecycle of a session invite.
type InviteStatus string

const (
	InviteStatusPending    InviteStatus = "pending"
	InviteStatusAccepted   InviteStatus = "accepted"
	InviteStatusCancelled  InviteStatus = "cancelled"
	InviteStatusCompleted  InviteStatus = "completed"
	InviteStatusIncomplete InviteStatus = "incomplete"
)

// BeekeeperAction is the Beekeeper response to a pending invite.
type BeekeeperAction string

const (
	BeekeeperAccept BeekeeperAction = "accept"
	BeekeeperReject BeekeeperAction = "reject"
	BeekeeperDefer  BeekeeperAction = "defer"
)

// SessionInvitePayload is emitted as SIGNAL with payload.kind=session.invite.
type SessionInvitePayload struct {
	Kind     InviteEventKind `json:"kind"`
	InviteID string          `json:"inviteId"`
	Bee      string          `json:"bee"`
	Intent   string          `json:"intent,omitempty"`
	Task     string          `json:"task"`
	Status   InviteStatus    `json:"status"`
	SpecRef  string          `json:"specRef,omitempty"`
}

// BeekeeperReadyPayload is emitted as SIGNAL with payload.kind=beekeeper.ready.
type BeekeeperReadyPayload struct {
	Kind     InviteEventKind `json:"kind"`
	InviteID string          `json:"inviteId"`
	Action   BeekeeperAction `json:"action"`
}
