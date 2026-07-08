package console

import (
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
)

const dashboardTraceLimit = 10

// NATSStatusView summarizes NATS connectivity for the dashboard.
type NATSStatusView struct {
	Configured bool     `json:"configured"`
	Connected  bool     `json:"connected"`
	OK         bool     `json:"ok"`
	URL        string   `json:"url,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// DashboardView is the colony-wide snapshot for the Queen Console dashboard.
type DashboardView struct {
	Runtime         RuntimeView        `json:"runtime"`
	NATS            NATSStatusView     `json:"nats"`
	ActiveSessions  int                `json:"activeSessions"`
	ActiveWorktrees int                `json:"activeWorktrees"`
	TaskCounts      map[string]int     `json:"taskCounts"`
	RecentTraces    []TraceSummaryView `json:"recentTraces"`
	FailedRuns      []RunView          `json:"failedRuns"`
	RecentInsights  []InsightHighlight `json:"recentInsights"`
}

// GetDashboard aggregates colony-wide observability data.
func GetDashboard(ctx colony.Context, sup *runtime.Supervisor, mgr *sessions.Manager) (DashboardView, error) {
	if sup == nil {
		sup = runtime.DefaultSupervisor()
	}
	if mgr == nil {
		mgr = sessions.NewManager()
	}

	view := DashboardView{
		TaskCounts: map[string]int{},
	}

	rt, err := GetRuntime(ctx, sup)
	if err != nil {
		return DashboardView{}, err
	}
	view.Runtime = rt

	nats, err := diagnoseNATS(ctx)
	if err != nil {
		return DashboardView{}, err
	}
	view.NATS = nats

	st, err := colony.LoadState(ctx.Slug)
	if err != nil {
		return DashboardView{}, err
	}
	view.ActiveWorktrees = len(st.Worktrees)

	sessionsList, err := ListSessions(ctx, mgr)
	if err != nil {
		return DashboardView{}, err
	}
	for _, s := range sessionsList {
		if s.Active {
			view.ActiveSessions++
		}
	}

	traces, err := runs.ScanRecentTraces(ctx.ColonyRoot, dashboardTraceLimit*3)
	if err != nil {
		return DashboardView{}, err
	}
	for i, trace := range traces {
		if i >= dashboardTraceLimit {
			break
		}
		view.RecentTraces = append(view.RecentTraces, traceSummaryViewFromRuns(trace))
	}

	view.TaskCounts = aggregateTaskCounts(ctx, traces)

	runsList, err := ListRuns(ctx)
	if err != nil {
		return DashboardView{}, err
	}
	for _, run := range runsList {
		state := strings.ToLower(run.State)
		if state == string(protocol.StatusFailed) || state == string(protocol.StatusCancelled) {
			view.FailedRuns = append(view.FailedRuns, run)
		}
	}
	if len(view.FailedRuns) > dashboardTraceLimit {
		view.FailedRuns = view.FailedRuns[:dashboardTraceLimit]
	}

	insights, err := CollectRecentInsights(ctx, dashboardTraceLimit)
	if err != nil {
		return DashboardView{}, err
	}
	view.RecentInsights = insights

	return view, nil
}

func diagnoseNATS(ctx colony.Context) (NATSStatusView, error) {
	report, err := bus.Diagnose(ctx)
	if err != nil {
		return NATSStatusView{}, err
	}
	view := NATSStatusView{
		Configured: report.URL != "",
		Connected:  report.Connected,
		URL:        report.URL,
		Errors:     append([]string(nil), report.Errors...),
	}
	view.OK = view.Configured && report.Connected && len(report.Errors) == 0
	return view, nil
}

func aggregateTaskCounts(ctx colony.Context, traces []runs.TraceSummary) map[string]int {
	counts := map[string]int{}
	seen := map[string]struct{}{}
	for _, trace := range traces {
		if _, ok := seen[trace.TraceID]; ok {
			continue
		}
		seen[trace.TraceID] = struct{}{}
		snap, err := runs.LoadTraceTasksFromFS(ctx.ColonyRoot, trace.TraceID)
		if err != nil {
			continue
		}
		for _, task := range snap.Tasks {
			status := string(task.Status)
			if status == "" {
				status = string(protocol.TaskStatusPlanned)
			}
			counts[status]++
		}
	}
	return counts
}
