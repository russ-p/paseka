package protocol

import "time"

// TaskEventKind identifies task lifecycle payloads inside bus event payload.kind.
type TaskEventKind string

const (
	TaskEventPlan      TaskEventKind = "task.plan"
	TaskEventReady     TaskEventKind = "task.ready"
	TaskEventStatus    TaskEventKind = "task.status"
	TaskEventCompleted TaskEventKind = "task.completed"
)

// TaskReviewPolicy controls when a task requires human review before completion.
type TaskReviewPolicy string

const (
	// TaskReviewNone is the default: no human mid-task review. Runtime auto-completes
	// unless a colony bee explicitly publishes task.completed and this run opened
	// a code.proposal gate (builder path); then status stays waiting_review until
	// the declared publisher emits task.completed on the bus.
	TaskReviewNone TaskReviewPolicy = "none"
	// TaskReviewRequired waits for human approval after the bee run succeeds.
	TaskReviewRequired TaskReviewPolicy = "required"
	// TaskReviewFinal is the trace-level merge gate after all other tasks complete.
	TaskReviewFinal TaskReviewPolicy = "final"
)

// NormalizeTaskReviewPolicy returns a known review policy or TaskReviewNone.
func NormalizeTaskReviewPolicy(p TaskReviewPolicy) TaskReviewPolicy {
	switch p {
	case TaskReviewRequired, TaskReviewFinal:
		return p
	default:
		return TaskReviewNone
	}
}

// TaskPhase is the ledger phase of one task within a trace. The vocabulary is
// closed: a colony cannot name phases of its own, because eligibility must not
// depend on configuration at replay time.
type TaskPhase string

const (
	// TaskPhaseWork is the default phase: ordinary trail work a bee or operator
	// may plan. An empty phase reads as work.
	TaskPhaseWork TaskPhase = "work"
	// TaskPhaseClosing marks the runtime-seeded trail wrap-up round. Only the
	// runtime may publish it; every other publisher is coerced to work.
	TaskPhaseClosing TaskPhase = "closing"
	// TaskPhaseMerge marks the trace-level merge gate. It is derived from
	// review: final and is never dispatched.
	TaskPhaseMerge TaskPhase = "merge"
)

// RuntimeAgentID is the reserved agent id the platform runtime publishes under.
// A task.plan carrying it is the only plan honoured as phase closing.
const RuntimeAgentID = "runtime"

// IsKnownTaskPhase reports whether p is part of the closed phase vocabulary.
// The empty value is accepted and means the default, work.
func IsKnownTaskPhase(p TaskPhase) bool {
	switch p {
	case "", TaskPhaseWork, TaskPhaseClosing, TaskPhaseMerge:
		return true
	default:
		return false
	}
}

// NormalizeTaskPhase returns a known phase, defaulting to TaskPhaseWork. An
// empty phase is the default and an unknown name is not a phase.
func NormalizeTaskPhase(p TaskPhase) TaskPhase {
	switch p {
	case TaskPhaseClosing, TaskPhaseMerge:
		return p
	default:
		return TaskPhaseWork
	}
}

// DeriveTaskPhase resolves the phase of a recorded task: review: final always
// reads as merge, and merge without that policy falls back to work. It also
// covers snapshots written before phase existed, where the recorded phase is
// empty and the review policy is the only evidence.
func DeriveTaskPhase(phase TaskPhase, review TaskReviewPolicy) TaskPhase {
	if NormalizeTaskReviewPolicy(review) == TaskReviewFinal {
		return TaskPhaseMerge
	}
	p := NormalizeTaskPhase(phase)
	if p == TaskPhaseMerge {
		return TaskPhaseWork
	}
	return p
}

// TaskStatus is the lifecycle state of one task within a trace.
type TaskStatus string

const (
	TaskStatusPlanned       TaskStatus = "planned"
	TaskStatusReady         TaskStatus = "ready"
	TaskStatusRunning       TaskStatus = "running"
	TaskStatusWaitingReview TaskStatus = "waiting_review"
	TaskStatusCompleted     TaskStatus = "completed"
	TaskStatusFailed        TaskStatus = "failed"
	TaskStatusBlocked       TaskStatus = "blocked"
	TaskStatusCancelled     TaskStatus = "cancelled"
)

// TaskSpec describes one planned task inside a trace.
type TaskSpec struct {
	TaskID    string           `json:"taskId"`
	Title     string           `json:"title"`
	Body      string           `json:"body,omitempty"`
	Bee       string           `json:"bee,omitempty"`
	Sector    string           `json:"sector,omitempty"`
	Intent    string           `json:"intent,omitempty"`
	Review    TaskReviewPolicy `json:"review,omitempty"`
	Phase     TaskPhase        `json:"phase,omitempty"`
	DependsOn []string         `json:"dependsOn,omitempty"`
	CueID     string           `json:"cueId,omitempty"`
}

// TaskPlanPayload is emitted as INSIGHT with payload.kind=task.plan.
type TaskPlanPayload struct {
	Kind  TaskEventKind `json:"kind"`
	Tasks []TaskSpec    `json:"tasks"`
}

// TaskReadyPayload is emitted as SIGNAL with payload.kind=task.ready.
type TaskReadyPayload struct {
	Kind   TaskEventKind `json:"kind"`
	TaskID string        `json:"taskId"`
	Title  string        `json:"title,omitempty"`
	Body   string        `json:"body,omitempty"`
	Bee    string        `json:"bee,omitempty"`
	Sector string        `json:"sector,omitempty"`
	Intent string        `json:"intent,omitempty"`
	CueID  string        `json:"cueId,omitempty"`
}

// TaskStatusPayload is emitted as SIGNAL with payload.kind=task.status.
type TaskStatusPayload struct {
	Kind    TaskEventKind `json:"kind"`
	TaskID  string        `json:"taskId"`
	Status  TaskStatus    `json:"status"`
	Summary string        `json:"summary,omitempty"`
}

// TaskCompletedPayload is emitted as VERIFICATION with payload.kind=task.completed.
type TaskCompletedPayload struct {
	Kind        TaskEventKind `json:"kind"`
	TaskID      string        `json:"taskId"`
	Status      TaskStatus    `json:"status"`
	Summary     string        `json:"summary,omitempty"`
	Commit      string        `json:"commit,omitempty"`
	CompletedAt time.Time     `json:"completedAt,omitempty"`
}
