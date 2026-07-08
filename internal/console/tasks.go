package console

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

const taskBoardTraceLimit = 30

var taskStatusOrder = []string{
	string(protocol.TaskStatusReady),
	string(protocol.TaskStatusRunning),
	string(protocol.TaskStatusWaitingReview),
	string(protocol.TaskStatusPlanned),
	string(protocol.TaskStatusBlocked),
	string(protocol.TaskStatusFailed),
	string(protocol.TaskStatusCompleted),
}

// TaskRunView links a task to one agent run.
type TaskRunView struct {
	AgentID    string    `json:"agentId"`
	Bee        string    `json:"bee,omitempty"`
	RunDir     string    `json:"runDir,omitempty"`
	RunStatus  string    `json:"runStatus,omitempty"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

// TaskListItem is one task row on the board.
type TaskListItem struct {
	TraceID    string    `json:"traceId"`
	TaskID     string    `json:"taskId"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Review     string    `json:"review,omitempty"`
	Bee        string    `json:"bee,omitempty"`
	Sector     string    `json:"sector,omitempty"`
	DependsOn  []string  `json:"dependsOn,omitempty"`
	RunCount   int       `json:"runCount"`
	CanStart   bool      `json:"canStart"`
	CanApprove bool      `json:"canApprove"`
	CanReject  bool      `json:"canReject"`
	IsFinal    bool      `json:"isFinal"`
	UpdatedAt  time.Time `json:"updatedAt,omitempty"`
}

// TaskStatusGroup groups tasks by lifecycle status.
type TaskStatusGroup struct {
	Status string         `json:"status"`
	Tasks  []TaskListItem `json:"tasks"`
}

// TaskBoardView is the colony-wide task board projection.
type TaskBoardView struct {
	Groups     []TaskStatusGroup `json:"groups"`
	TaskCounts map[string]int    `json:"taskCounts"`
}

// TaskDetailView is a full task inspection view.
type TaskDetailView struct {
	TaskListItem
	Body    string        `json:"body,omitempty"`
	Intent  string        `json:"intent,omitempty"`
	Summary string        `json:"summary,omitempty"`
	Commit  string        `json:"commit,omitempty"`
	Runs    []TaskRunView `json:"runs"`
	Source  string        `json:"source"`
}

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

// ListTaskBoard returns tasks grouped by status across recent traces.
func ListTaskBoard(ctx colony.Context) (TaskBoardView, error) {
	items, err := collectRecentTaskItems(ctx)
	if err != nil {
		return TaskBoardView{}, err
	}
	return buildTaskBoard(items), nil
}

// ListTraceTasks returns tasks for one trace.
func ListTraceTasks(ctx colony.Context, traceID string) ([]TaskListItem, error) {
	if traceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}
	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	snap, _, err := tasks.LoadTrace(ctx, session.Ledger, traceID)
	if err != nil {
		return nil, err
	}
	return taskItemsFromSnapshot(ctx, traceID, snap), nil
}

// GetTask returns one task detail view.
func GetTask(ctx colony.Context, traceID, taskID string) (TaskDetailView, bool, error) {
	if traceID == "" || taskID == "" {
		return TaskDetailView{}, false, nil
	}
	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	defer session.Close()

	snap, source, err := tasks.LoadTrace(ctx, session.Ledger, traceID)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	task, ok := snap.Tasks[taskID]
	if !ok {
		return TaskDetailView{}, false, nil
	}

	item := taskItemFromSnapshot(ctx, traceID, snap, task)
	view := TaskDetailView{
		TaskListItem: item,
		Body:         task.Body,
		Intent:       task.Intent,
		Summary:      task.Summary,
		Commit:       task.Commit,
		Source:       string(source),
	}

	taskDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, taskID)
	if err != nil {
		return TaskDetailView{}, false, err
	}
	runEntries, err := taskDir.ReadTaskRuns()
	if err != nil {
		return TaskDetailView{}, false, err
	}
	for _, entry := range runEntries {
		view.Runs = append(view.Runs, TaskRunView{
			AgentID:    entry.AgentID,
			Bee:        entry.Bee,
			RunDir:     entry.RunDir,
			RunStatus:  entry.RunStatus,
			StartedAt:  entry.StartedAt,
			FinishedAt: entry.FinishedAt,
		})
	}
	return view, true, nil
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

