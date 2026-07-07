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
	"github.com/paseka/paseka/internal/sessions"
)

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
