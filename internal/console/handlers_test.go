package console_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/worktree"
)

func TestRuntimeAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	var child *exec.Cmd
	sup := &runtime.Supervisor{
		Spawn: func(exe, colonyRoot string) (int, error) {
			child = exec.Command("sleep", "300")
			if err := child.Start(); err != nil {
				return 0, err
			}
			go func() { _ = child.Wait() }()
			pid := child.Process.Pid
			if err := colony.RegisterRuntime(ctxColony.Slug, colony.RuntimeEntry{
				PID:        pid,
				StartedAt:  time.Now().UTC(),
				ColonyRoot: colonyRoot,
				Status:     runtime.RuntimeStatusRunning,
			}); err != nil {
				_ = child.Process.Kill()
				return 0, err
			}
			return pid, nil
		},
	}
	t.Cleanup(func() {
		_ = colony.ClearRuntime(ctxColony.Slug)
		if child != nil && child.Process != nil {
			_ = child.Process.Kill()
		}
	})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
		Runtime:  sup,
	})

	statusReq := httptest.NewRequest(http.MethodGet, "/api/runtime", nil)
	statusRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", statusRec.Code, statusRec.Body.String())
	}
	var stopped console.RuntimeView
	if err := json.NewDecoder(statusRec.Body).Decode(&stopped); err != nil {
		t.Fatal(err)
	}
	if stopped.Status != runtime.RuntimeStatusStopped {
		t.Fatalf("stopped view = %+v", stopped)
	}
	if stopped.Slug != ctxColony.Slug || stopped.ColonyRoot != ctxColony.ColonyRoot {
		t.Fatalf("stopped identity = slug=%q root=%q want slug=%q root=%q", stopped.Slug, stopped.ColonyRoot, ctxColony.Slug, ctxColony.ColonyRoot)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/runtime/start", nil)
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	var started console.RuntimeView
	if err := json.NewDecoder(startRec.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	if started.Status != runtime.RuntimeStatusRunning || !started.Alive {
		t.Fatalf("started view = %+v", started)
	}

	startAgainRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startAgainRec, startReq)
	if startAgainRec.Code != http.StatusOK {
		t.Fatalf("start again status = %d body=%s", startAgainRec.Code, startAgainRec.Body.String())
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/api/runtime/stop", nil)
	stopRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
	var stoppedAgain console.RuntimeView
	if err := json.NewDecoder(stopRec.Body).Decode(&stoppedAgain); err != nil {
		t.Fatal(err)
	}
	if stoppedAgain.Status != runtime.RuntimeStatusStopped {
		t.Fatalf("stopped again view = %+v", stoppedAgain)
	}
}

func TestConsoleAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &outputSessionAdapter{})

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: mgr,
	})

	beesReq := httptest.NewRequest(http.MethodGet, "/api/bees", nil)
	beesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(beesRec, beesReq)
	if beesRec.Code != http.StatusOK {
		t.Fatalf("bees status = %d body=%s", beesRec.Code, beesRec.Body.String())
	}
	var bees []console.BeeView
	if err := json.NewDecoder(beesRec.Body).Decode(&bees); err != nil {
		t.Fatal(err)
	}
	if len(bees) != 1 || bees[0].Role != "scout" {
		t.Fatalf("bees = %+v", bees)
	}

	createBody := `{"bee":"scout","body":"console hello"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/sessions", bytes.NewBufferString(createBody))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created console.SessionView
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.SessionID == "" || !created.Active {
		t.Fatalf("created = %+v", created)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		d := runs.Dir{ColonyRoot: repo, TraceID: created.TraceID, AgentID: created.AgentID}
		meta, err := d.ReadSession()
		if err == nil && meta.State == string(adapters.SessionCompleted) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list []console.SessionView
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) == 0 {
		t.Fatal("expected sessions in list")
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.SessionID, nil)
	detailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailRec.Code, detailRec.Body.String())
	}

	transcriptReq := httptest.NewRequest(http.MethodGet, "/api/sessions/"+created.SessionID+"/transcript?after=0", nil)
	transcriptRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(transcriptRec, transcriptReq)
	if transcriptRec.Code != http.StatusOK {
		t.Fatalf("transcript status = %d body=%s", transcriptRec.Code, transcriptRec.Body.String())
	}
	var page console.TranscriptPage
	if err := json.NewDecoder(transcriptRec.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) == 0 {
		t.Fatal("expected transcript entries")
	}

	d := runs.Dir{ColonyRoot: repo, TraceID: created.TraceID, AgentID: created.AgentID}
	if _, err := os.Stat(d.ResultPath()); err != nil {
		t.Fatalf("expected summary.md: %v", err)
	}
}

func TestListSessionsProjection(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC().Add(-30 * time.Minute)
	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-hist", AgentID: "agent-hist"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	finished := started.Add(5 * time.Minute)
	if err := d.WriteSession(runs.SessionMeta{
		SessionID:  "agent-hist",
		TraceID:    "trace-hist",
		AgentID:    "agent-hist",
		Bee:        "scout",
		Adapter:    "cursor",
		Workspace:  repo,
		ColonyRoot: repo,
		State:      string(adapters.SessionCompleted),
		StartedAt:  started,
		FinishedAt: finished,
	}); err != nil {
		t.Fatal(err)
	}

	list, err := console.ListSessions(ctxColony, sessions.NewManager())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].SessionID != "agent-hist" {
		t.Fatalf("list = %+v", list)
	}
}

func TestRunsAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC().Add(-20 * time.Minute)
	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-run", AgentID: "agent-run"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-run",
		AgentID:         "agent-run",
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		TaskID:          "task-1",
		Task:            "headless task",
		Intent:          "feature",
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      started.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-run",
		AgentID:         "agent-run",
		Status:          protocol.StatusCompleted,
		Summary:         "run done",
		Usage: &protocol.Usage{
			InputTokens:      8848,
			OutputTokens:     56,
			CacheReadTokens:  5472,
			CacheWriteTokens: 0,
			DurationMs:       2453,
			Source:           protocol.UsageSourceCursorStreamJSON,
		},
		FinishedAt: started.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent("trace-run", "agent-run", 0, protocol.EventLog, map[string]string{"line": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/runs", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var list []console.RunView
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AgentID != "agent-run" {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Summary != "run done" || !list[0].HasEvents {
		t.Fatalf("list item = %+v", list[0])
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/runs/trace-run/agent-run", nil)
	detailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail console.RunView
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Body != "headless task" || detail.Intent != "feature" {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Usage == nil || detail.Usage.InputTokens != 8848 || detail.Usage.OutputTokens != 56 {
		t.Fatalf("detail.Usage = %+v", detail.Usage)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/runs/trace-run/agent-run/events?after=0", nil)
	eventsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var page console.EventsPage
	if err := json.NewDecoder(eventsRec.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || page.NextCursor != 1 {
		t.Fatalf("events page = %+v", page)
	}
}

func TestDashboardAndTimelineAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC().Add(-10 * time.Minute)
	traceID := "trace-dash"
	agentID := "agent-dash"

	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		TaskID:          "task-dash",
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusFailed,
		StartedAt:       started,
		FinishedAt:      started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	insight, err := protocol.NewEvent(traceID, agentID, 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "dashboard summary",
		TaskID:  "task-dash",
	})
	if err != nil {
		t.Fatal(err)
	}
	insight.CreatedAt = started.Add(30 * time.Second)
	if err := d.AppendEvent(insight); err != nil {
		t.Fatal(err)
	}

	signal, err := protocol.NewEvent(traceID, agentID, 2, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-dash",
		Title:  "Do work",
	})
	if err != nil {
		t.Fatal(err)
	}
	signal.CreatedAt = started.Add(45 * time.Second)
	if err := d.AppendEvent(signal); err != nil {
		t.Fatal(err)
	}

	titleEv, err := protocol.NewEvent(traceID, agentID, 3, protocol.EventInsight, protocol.TraceTitlePayload{
		Kind:  protocol.InsightTraceTitle,
		Title: "Dashboard trail title",
	})
	if err != nil {
		t.Fatal(err)
	}
	titleEv.CreatedAt = started.Add(50 * time.Second)
	if err := d.AppendEvent(titleEv); err != nil {
		t.Fatal(err)
	}

	taskDir, err := runs.NewTaskDir(repo, traceID, "task-dash")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-dash",
		Title:   "Do work",
		Bee:     "scout",
		Status:  protocol.TaskStatusReady,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	dashReq := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	dashRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dashRec, dashReq)
	if dashRec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", dashRec.Code, dashRec.Body.String())
	}
	var dash console.DashboardView
	if err := json.NewDecoder(dashRec.Body).Decode(&dash); err != nil {
		t.Fatal(err)
	}
	if len(dash.RecentTraces) == 0 {
		t.Fatalf("expected recent traces, got %+v", dash)
	}
	if dash.RecentTraces[0].Title != "Dashboard trail title" {
		t.Fatalf("dashboard trace title = %q", dash.RecentTraces[0].Title)
	}
	if len(dash.FailedRuns) == 0 {
		t.Fatalf("expected failed runs, got %+v", dash)
	}
	if len(dash.RecentInsights) == 0 {
		t.Fatalf("expected recent insights, got %+v", dash)
	}
	if dash.TaskCounts[string(protocol.TaskStatusReady)] != 1 {
		t.Fatalf("task counts = %+v", dash.TaskCounts)
	}

	tracesReq := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	tracesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tracesRec, tracesReq)
	if tracesRec.Code != http.StatusOK {
		t.Fatalf("traces status = %d body=%s", tracesRec.Code, tracesRec.Body.String())
	}
	var traces []console.TraceSummaryView
	if err := json.NewDecoder(tracesRec.Body).Decode(&traces); err != nil {
		t.Fatal(err)
	}
	if len(traces) == 0 || traces[0].TraceID != traceID {
		t.Fatalf("traces = %+v", traces)
	}
	if traces[0].Title != "Dashboard trail title" {
		t.Fatalf("trace title = %q", traces[0].Title)
	}

	traceReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID, nil)
	traceRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(traceRec, traceReq)
	if traceRec.Code != http.StatusOK {
		t.Fatalf("trace detail status = %d body=%s", traceRec.Code, traceRec.Body.String())
	}
	var traceDetail console.TraceDetailView
	if err := json.NewDecoder(traceRec.Body).Decode(&traceDetail); err != nil {
		t.Fatal(err)
	}
	if traceDetail.TraceID != traceID || len(traceDetail.Tasks) != 1 {
		t.Fatalf("trace detail = %+v", traceDetail)
	}
	if traceDetail.Title != "Dashboard trail title" {
		t.Fatalf("trace detail title = %q", traceDetail.Title)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/events?type=SIGNAL&kind=task.ready", nil)
	eventsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status = %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var feed console.EventFeedPage
	if err := json.NewDecoder(eventsRec.Body).Decode(&feed); err != nil {
		t.Fatal(err)
	}
	if len(feed.Items) != 1 {
		t.Fatalf("feed = %+v", feed)
	}
	if feed.Items[0].PayloadKind != string(protocol.TaskEventReady) {
		t.Fatalf("feed item = %+v", feed.Items[0])
	}
	if feed.Items[0].Raw.TraceID != traceID {
		t.Fatalf("raw event missing: %+v", feed.Items[0])
	}

	traceEventsReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/events", nil)
	traceEventsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(traceEventsRec, traceEventsReq)
	if traceEventsRec.Code != http.StatusOK {
		t.Fatalf("trace events status = %d body=%s", traceEventsRec.Code, traceEventsRec.Body.String())
	}
	var traceFeed console.EventFeedPage
	if err := json.NewDecoder(traceEventsRec.Body).Decode(&traceFeed); err != nil {
		t.Fatal(err)
	}
	if len(traceFeed.Items) < 2 {
		t.Fatalf("trace feed = %+v", traceFeed)
	}
}

func TestTraceSummaryProjection(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC().Add(-8 * time.Minute)
	withSummary := "trace-with-summary"
	withoutSummary := "trace-without-summary"
	agentID := "agent-summary"

	for _, spec := range []struct {
		traceID string
		summary string
		title   string
	}{
		{withSummary, "Implemented OAuth callback and added focused tests", "Trail with summary"},
		{withoutSummary, "", "Trail without summary"},
	} {
		d := runs.Dir{ColonyRoot: repo, TraceID: spec.traceID, AgentID: agentID}
		if err := d.Prepare(); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteRequest(protocol.Request{
			ProtocolVersion: protocol.Version,
			TraceID:         spec.traceID,
			AgentID:         agentID,
			Bee:             "scout",
			Adapter:         "cursor",
			Workspace:       repo,
			ColonyRoot:      repo,
			CreatedAt:       started,
		}); err != nil {
			t.Fatal(err)
		}
		if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			State:           protocol.StatusCompleted,
			StartedAt:       started,
			FinishedAt:      started.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}

		titleEv, err := protocol.NewEvent(spec.traceID, agentID, 1, protocol.EventInsight, protocol.TraceTitlePayload{
			Kind:  protocol.InsightTraceTitle,
			Title: spec.title,
		})
		if err != nil {
			t.Fatal(err)
		}
		titleEv.CreatedAt = started.Add(10 * time.Second)
		if err := d.AppendEvent(titleEv); err != nil {
			t.Fatal(err)
		}

		if spec.summary != "" {
			summaryEv, err := protocol.NewEvent(spec.traceID, agentID, 2, protocol.EventInsight, protocol.TraceSummaryPayload{
				Kind:    protocol.InsightTraceSummary,
				Summary: spec.summary,
			})
			if err != nil {
				t.Fatal(err)
			}
			summaryEv.CreatedAt = started.Add(20 * time.Second)
			if err := d.AppendEvent(summaryEv); err != nil {
				t.Fatal(err)
			}
		}
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	dashReq := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	dashRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dashRec, dashReq)
	if dashRec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d body=%s", dashRec.Code, dashRec.Body.String())
	}
	var dash console.DashboardView
	if err := json.NewDecoder(dashRec.Body).Decode(&dash); err != nil {
		t.Fatal(err)
	}
	dashByID := map[string]console.TraceSummaryView{}
	for _, trace := range dash.RecentTraces {
		dashByID[trace.TraceID] = trace
	}
	if dashByID[withSummary].Summary != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("dashboard summary = %q", dashByID[withSummary].Summary)
	}
	if dashByID[withoutSummary].Summary != "" {
		t.Fatalf("dashboard summary without emit = %q", dashByID[withoutSummary].Summary)
	}

	tracesReq := httptest.NewRequest(http.MethodGet, "/api/traces", nil)
	tracesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tracesRec, tracesReq)
	if tracesRec.Code != http.StatusOK {
		t.Fatalf("traces status = %d body=%s", tracesRec.Code, tracesRec.Body.String())
	}
	var traces []console.TraceSummaryView
	if err := json.NewDecoder(tracesRec.Body).Decode(&traces); err != nil {
		t.Fatal(err)
	}
	listByID := map[string]console.TraceSummaryView{}
	for _, trace := range traces {
		listByID[trace.TraceID] = trace
	}
	if listByID[withSummary].Summary != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("list summary = %q", listByID[withSummary].Summary)
	}
	if listByID[withoutSummary].Summary != "" {
		t.Fatalf("list summary without emit = %q", listByID[withoutSummary].Summary)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+withSummary, nil)
	detailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail status = %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail console.TraceDetailView
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Summary != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("detail summary = %q", detail.Summary)
	}

	omitReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+withoutSummary, nil)
	omitRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(omitRec, omitReq)
	if omitRec.Code != http.StatusOK {
		t.Fatalf("omit detail status = %d body=%s", omitRec.Code, omitRec.Body.String())
	}
	var detailMap map[string]json.RawMessage
	if err := json.Unmarshal(omitRec.Body.Bytes(), &detailMap); err != nil {
		t.Fatal(err)
	}
	if _, ok := detailMap["summary"]; ok {
		t.Fatalf("expected summary omitted from JSON, got %s", omitRec.Body.String())
	}
}

func TestTraceDetailAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC().Add(-15 * time.Minute)
	traceID := "trace-detail"
	otherTrace := "trace-other"

	writeConsoleRun(t, repo, traceID, "agent-a", started, protocol.StatusCompleted, "first")
	writeConsoleRun(t, repo, traceID, "agent-b", started.Add(5*time.Minute), protocol.StatusFailed, "second")
	writeConsoleRun(t, repo, otherTrace, "agent-x", started.Add(time.Minute), protocol.StatusRunning, "")

	dA := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-a"}
	if err := dA.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-a",
		Status:          protocol.StatusCompleted,
		Summary:         "first",
		Usage: &protocol.Usage{
			InputTokens:  100,
			OutputTokens: 10,
			Source:       protocol.UsageSourceCursorStreamJSON,
		},
		FinishedAt: started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	dB := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-b"}
	if err := dB.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         "agent-b",
		Status:          protocol.StatusFailed,
		Summary:         "second",
		Usage: &protocol.Usage{
			InputTokens:  200,
			OutputTokens: 30,
			Source:       protocol.UsageSourceCursorStreamJSON,
		},
		FinishedAt: started.Add(6 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	ev1, err := protocol.NewEvent(traceID, "agent-a", 1, protocol.EventInsight, protocol.NarrativeInsightPayload{
		Kind:    protocol.InsightRunSummary,
		Summary: "first insight",
		TaskID:  "task-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev1.CreatedAt = started.Add(time.Minute)
	if err := dA.AppendEvent(ev1); err != nil {
		t.Fatal(err)
	}
	ev2, err := protocol.NewEvent(traceID, "agent-a", 2, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Title:  "Ship it",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev2.CreatedAt = started.Add(2 * time.Minute)
	if err := dA.AppendEvent(ev2); err != nil {
		t.Fatal(err)
	}

	taskDir, err := runs.NewTaskDir(repo, traceID, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-1",
		Title:   "Ship it",
		Bee:     "scout",
		Status:  protocol.TaskStatusReady,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	wtCreated := started.Add(3 * time.Minute)
	if err := colony.RegisterWorktree(ctxColony.Slug, colony.WorktreeEntry{
		TraceID:   traceID,
		Path:      filepath.Join(repo, ".paseka", "worktrees", traceID),
		BaseSHA:   "abc123",
		Branch:    "paseka/" + traceID,
		CreatedAt: wtCreated,
	}); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var detail console.TraceDetailView
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.TraceID != traceID {
		t.Fatalf("traceId = %q", detail.TraceID)
	}
	if len(detail.Tasks) != 1 || detail.Tasks[0].TaskID != "task-1" || detail.Tasks[0].Title != "Ship it" {
		t.Fatalf("tasks = %+v", detail.Tasks)
	}
	if detail.TaskCount != len(detail.Tasks) {
		t.Fatalf("TaskCount=%d len(Tasks)=%d", detail.TaskCount, len(detail.Tasks))
	}
	if len(detail.Runs) != 2 {
		t.Fatalf("runs = %+v", detail.Runs)
	}
	for _, run := range detail.Runs {
		if run.TraceID != traceID {
			t.Fatalf("unrelated run leaked: %+v", run)
		}
	}
	if detail.Runs[0].AgentID != "agent-b" || detail.Runs[1].AgentID != "agent-a" {
		t.Fatalf("run order = %+v", detail.Runs)
	}
	if detail.Usage == nil || detail.Usage.RunCountWithUsage != 2 {
		t.Fatalf("usage aggregate = %+v", detail.Usage)
	}
	if detail.Usage.InputTokens != 300 || detail.Usage.OutputTokens != 40 {
		t.Fatalf("usage totals = %+v", detail.Usage)
	}
	if detail.Runs[1].Usage == nil || detail.Runs[1].Usage.InputTokens != 100 {
		t.Fatalf("run usage = %+v", detail.Runs[1].Usage)
	}
	if detail.Worktree == nil || detail.Worktree.BaseSHA != "abc123" || detail.Worktree.Branch == "" {
		t.Fatalf("worktree = %+v", detail.Worktree)
	}
	if len(detail.RecentEvents) < 2 {
		t.Fatalf("recentEvents = %+v", detail.RecentEvents)
	}
	for _, item := range detail.RecentEvents {
		if item.TraceID != traceID {
			t.Fatalf("unrelated event leaked: %+v", item)
		}
	}
	if detail.RecentEvents[0].CreatedAt.Before(detail.RecentEvents[1].CreatedAt) {
		t.Fatalf("events not ordered newest-first: %+v", detail.RecentEvents)
	}
}

func TestTraceMergeDiffAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	traceID := "trace-merge-diff"

	entry, err := worktree.Ensure(worktree.EnsureOptions{
		ColonyRoot: repo,
		TraceID:    traceID,
		Slug:       ctxColony.Slug,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entry.Path, "feature.txt"), []byte("merge me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, entry.Path, "add", "feature.txt")
	runGit(t, entry.Path, "commit", "-m", "feature")

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/merge-diff", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.MergeDiffView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.TraceID != traceID {
		t.Fatalf("traceId = %q", view.TraceID)
	}
	if view.MissingWorktree {
		t.Fatal("expected worktree branch")
	}
	if view.Empty {
		t.Fatal("expected non-empty diff")
	}
	if !strings.Contains(view.Diff, "feature.txt") {
		t.Fatalf("diff = %q", view.Diff)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/traces/trace-no-branch/merge-diff", nil)
	missingRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusOK {
		t.Fatalf("missing status = %d body=%s", missingRec.Code, missingRec.Body.String())
	}
	var missing console.MergeDiffView
	if err := json.NewDecoder(missingRec.Body).Decode(&missing); err != nil {
		t.Fatal(err)
	}
	if !missing.MissingWorktree {
		t.Fatalf("expected missingWorktree, got %+v", missing)
	}
}

func TestEnergyAddAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)

	t.Run("validation", func(t *testing.T) {
		ctxColony := setupConsoleHome(t, repo)
		srv := console.NewServer(console.Options{
			Addr:     "127.0.0.1:0",
			Colony:   ctxColony,
			Sessions: sessions.NewManager(),
		})

		req := httptest.NewRequest(http.MethodPost, "/api/traces/trace-energy/energy/add", bytes.NewBufferString(`{"amount":0}`))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("validation status = %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "positive") {
			t.Fatalf("validation body = %q", rec.Body.String())
		}
	})

	t.Run("unavailable", func(t *testing.T) {
		ctxColony := setupConsoleHome(t, repo)
		srv := console.NewServer(console.Options{
			Addr:     "127.0.0.1:0",
			Colony:   ctxColony,
			Sessions: sessions.NewManager(),
		})

		req := httptest.NewRequest(http.MethodPost, "/api/traces/trace-energy/energy/add", bytes.NewBufferString(`{"amount":1}`))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("unavailable status = %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("happy", func(t *testing.T) {
		url := os.Getenv("PASEKA_NATS_URL")
		if url == "" {
			url = "nats://127.0.0.1:4222"
		}
		ctxColony := setupConsoleHomeWithNATS(t, repo, url)
		client, err := bus.ConnectColony(ctxColony, true)
		if err != nil {
			t.Skipf("nats unavailable: %v", err)
		}
		if client == nil {
			t.Skip("nats url not configured")
		}
		defer client.Close()

		traceID := "trace-console-energy-" + time.Now().Format("150405")
		kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(ctxColony.Slug))
		if err != nil {
			t.Fatalf("kv: %v", err)
		}
		ledger := taskledger.NewKVLedger(kv)
		if err := ledger.SeedEnergy(traceID, 10); err != nil {
			t.Fatal(err)
		}

		srv := console.NewServer(console.Options{
			Addr:     "127.0.0.1:0",
			Colony:   ctxColony,
			Sessions: sessions.NewManager(),
		})

		req := httptest.NewRequest(http.MethodPost, "/api/traces/"+traceID+"/energy/add", bytes.NewBufferString(`{"amount":5}`))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("add status = %d body=%s", rec.Code, rec.Body.String())
		}
		var res console.EnergyAddResponse
		if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
			t.Fatal(err)
		}
		if res.TraceID != traceID || res.Amount != 5 {
			t.Fatalf("response = %+v", res)
		}
		if res.EnergyRemaining != 15 || res.EnergyBudget != 10 {
			t.Fatalf("energy = remaining %d budget %d, want 15/10", res.EnergyRemaining, res.EnergyBudget)
		}

		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			t.Fatal(err)
		}
		if snap.EnergyRemaining != 15 {
			t.Fatalf("ledger remaining = %d, want 15", snap.EnergyRemaining)
		}
	})
}

func TestTraceDetailFallsBackWhenNATSUnavailable(t *testing.T) {
	repo := initConsoleRepo(t)
	// Point at a closed local port so Connect fails instead of hanging.
	ctxColony := setupConsoleHomeWithNATS(t, repo, "nats://127.0.0.1:1")

	started := time.Now().UTC().Add(-5 * time.Minute)
	traceID := "trace-offline"
	writeConsoleRun(t, repo, traceID, "agent-1", started, protocol.StatusCompleted, "ok")

	taskDir, err := runs.NewTaskDir(repo, traceID, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := taskDir.WriteTask(runs.TaskFrontmatter{
		TraceID: traceID,
		TaskID:  "task-1",
		Title:   "Offline task",
		Bee:     "scout",
		Status:  protocol.TaskStatusReady,
	}, "body"); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var detail console.TraceDetailView
	if err := json.NewDecoder(rec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.TraceID != traceID {
		t.Fatalf("traceId = %q", detail.TraceID)
	}
	if len(detail.Tasks) != 1 || detail.Tasks[0].TaskID != "task-1" {
		t.Fatalf("expected filesystem task fallback, got %+v", detail.Tasks)
	}
	if len(detail.Runs) != 1 {
		t.Fatalf("runs = %+v", detail.Runs)
	}
}

func writeConsoleRun(t *testing.T, root, traceID, agentID string, started time.Time, state protocol.RunStatus, summary string) {
	t.Helper()
	d := runs.Dir{ColonyRoot: root, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       root,
		ColonyRoot:      root,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	snap := protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           state,
		StartedAt:       started,
	}
	if state == protocol.StatusCompleted || state == protocol.StatusFailed {
		snap.FinishedAt = started.Add(time.Minute)
	}
	if err := d.WriteStatusSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	if summary != "" {
		if err := d.WriteResult(protocol.Result{
			ProtocolVersion: protocol.Version,
			TraceID:         traceID,
			AgentID:         agentID,
			Status:          state,
			Summary:         summary,
			FinishedAt:      started.Add(time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTasksAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	traceID := "trace-tasks"
	plannedID := "task-planned"
	readyID := "task-ready"

	for _, spec := range []struct {
		taskID string
		title  string
		status protocol.TaskStatus
	}{
		{plannedID, "Planned work", protocol.TaskStatusPlanned},
		{readyID, "Ready work", protocol.TaskStatusReady},
	} {
		taskDir, err := runs.NewTaskDir(repo, traceID, spec.taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := taskDir.WriteTask(runs.TaskFrontmatter{
			TraceID: traceID,
			TaskID:  spec.taskID,
			Title:   spec.title,
			Bee:     "scout",
			Status:  spec.status,
		}, "task body"); err != nil {
			t.Fatal(err)
		}
	}

	readyDir, err := runs.NewTaskDir(repo, traceID, readyID)
	if err != nil {
		t.Fatal(err)
	}
	if err := readyDir.AppendTaskRun(runs.TaskRunEntry{
		AgentID:   "agent-task",
		Bee:       "scout",
		RunDir:    filepath.Join(repo, ".paseka", "runs", traceID, "agent-task"),
		RunStatus: string(protocol.StatusCompleted),
		StartedAt: time.Now().UTC().Add(-5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	boardReq := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	boardRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(boardRec, boardReq)
	if boardRec.Code != http.StatusOK {
		t.Fatalf("board status = %d body=%s", boardRec.Code, boardRec.Body.String())
	}
	var board console.TaskBoardView
	if err := json.NewDecoder(boardRec.Body).Decode(&board); err != nil {
		t.Fatal(err)
	}
	if board.TaskCounts[string(protocol.TaskStatusPlanned)] != 1 {
		t.Fatalf("planned count = %+v", board.TaskCounts)
	}
	if board.TaskCounts[string(protocol.TaskStatusReady)] != 1 {
		t.Fatalf("ready count = %+v", board.TaskCounts)
	}
	if len(board.Groups) < 2 {
		t.Fatalf("groups = %+v", board.Groups)
	}

	traceTasksReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/tasks", nil)
	traceTasksRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(traceTasksRec, traceTasksReq)
	if traceTasksRec.Code != http.StatusOK {
		t.Fatalf("trace tasks status = %d body=%s", traceTasksRec.Code, traceTasksRec.Body.String())
	}
	var traceTasks []console.TaskListItem
	if err := json.NewDecoder(traceTasksRec.Body).Decode(&traceTasks); err != nil {
		t.Fatal(err)
	}
	if len(traceTasks) != 2 {
		t.Fatalf("trace tasks = %+v", traceTasks)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/tasks/"+readyID, nil)
	detailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("task detail status = %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail console.TaskDetailView
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.TaskID != readyID || len(detail.Runs) != 1 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Status == string(protocol.TaskStatusReady) && detail.CanStart {
		t.Fatalf("ready task should not be startable: %+v", detail)
	}

	createBadReq := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString(`{}`))
	createBadRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createBadRec, createBadReq)
	if createBadRec.Code != http.StatusBadRequest {
		t.Fatalf("create bad status = %d body=%s", createBadRec.Code, createBadRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewBufferString(`{"title":"New task","body":"do things"}`))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("create without nats status = %d body=%s", createRec.Code, createRec.Body.String())
	}

	startReq := httptest.NewRequest(http.MethodPost, "/api/traces/"+traceID+"/tasks/"+plannedID+"/start", nil)
	startRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("start without nats status = %d body=%s", startRec.Code, startRec.Body.String())
	}
}

func TestReviewQueueAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	traceID := "trace-review"
	requiredID := "task-required"
	finalID := "task-final"
	plannedID := "task-planned"

	for _, spec := range []struct {
		taskID  string
		title   string
		status  protocol.TaskStatus
		review  protocol.TaskReviewPolicy
		summary string
	}{
		{requiredID, "Per-task review", protocol.TaskStatusWaitingReview, protocol.TaskReviewRequired, "Awaiting human approval"},
		{finalID, "Final merge gate", protocol.TaskStatusWaitingReview, protocol.TaskReviewFinal, "All tasks completed — awaiting human review and merge"},
		{plannedID, "Planned work", protocol.TaskStatusPlanned, protocol.TaskReviewNone, ""},
	} {
		taskDir, err := runs.NewTaskDir(repo, traceID, spec.taskID)
		if err != nil {
			t.Fatal(err)
		}
		if err := taskDir.WriteTask(runs.TaskFrontmatter{
			TraceID: traceID,
			TaskID:  spec.taskID,
			Title:   spec.title,
			Bee:     "builder",
			Status:  spec.status,
			Review:  spec.review,
			Summary: spec.summary,
		}, "task body"); err != nil {
			t.Fatal(err)
		}
	}

	started := time.Now().UTC()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "builder-1"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	summaryEv, err := protocol.NewEvent(traceID, "builder-1", 1, protocol.EventInsight, protocol.TraceSummaryPayload{
		Kind:    protocol.InsightTraceSummary,
		Summary: "Implemented OAuth callback and added focused tests",
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryEv.CreatedAt = started
	if err := d.AppendEvent(summaryEv); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	queueReq := httptest.NewRequest(http.MethodGet, "/api/review-queue", nil)
	queueRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(queueRec, queueReq)
	if queueRec.Code != http.StatusOK {
		t.Fatalf("review queue status = %d body=%s", queueRec.Code, queueRec.Body.String())
	}
	var queue console.ReviewQueueView
	if err := json.NewDecoder(queueRec.Body).Decode(&queue); err != nil {
		t.Fatal(err)
	}
	if queue.Count != 2 || len(queue.Items) != 2 {
		t.Fatalf("queue = %+v", queue)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/tasks/"+requiredID, nil)
	detailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("task detail status = %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail console.TaskDetailView
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.Review != string(protocol.TaskReviewRequired) {
		t.Fatalf("review policy = %q", detail.Review)
	}
	if !detail.CanApprove || !detail.CanReject {
		t.Fatalf("detail actions = %+v", detail)
	}
	if detail.IsFinal {
		t.Fatalf("required task should not be final: %+v", detail)
	}

	finalDetailReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/tasks/"+finalID, nil)
	finalDetailRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(finalDetailRec, finalDetailReq)
	if finalDetailRec.Code != http.StatusOK {
		t.Fatalf("final detail status = %d body=%s", finalDetailRec.Code, finalDetailRec.Body.String())
	}
	var finalDetail console.TaskDetailView
	if err := json.NewDecoder(finalDetailRec.Body).Decode(&finalDetail); err != nil {
		t.Fatal(err)
	}
	if !finalDetail.IsFinal {
		t.Fatalf("final task detail = %+v", finalDetail)
	}
	if finalDetail.TraceSummary != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("final traceSummary = %q", finalDetail.TraceSummary)
	}

	var finalQueueItem *console.ReviewQueueItem
	for i := range queue.Items {
		if queue.Items[i].TaskID == finalID {
			finalQueueItem = &queue.Items[i]
			break
		}
	}
	if finalQueueItem == nil {
		t.Fatal("final queue item not found")
	}
	if finalQueueItem.TraceSummary != "Implemented OAuth callback and added focused tests" {
		t.Fatalf("queue traceSummary = %q", finalQueueItem.TraceSummary)
	}
	var requiredQueueItem *console.ReviewQueueItem
	for i := range queue.Items {
		if queue.Items[i].TaskID == requiredID {
			requiredQueueItem = &queue.Items[i]
			break
		}
	}
	if requiredQueueItem == nil {
		t.Fatal("required queue item not found")
	}
	if requiredQueueItem.TraceSummary != "" {
		t.Fatalf("required queue traceSummary = %q, want omitted", requiredQueueItem.TraceSummary)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/traces/"+traceID+"/tasks/"+requiredID+"/approve", bytes.NewBufferString(`{"summary":"looks good"}`))
	approveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("approve without nats status = %d body=%s", approveRec.Code, approveRec.Body.String())
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/traces/"+traceID+"/tasks/"+plannedID+"/reject", bytes.NewBufferString(`{"feedback":"needs changes"}`))
	rejectPlannedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rejectPlannedRec, rejectReq)
	if rejectPlannedRec.Code != http.StatusBadRequest {
		t.Fatalf("reject planned task status = %d body=%s", rejectPlannedRec.Code, rejectPlannedRec.Body.String())
	}

	rejectReq = httptest.NewRequest(http.MethodPost, "/api/traces/"+traceID+"/tasks/"+requiredID+"/reject", bytes.NewBufferString(`{"feedback":"needs changes"}`))
	rejectRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rejectRec, rejectReq)
	if rejectRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("reject without nats status = %d body=%s", rejectRec.Code, rejectRec.Body.String())
	}

	methodReq := httptest.NewRequest(http.MethodGet, "/api/review-queue", nil)
	methodReq.Method = http.MethodPost
	methodRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(methodRec, methodReq)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("review queue POST status = %d", methodRec.Code)
	}
}

func TestListRunsProjection(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	started := time.Now().UTC()
	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-proj", AgentID: "agent-proj"}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         "trace-proj",
		AgentID:         "agent-proj",
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}

	list, err := console.ListRuns(ctxColony)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].AgentID != "agent-proj" {
		t.Fatalf("list = %+v", list)
	}
}

type outputSessionAdapter struct{}

func (o *outputSessionAdapter) Name() string { return "cursor" }

func (o *outputSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", `printf '\033[31mhello-console\033[0m\n'; exit 0`},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

func TestColonyTopologyAPIHandler(t *testing.T) {
	repo := initTopologyFixtureRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	srv := console.NewServer(console.Options{
		Addr:   "127.0.0.1:0",
		Colony: ctxColony,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/colony/topology", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("topology status = %d body=%s", rec.Code, rec.Body.String())
	}

	var got colony.Topology
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got.Bees) == 0 || len(got.Events) == 0 || len(got.Edges) == 0 {
		t.Fatalf("expected non-empty topology: bees=%d events=%d edges=%d",
			len(got.Bees), len(got.Events), len(got.Edges))
	}
	if strings.TrimSpace(got.Mermaid) == "" {
		t.Fatal("expected non-empty mermaid")
	}

	fixtureRoot := filepath.Join("..", "colony", "testdata", "topology-fixture")

	gotStructured, err := json.MarshalIndent(struct {
		Bees   []colony.TopologyBee   `json:"bees"`
		Events []colony.TopologyEvent `json:"events"`
		Edges  []colony.TopologyEdge  `json:"edges"`
	}{
		Bees:   got.Bees,
		Events: got.Events,
		Edges:  got.Edges,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	gotStructured = append(gotStructured, '\n')

	wantStructured, err := os.ReadFile(filepath.Join(fixtureRoot, "topology.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotStructured) != string(wantStructured) {
		t.Fatalf("topology JSON mismatch:\nwant:\n%s\ngot:\n%s", wantStructured, gotStructured)
	}

	wantMermaid, err := os.ReadFile(filepath.Join(fixtureRoot, "topology.golden.mermaid"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Mermaid != strings.TrimRight(string(wantMermaid), "\n") {
		t.Fatalf("mermaid mismatch:\nwant:\n%s\ngot:\n%s", wantMermaid, got.Mermaid)
	}

	methodReq := httptest.NewRequest(http.MethodPost, "/api/colony/topology", nil)
	methodRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(methodRec, methodReq)
	if methodRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("topology POST status = %d", methodRec.Code)
	}
}

func initTopologyFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")

	src := filepath.Join("..", "colony", "testdata", "topology-fixture", ".paseka")
	dst := filepath.Join(dir, ".paseka")
	cmd := exec.Command("cp", "-a", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("copy topology fixture: %v\n%s", err, out)
	}
	runGit(t, dir, "add", ".paseka")
	runGit(t, dir, "commit", "-m", "topology fixture")
	return dir
}

func initConsoleRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	runGit(t, dir, "commit", "--allow-empty", "-m", "init")

	paseka := filepath.Join(dir, ".paseka")
	for _, sub := range []string{"bees", "prompts"} {
		if err := os.MkdirAll(filepath.Join(paseka, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: console-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scoutBee := `role: scout
adapter: cursor
prompt_template: scout.md
`
	if err := os.WriteFile(filepath.Join(paseka, "bees", "scout.yaml"), []byte(scoutBee), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paseka, "prompts", "scout.md"), []byte("Task: {{.Task}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func setupConsoleHome(t *testing.T, repo string) colony.Context {
	t.Helper()
	return setupConsoleHomeWithNATS(t, repo, "")
}

func setupConsoleHomeWithNATS(t *testing.T, repo, natsURL string) colony.Context {
	t.Helper()
	slug := "console-test"
	if manifest, err := colony.LoadColony(repo); err == nil && strings.TrimSpace(manifest.Slug) != "" {
		slug = strings.TrimSpace(manifest.Slug)
	}
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\n"
	if natsURL != "" {
		cfg += "nats:\n  url: " + natsURL + "\n"
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "adapters", "cursor.yaml"), []byte("binary: agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
