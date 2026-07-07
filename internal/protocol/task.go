package protocol

import "time"

// TaskEventKind identifies task lifecycle payloads inside bus event payload.kind.
type TaskEventKind string

const (
	TaskEventPlan      TaskEventKind = "task.plan"
	TaskEventReady     TaskEventKind = "task.ready"
	TaskEventCompleted TaskEventKind = "task.completed"
)

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
	TaskID    string   `json:"taskId"`
	Title     string   `json:"title"`
	Body      string   `json:"body,omitempty"`
	Bee       string   `json:"bee,omitempty"`
	Intent    string   `json:"intent,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"`
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
	Intent string        `json:"intent,omitempty"`
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