func collectRecentTaskItems(ctx colony.Context) ([]TaskListItem, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, taskBoardTraceLimit)
	if err != nil {
		return nil, err
	}

	session, err := tasks.OpenLedger(ctx)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var items []TaskListItem
	for _, trace := range traceSummaries {
		snap, _, err := tasks.LoadTrace(ctx, session.Ledger, trace.TraceID)
		if err != nil {
			continue
		}
		items = append(items, taskItemsFromSnapshot(ctx, trace.TraceID, snap)...)
	}
	return items, nil
}

func buildTaskBoard(items []TaskListItem) TaskBoardView {
	counts := map[string]int{}
	byStatus := map[string][]TaskListItem{}
	for _, item := range items {
		status := item.Status
		if status == "" {
			status = string(protocol.TaskStatusPlanned)
		}
		counts[status]++
		byStatus[status] = append(byStatus[status], item)
	}

	view := TaskBoardView{TaskCounts: counts}
	for _, status := range taskStatusOrder {
		groupItems := byStatus[status]
		if len(groupItems) == 0 {
			continue
		}
		sort.Slice(groupItems, func(i, j int) bool {
			if !groupItems[i].UpdatedAt.Equal(groupItems[j].UpdatedAt) {
				return groupItems[i].UpdatedAt.After(groupItems[j].UpdatedAt)
			}
			return groupItems[i].TaskID < groupItems[j].TaskID
		})
		view.Groups = append(view.Groups, TaskStatusGroup{
			Status: status,
			Tasks:  groupItems,
		})
		delete(byStatus, status)
	}
	remaining := make([]string, 0, len(byStatus))
	for status := range byStatus {
		remaining = append(remaining, status)
	}
	sort.Strings(remaining)
	for _, status := range remaining {
		groupItems := byStatus[status]
		sort.Slice(groupItems, func(i, j int) bool {
			return groupItems[i].TaskID < groupItems[j].TaskID
		})
		view.Groups = append(view.Groups, TaskStatusGroup{
			Status: status,
			Tasks:  groupItems,
		})
	}
	return view
}

func taskItemsFromSnapshot(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot) []TaskListItem {
	ids := make([]string, 0, len(snap.Tasks))
	for id := range snap.Tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]TaskListItem, 0, len(ids))
	for _, id := range ids {
		out = append(out, taskItemFromSnapshot(ctx, traceID, snap, snap.Tasks[id]))
	}
	return out
}

func taskItemFromSnapshot(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot, task taskledger.TaskSnapshot) TaskListItem {
	title := task.Title
	if title == "" {
		title = task.TaskID
	}
	status := string(task.Status)
	if status == "" {
		status = string(protocol.TaskStatusPlanned)
	}

	runCount := 0
	if taskDir, err := runs.NewTaskDir(ctx.ColonyRoot, traceID, task.TaskID); err == nil {
		if entries, err := taskDir.ReadTaskRuns(); err == nil {
			runCount = len(entries)
		}
	}

	reviewPolicy := string(protocol.NormalizeTaskReviewPolicy(task.Review))
	canApprove, canReject := reviewActionsForTask(task)
	return TaskListItem{
		TraceID:    traceID,
		TaskID:     task.TaskID,
		Title:      title,
		Status:     status,
		Review:     reviewPolicy,
		Bee:        task.Bee,
		Sector:     task.Sector,
		DependsOn:  append([]string(nil), task.DependsOn...),
		RunCount:   runCount,
		CanStart:   tasks.CanStartTask(snap, task.TaskID),
		CanApprove: canApprove,
		CanReject:  canReject,
		IsFinal:    taskledger.IsFinalReviewTask(task),
		UpdatedAt:  task.UpdatedAt,
	}
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
		taskledger.ErrNoEligibleTasks:
		return true
	default:
		return strings.Contains(msg, "already ready")
	}
}
