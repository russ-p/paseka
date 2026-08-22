package console

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// ReviewQueueItem is one task awaiting human review.
type ReviewQueueItem struct {
	TraceID           string    `json:"traceId"`
	TaskID            string    `json:"taskId"`
	Title             string    `json:"title"`
	Review            string    `json:"review"`
	Summary           string    `json:"summary,omitempty"`
	TraceSummary      string    `json:"traceSummary,omitempty"`
	Bee               string    `json:"bee,omitempty"`
	Sector            string    `json:"sector,omitempty"`
	RunCount          int       `json:"runCount"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
	IsFinal           bool      `json:"isFinal"`
	ProposalWorkspace string    `json:"proposalWorkspace,omitempty"`
	CanApprove        bool      `json:"canApprove"`
	CanReject         bool      `json:"canReject"`
	CanRequestChanges bool      `json:"canRequestChanges,omitempty"`
	ReworkTaskID      string    `json:"reworkTaskId,omitempty"`
	ReworkStatus      string    `json:"reworkStatus,omitempty"`
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
	Feedback string                `json:"feedback"`
	HeadSHA  string                `json:"headSha,omitempty"`
	Comments *[]ReviewCommentInput `json:"comments,omitempty"`
}

// ReviewCommentInput is one line-anchored comment from Queen Console.
type ReviewCommentInput struct {
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Body      string `json:"body"`
}

// RejectTaskResponse is returned after rejecting a review-gated task.
type RejectTaskResponse struct {
	TraceID      string `json:"traceId"`
	TaskID       string `json:"taskId"`
	ReworkTaskID string `json:"reworkTaskId,omitempty"`
	Message      string `json:"message,omitempty"`
}

// ListReviewQueue returns tasks awaiting human review across recent traces.
func ListReviewQueue(ctx colony.Context) (ReviewQueueView, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, hiveview.TaskBoardTraceLimit)
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
			item := hiveview.TaskItemFromSnapshot(ctx, trace.TraceID, snap, task)
			qi := reviewQueueItemFromTask(item)
			qi.Summary = task.Summary
			if taskledger.IsFinalReviewTask(task) {
				if traceSummary, err := runs.ResolveTraceSummary(ctx.ColonyRoot, trace.TraceID); err == nil {
					qi.TraceSummary = traceSummary
				}
			}
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
	if session.Publisher == nil || session.Ledger == nil {
		return ApproveTaskResponse{}, fmt.Errorf("nats url not configured")
	}

	approveRes, err := review.Approve(ctx, colonyCtx, session.Publisher, session.Ledger, review.ApproveInput{
		TraceID:      traceID,
		TaskID:       taskID,
		Summary:      req.Summary,
		MergeMessage: req.MergeMessage,
		AgentID:      "console",
	}, review.WriteOptions{})
	if err != nil {
		return ApproveTaskResponse{}, err
	}

	snap, _, err := tasks.LoadTrace(colonyCtx, session.Ledger, traceID)
	if err != nil {
		return ApproveTaskResponse{}, err
	}
	task := snap.Tasks[taskID]

	msg := review.ApproveMessage(review.ApproveMessageOptions{
		ProposalWorkspace: task.ProposalWorkspace,
		CommitSHA:         approveRes.CommitSHA,
		StashOutcome:      approveRes.StashOutcome,
	})
	return ApproveTaskResponse{
		TraceID:   traceID,
		TaskID:    taskID,
		CommitSHA: approveRes.CommitSHA,
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

	snap, err := validateReviewTaskTarget(colonyCtx, session, traceID, taskID)
	if err != nil {
		return RejectTaskResponse{}, err
	}
	task := snap.Tasks[taskID]
	if session.Publisher == nil || session.Ledger == nil {
		return RejectTaskResponse{}, fmt.Errorf("nats url not configured")
	}

	isFinal := taskledger.IsFinalReviewTask(task)
	if req.Comments != nil {
		packet := review.CommentsPacket{
			HeadSHA:  req.HeadSHA,
			Summary:  req.Feedback,
			Comments: reviewCommentsFromInput(*req.Comments),
		}
		res, err := review.SubmitAnnotatedReview(ctx, session.Publisher, colonyCtx.ColonyRoot, session.Ledger, review.AnnotatedReviewInput{
			TraceID:  traceID,
			TaskID:   taskID,
			AgentID:  "console",
			Producer: artifacts.ProducerConsole,
			Packet:   packet,
		})
		if err != nil {
			return RejectTaskResponse{}, err
		}
		return RejectTaskResponse{
			TraceID:      traceID,
			TaskID:       taskID,
			ReworkTaskID: res.ReworkTaskID,
			Message:      review.RejectResponseMessage(isFinal, true, res.ReworkTaskID),
		}, nil
	}

	if err := review.Reject(ctx, session.Publisher, session.Ledger, review.RejectInput{
		TraceID:  traceID,
		TaskID:   taskID,
		Feedback: req.Feedback,
		AgentID:  "console",
	}); err != nil {
		return RejectTaskResponse{}, err
	}

	return RejectTaskResponse{
		TraceID: traceID,
		TaskID:  taskID,
		Message: review.RejectResponseMessage(isFinal, false, ""),
	}, nil
}

func reviewCommentsFromInput(in []ReviewCommentInput) []review.ReviewComment {
	out := make([]review.ReviewComment, 0, len(in))
	for _, c := range in {
		out = append(out, review.ReviewComment{
			Path:      c.Path,
			Side:      c.Side,
			StartLine: c.StartLine,
			EndLine:   c.EndLine,
			Snippet:   c.Snippet,
			Body:      c.Body,
		})
	}
	return out
}

func reviewQueueItemFromTask(item hiveview.TaskListItem) ReviewQueueItem {
	return ReviewQueueItem{
		TraceID:           item.TraceID,
		TaskID:            item.TaskID,
		Title:             item.Title,
		Review:            item.Review,
		Bee:               item.Bee,
		Sector:            item.Sector,
		RunCount:          item.RunCount,
		UpdatedAt:         item.UpdatedAt,
		IsFinal:           item.IsFinal,
		ProposalWorkspace: item.ProposalWorkspace,
		CanApprove:        item.CanApprove,
		CanReject:         item.CanReject,
		CanRequestChanges: item.CanRequestChanges,
		ReworkTaskID:      item.ReworkTaskID,
		ReworkStatus:      item.ReworkStatus,
	}
}

func validateReviewTaskTarget(colonyCtx colony.Context, session *tasks.LedgerSession, traceID, taskID string) (taskledger.TraceSnapshot, error) {
	if traceID == "" || taskID == "" {
		return taskledger.TraceSnapshot{}, fmt.Errorf("trace and task id are required")
	}
	var ledger taskledger.Ledger
	if session != nil {
		ledger = session.Ledger
	}
	snap, _, err := tasks.LoadTrace(colonyCtx, ledger, traceID)
	if err != nil {
		return taskledger.TraceSnapshot{}, err
	}
	task, ok := snap.Tasks[taskID]
	if !ok {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task %q not found in trace %s", taskID, traceID)
	}
	if task.Status != protocol.TaskStatusWaitingReview {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task %q is %q, expected waiting_review", taskID, task.Status)
	}
	if !taskledger.IsReviewGate(task) {
		return taskledger.TraceSnapshot{}, fmt.Errorf("task %q is not a review gate task", taskID)
	}
	return snap, nil
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
