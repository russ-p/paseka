package console

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// ReviewQueueItem is one task awaiting human review.
type ReviewQueueItem struct {
	TraceID    string    `json:"traceId"`
	TaskID     string    `json:"taskId"`
	Title      string    `json:"title"`
	Review     string    `json:"review"`
	Summary    string    `json:"summary,omitempty"`
	Bee        string    `json:"bee,omitempty"`
	Sector     string    `json:"sector,omitempty"`
	RunCount   int       `json:"runCount"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
	IsFinal    bool      `json:"isFinal"`
	CanApprove bool      `json:"canApprove"`
	CanReject  bool      `json:"canReject"`
}

// ReviewQueueView is the colony-wide review queue projection.
type ReviewQueueView struct {
	Items []ReviewQueueItem `json:"items"`
	Count int               `json:"count"`
}

// ApproveTaskRequest is the JSON body for POST .../approve.
type ApproveTaskRequest struct {
	Summary      string `json:"summary"`
	MergeMessage string `json:"mergeMessage"`
}

// ApproveTaskResponse is returned after approving a review-gated task.
type ApproveTaskResponse struct {
	TraceID   string `json:"traceId"`
	TaskID    string `json:"taskId"`
	CommitSHA string `json:"commitSha,omitempty"`
	Message   string `json:"message,omitempty"`
}

// RejectTaskRequest is the JSON body for POST .../reject.
type RejectTaskRequest struct {
	Feedback string `json:"feedback"`
}

// RejectTaskResponse is returned after rejecting a review-gated task.
type RejectTaskResponse struct {
	TraceID string `json:"traceId"`
	TaskID  string `json:"taskId"`
	Message string `json:"message,omitempty"`
}

// ListReviewQueue returns tasks awaiting human review across recent traces.
func ListReviewQueue(ctx colony.Context) (ReviewQueueView, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, taskBoardTraceLimit)
	if err != nil {
		return ReviewQueueView{}, err
	}

	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return ReviewQueueView{}, err
	}
	defer session.Close()

	var queue []ReviewQueueItem
	for _, trace := range traceSummaries {
		snap, _, err := tasks.LoadTrace(ctx, session.Ledger, trace.TraceID)
		if err != nil {
			continue
		}
		for _, task := range snap.Tasks {
			if task.Status != protocol.TaskStatusWaitingReview || !taskledger.IsReviewGate(task) {
				continue
			}
			item := taskItemFromSnapshot(ctx, trace.TraceID, snap, task)
			qi := reviewQueueItemFromTask(item)
			qi.Summary = task.Summary
			queue = append(queue, qi)
		}
	}
	sortReviewQueue(queue)
	return ReviewQueueView{Items: queue, Count: len(queue)}, nil
}

// ApproveTask approves a review-gated task using the shared review domain flow.
func ApproveTask(ctx context.Context, colonyCtx colony.Context, traceID, taskID string, req ApproveTaskRequest) (ApproveTaskResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return ApproveTaskResponse{}, err
	}
	defer session.Close()
	if session.Client == nil || session.Ledger == nil {
		return ApproveTaskResponse{}, fmt.Errorf("nats url not configured")
	}

	commitSHA, err := review.Approve(ctx, colonyCtx, session.Client, session.Ledger, review.ApproveInput{
		TraceID:      traceID,
		TaskID:       taskID,
		Summary:      req.Summary,
		MergeMessage: req.MergeMessage,
		AgentID:      "console",
	})
	if err != nil {
		return ApproveTaskResponse{}, err
	}

	msg := "Task approved."
	if commitSHA != "" {
		msg = "Task approved and worktree merged."
	}
	return ApproveTaskResponse{
		TraceID:   traceID,
		TaskID:    taskID,
		CommitSHA: commitSHA,
		Message:   msg,
	}, nil
}

// RejectTask rejects a review-gated task by publishing human feedback.
func RejectTask(ctx context.Context, colonyCtx colony.Context, traceID, taskID string, req RejectTaskRequest) (RejectTaskResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return RejectTaskResponse{}, err
	}
	defer session.Close()

	if err := validateReviewTaskTarget(colonyCtx, session, traceID, taskID); err != nil {
		return RejectTaskResponse{}, err
	}
	if session.Client == nil || session.Ledger == nil {
		return RejectTaskResponse{}, fmt.Errorf("nats url not configured")
	}

	if err := review.Reject(ctx, session.Client, session.Ledger, review.RejectInput{
		TraceID:  traceID,
		TaskID:   taskID,
		Feedback: req.Feedback,
		AgentID:  "console",
	}); err != nil {
		return RejectTaskResponse{}, err
	}

	msg := "Feedback published. For review: required tasks the runtime will return the task to ready."
	return RejectTaskResponse{
		TraceID: traceID,
		TaskID:  taskID,
		Message: msg,
	}, nil
}

func reviewQueueItemFromTask(item TaskListItem) ReviewQueueItem {
	return ReviewQueueItem{
		TraceID:    item.TraceID,
		TaskID:     item.TaskID,
		Title:      item.Title,
		Review:     item.Review,
		Bee:        item.Bee,
		Sector:     item.Sector,
		RunCount:   item.RunCount,
		UpdatedAt:  item.UpdatedAt,
		IsFinal:    item.IsFinal,
		CanApprove: item.CanApprove,
		CanReject:  item.CanReject,
	}
}

func reviewActionsForTask(task taskledger.TaskSnapshot) (canApprove, canReject bool) {
	if task.Status != protocol.TaskStatusWaitingReview {
		return false, false
	}
	if !taskledger.IsReviewGate(task) {
		return false, false
	}
	return true, true
}

func validateReviewTaskTarget(colonyCtx colony.Context, session *tasks.LedgerSession, traceID, taskID string) error {
	if traceID == "" || taskID == "" {
		return fmt.Errorf("trace and task id are required")
	}
	var ledger taskledger.Ledger
	if session != nil {
		ledger = session.Ledger
	}
	snap, _, err := tasks.LoadTrace(colonyCtx, ledger, traceID)
	if err != nil {
		return err
	}
	task, ok := snap.Tasks[taskID]
	if !ok {
		return fmt.Errorf("task %q not found in trace %s", taskID, traceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return fmt.Errorf("task %q is %q, expected waiting_review", taskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return fmt.Errorf("task %q is not a review gate task", taskID)
	}
	return nil
}

func sortReviewQueue(items []ReviewQueueItem) {
	sort.Slice(items, func(i, j int) bool {
		if !items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].UpdatedAt.After(items[j].UpdatedAt)
		}
		if items[i].TraceID != items[j].TraceID {
			return items[i].TraceID < items[j].TraceID
		}
		return items[i].TaskID < items[j].TaskID
	})
}
