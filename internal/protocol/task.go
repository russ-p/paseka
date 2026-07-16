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
	DependsOn []string         `json:"dependsOn,omitempty"`
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
