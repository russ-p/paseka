package protocol

import (
	"encoding/json"
	"strings"
)

// PayloadKind extracts payload.kind from a bus event payload.
func PayloadKind(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Kind)
}

// PayloadTaskID extracts payload.taskId from a bus event payload when present.
func PayloadTaskID(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var meta struct {
		TaskID string `json:"taskId"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.TaskID)
}

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

// VerificationKind identifies verification payload variants.
type VerificationKind string

const (
	VerificationSuccess VerificationKind = "verification.success"
	VerificationFailed  VerificationKind = "verification.failed"
)

// MutationPayload is emitted as MUTATION for code change proposals.
type MutationPayload struct {
	Kind    MutationKind `json:"kind,omitempty"`
	Diff    string       `json:"diff,omitempty"`
	Summary string       `json:"summary,omitempty"`
	Ref     string       `json:"ref,omitempty"` // object store reference for large artifacts
	TaskID  string       `json:"taskId,omitempty"`
}

// VerificationPayload is emitted as VERIFICATION for review outcomes.
type VerificationPayload struct {
	Kind    VerificationKind `json:"kind,omitempty"`
	TaskID  string           `json:"taskId,omitempty"`
	Summary string           `json:"summary,omitempty"`
}

// InsightKindForEventType returns the expected top-level event type for an insight kind string.
func InsightKindForEventType(kind string) EventType {
	switch InsightKind(kind) {
	case InsightRunSummary, InsightReviewNote, InsightContextNote, InsightHumanFeedback:
		return EventInsight
	default:
		return ""
	}
}
