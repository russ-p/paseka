package console

import (
	"context"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// CreateTaskRequest is the JSON body for POST /api/tasks.
type CreateTaskRequest struct {
	TraceID   string   `json:"traceId"`
	TaskID    string   `json:"taskId"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Bee       string   `json:"bee"`
	Sector    string   `json:"sector"`
	Intent    string   `json:"intent"`
	DependsOn []string `json:"dependsOn"`
	Review    string   `json:"review"`
	Autorun   bool     `json:"autorun"`
}

// CreateTaskResponse is returned after creating a task.
type CreateTaskResponse struct {
	TraceID string `json:"traceId"`
	TaskID  string `json:"taskId"`
	Bee     string `json:"bee"`
	Autorun bool   `json:"autorun"`
	Message string `json:"message,omitempty"`
}

// StartTaskResponse is returned after publishing task.ready.
type StartTaskResponse struct {
	TraceID string   `json:"traceId"`
	TaskIDs []string `json:"taskIds"`
	Message string   `json:"message,omitempty"`
}

// RetryTaskResponse is returned after re-publishing task.ready for a failed task.
type RetryTaskResponse struct {
	TraceID string `json:"traceId"`
	TaskID  string `json:"taskId"`
	Message string `json:"message,omitempty"`
}

// CreateTask publishes task.plan (and optionally task.ready) from the console.
func CreateTask(ctx context.Context, colonyCtx colony.Context, req CreateTaskRequest) (CreateTaskResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return CreateTaskResponse{}, err
	}
	defer session.Close()

	res, err := tasks.Create(ctx, session, tasks.CreateInput{
		TraceID:   req.TraceID,
		TaskID:    req.TaskID,
		Title:     req.Title,
		Body:      req.Body,
		Bee:       req.Bee,
		Sector:    req.Sector,
		Intent:    req.Intent,
		DependsOn: req.DependsOn,
		Review:    req.Review,
		Autorun:   req.Autorun,
		AgentID:   "console",
	})
	if err != nil {
		return CreateTaskResponse{}, err
	}

	msg := "Task created. Ensure paseka run is active to process queued tasks."
	if res.Autorun {
		msg = "Task created and task.ready published. Ensure paseka run is active to dispatch the task."
	}
	return CreateTaskResponse{
		TraceID: res.TraceID,
		TaskID:  res.TaskID,
		Bee:     res.Bee,
		Autorun: res.Autorun,
		Message: msg,
	}, nil
}

// StartTask publishes task.ready for an eligible task.
func StartTask(ctx context.Context, colonyCtx colony.Context, traceID, taskID string) (StartTaskResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return StartTaskResponse{}, err
	}
	defer session.Close()

	started, err := tasks.Start(ctx, session, traceID, taskID, "console")
	if err != nil {
		return StartTaskResponse{}, err
	}
	ids := make([]string, 0, len(started))
	for _, task := range started {
		ids = append(ids, task.TaskID)
	}
	return StartTaskResponse{
		TraceID: traceID,
		TaskIDs: ids,
		Message: "Published task.ready. Ensure paseka run is active to dispatch queued tasks.",
	}, nil
}

// RetryTask re-publishes task.ready for a failed or stuck running task.
func RetryTask(ctx context.Context, colonyCtx colony.Context, traceID, taskID string) (RetryTaskResponse, error) {
	session, err := tasks.OpenLedger(colonyCtx)
	if err != nil {
		return RetryTaskResponse{}, err
	}
	defer session.Close()

	task, err := tasks.Retry(ctx, session, traceID, taskID, "console")
	if err != nil {
		return RetryTaskResponse{}, err
	}
	return RetryTaskResponse{
		TraceID: traceID,
		TaskID:  task.TaskID,
		Message: "Published task.ready for retry. Ensure paseka run is active to dispatch queued tasks.",
	}, nil
}

func mapTaskError(err error) string {
	if err == nil {
		return ""
	}
	switch err {
	case taskledger.ErrTaskNotFound:
		return "task not found"
	case taskledger.ErrTaskAlreadyReady:
		return "task is already ready"
	case taskledger.ErrTaskCompleted:
		return "task is already completed"
	case taskledger.ErrTaskNotEligible:
		return "task is not eligible to start"
	case taskledger.ErrDependenciesIncomplete:
		return "task dependencies are not completed"
	case taskledger.ErrNoEligibleTasks:
		return "no eligible tasks to start"
	case taskledger.ErrTaskNotRetryable:
		return "task is not eligible to retry"
	default:
		return err.Error()
	}
}

func isTaskClientError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
		return true
	}
	switch err {
	case taskledger.ErrTaskNotFound,
		taskledger.ErrTaskAlreadyReady,
		taskledger.ErrTaskCompleted,
		taskledger.ErrTaskNotEligible,
		taskledger.ErrDependenciesIncomplete,
		taskledger.ErrNoEligibleTasks,
		taskledger.ErrTaskNotRetryable:
		return true
	default:
		return strings.Contains(msg, "already ready")
	}
}
