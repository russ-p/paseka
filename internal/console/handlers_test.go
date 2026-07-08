package console_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
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

	createBody := `{"bee":"scout","task":"console hello"}`
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
		t.Fatalf("expected result.txt: %v", err)
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
		FinishedAt:      started.Add(2 * time.Minute),
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
	if detail.Task != "headless task" || detail.Intent != "feature" {
		t.Fatalf("detail = %+v", detail)
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
	slug := "console-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: " + slug + "\n"
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
