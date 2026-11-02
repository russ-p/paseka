package protocol

// Completion-contract verdicts recorded on a finalized AFK run result.
//
// The values are additive to the run record and need no protocol version bump:
// a legacy record without a contract field proves neither the absence nor the
// success of a declared contract, and readers must treat it as unknown.
const (
	// ContractPassed means the run's domain events satisfied every declared rule.
	ContractPassed = "passed"
	// ContractFailed means at least one declared rule was not satisfied.
	ContractFailed = "failed"
	// ContractUnavailable means the contract could not be evaluated, usually
	// because the run's event record could not be read. It is never a pass.
	ContractUnavailable = "unavailable"
)

// ContractRule is one effective declared completion-contract rule, with the
// defaults the runtime applied (a missing count requires exactly one event).
type ContractRule struct {
	Type      string   `json:"type"`
	KindOneOf []string `json:"kindOneOf,omitempty"`
	Count     int      `json:"count"`
}

// ContractObservation is one declared rule measured against the run's domain
// events. Want is the effective required count, so a reader can tell what was
// asked for without re-deriving the default.
type ContractObservation struct {
	Type      string   `json:"type"`
	KindOneOf []string `json:"kindOneOf,omitempty"`
	Want      int      `json:"want"`
	Got       int      `json:"got"`
	Satisfied bool     `json:"satisfied"`
}

// ContractRecord is the runtime-owned completion-contract verdict persisted on
// result.json. It is present for every finalized AFK run whose bee declares a
// contract and omitted entirely otherwise, so "no declared contract" stays
// distinguishable from "contract passed".
//
// Observations carry the measured counts when the contract was evaluated;
// Error carries the evaluation error when it was not (Status unavailable).
type ContractRecord struct {
	Status       string                `json:"status"`
	Rules        []ContractRule        `json:"rules,omitempty"`
	Observations []ContractObservation `json:"observations,omitempty"`
	Error        string                `json:"error,omitempty"`
}
