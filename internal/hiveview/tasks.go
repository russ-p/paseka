package hiveview

import (
	"fmt"
	"sort"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// TaskBoardTraceLimit is the number of recent traces scanned for board/queue projections.
const TaskBoardTraceLimit = 30

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
	TraceID           string    `json:"traceId"`
	TaskID            string    `json:"taskId"`
	Title             string    `json:"title"`
	Status            string    `json:"status"`
	Review            string    `json:"review,omitempty"`
	Bee               string    `json:"bee,omitempty"`
	Sector            string    `json:"sector,omitempty"`
	DependsOn         []string  `json:"dependsOn,omitempty"`
	RunCount          int       `json:"runCount"`
	CanStart          bool      `json:"canStart"`
	CanRetry          bool      `json:"canRetry"`
	CanApprove        bool      `json:"canApprove"`
	CanReject         bool      `json:"canReject"`
	IsFinal           bool      `json:"isFinal"`
	ProposalWorkspace string    `json:"proposalWorkspace,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
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
	Body         string        `json:"body,omitempty"`
	Intent       string        `json:"intent,omitempty"`
	Summary      string        `json:"summary,omitempty"`
	TraceSummary string        `json:"traceSummary,omitempty"`
	Commit       string        `json:"commit,omitempty"`
	Runs         []TaskRunView `json:"runs"`
	Source       string        `json:"source"`
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

	item := TaskItemFromSnapshot(ctx, traceID, snap, task)
	view := TaskDetailView{
		TaskListItem: item,
		Body:         task.Body,
		Intent:       task.Intent,
		Summary:      task.Summary,
		Commit:       task.Commit,
		Source:       string(source),
	}
	if taskledger.IsFinalReviewTask(task) {
		if traceSummary, err := runs.ResolveTraceSummary(ctx.ColonyRoot, traceID); err == nil {
			view.TraceSummary = traceSummary
		}
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

// TaskItemFromSnapshot builds one task list row from ledger state.
func TaskItemFromSnapshot(ctx colony.Context, traceID string, snap taskledger.TraceSnapshot, task taskledger.TaskSnapshot) TaskListItem {
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
	proposalWorkspace := string(task.ProposalWorkspace)
	return TaskListItem{
		TraceID:           traceID,
		TaskID:            task.TaskID,
		Title:             title,
		Status:            status,
		Review:            reviewPolicy,
		Bee:               task.Bee,
		Sector:            task.Sector,
		DependsOn:         append([]string(nil), task.DependsOn...),
		RunCount:          runCount,
		CanStart:          tasks.CanStartTask(snap, task.TaskID),
		CanRetry:          tasks.CanRetryTask(snap, task.TaskID),
		CanApprove:        canApprove,
		CanReject:         canReject,
		IsFinal:           taskledger.IsFinalReviewTask(task),
		ProposalWorkspace: proposalWorkspace,
		UpdatedAt:         task.UpdatedAt,
	}
}

func collectRecentTaskItems(ctx colony.Context) ([]TaskListItem, error) {
	traceSummaries, err := runs.ScanRecentTraces(ctx.ColonyRoot, TaskBoardTraceLimit)
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
		out = append(out, TaskItemFromSnapshot(ctx, traceID, snap, snap.Tasks[id]))
	}
	return out
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
