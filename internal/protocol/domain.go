package protocol

// IsDomainEvent reports whether an event type belongs on the NATS bus.
func IsDomainEvent(t EventType) bool {
	switch t {
	case EventSignal, EventInsight, EventMutation, EventVerification:
		return true
	default:
		return false
	}
}

// MutationKind identifies mutation payload variants.
type MutationKind string

const (
	MutationCodeProposal MutationKind = "code.proposal"
)

// MutationPayload is emitted as MUTATION for code change proposals.
type MutationPayload struct {
	Kind    MutationKind `json:"kind,omitempty"`
	Diff    string       `json:"diff,omitempty"`
	Summary string       `json:"summary,omitempty"`
	Ref     string       `json:"ref,omitempty"` // object store reference for large artifacts
}
