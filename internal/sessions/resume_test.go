package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/homestate"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

func TestManagerResumeHappyPath(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	source := writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")
	sourceBytes, err := os.ReadFile(source.SessionPath())
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{
		StartDir:  repo,
		SessionID: "agent-src",
		Continue:  "keep going",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID == "" || res.SessionID == "agent-src" {
		t.Fatalf("new session id = %q", res.SessionID)
	}
	if res.TraceID != "trace-src" {
		t.Fatalf("trace = %q", res.TraceID)
	}
	if rec.lastReq.ResumeSessionID != "cursor-uuid" {
		t.Fatalf("ResumeSessionID = %q", rec.lastReq.ResumeSessionID)
	}
	if rec.lastReq.InitialPrompt != "keep going" {
		t.Fatalf("continue = %q", rec.lastReq.InitialPrompt)
	}
	if rec.lastReq.SystemPrompt != "" {
		t.Fatalf("system prompt = %q", rec.lastReq.SystemPrompt)
	}

	got, err := os.ReadFile(source.SessionPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(sourceBytes) {
		t.Fatalf("source session.json changed")
	}

	d := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}
	waitSessionDone(t, mgr)
	meta, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProviderSessionID != "cursor-uuid" {
		t.Fatalf("providerSessionId = %q", meta.ProviderSessionID)
	}
	if meta.ResumedFrom != "agent-src" {
		t.Fatalf("resumedFrom = %q", meta.ResumedFrom)
	}
	prompt, err := os.ReadFile(d.PromptPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(prompt) != "keep going" {
		t.Fatalf("prompt.txt = %q", prompt)
	}
}

func TestManagerResumeEmptyContinueOmitsPrompt(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")

	rec := &recordingSessionAdapter{id: "cursor-uuid"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.InitialPrompt != "" {
		t.Fatalf("continue = %q", rec.lastReq.InitialPrompt)
	}
	waitSessionDone(t, mgr)
	if _, err := os.Stat(runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.PromptPath()); !os.IsNotExist(err) {
		t.Fatalf("prompt.txt should be omitted, err=%v", err)
	}
}

func TestManagerResumeChain(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-mid", "trace-src", "cursor-uuid", "agent-src")

	rec := &recordingSessionAdapter{id: "cursor-uuid"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "agent-mid"})
	if err != nil {
		t.Fatal(err)
	}
	waitSessionDone(t, mgr)
	meta, err := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.ResumedFrom != "agent-mid" || meta.ProviderSessionID != "cursor-uuid" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestManagerResumeIneligible(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &recordingSessionAdapter{})
	ctx := context.Background()

	_, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "missing"})
	assertResumeCode(t, err, "", 404)

	writeFinishedCursorSession(t, repo, "pi-src", "trace-src", "pi-id", "")
	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-src", AgentID: "pi-src"}
	meta, _ := d.ReadSession()
	meta.Adapter = "pi"
	if err := d.WriteSession(meta); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "pi-src"})
	assertResumeCode(t, err, sessions.ResumeErrNotResumable, 400)

	writeFinishedCursorSession(t, repo, "no-id", "trace-src", "", "")
	_, err = mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "no-id"})
	assertResumeCode(t, err, sessions.ResumeErrNoProviderSessionID, 400)
}

func TestManagerResumeRejectsActiveAndBusy(t *testing.T) {
	repo := initSessionRepo(t)
	slug := setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")

	slow := &recordingSessionAdapter{script: "sleep 2; exit 0"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", slow)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	live, err := mgr.StartDetached(ctx, sessions.RunRequest{StartDir: repo, Bee: "scout", Task: "hello", TraceID: "other-trace"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(live.SessionID) })

	_, err = mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: live.SessionID})
	assertResumeCode(t, err, sessions.ResumeErrStillActive, 409)

	if err := homestate.RegisterSession(slug, homestate.SessionEntry{
		SessionID: "agent-src",
		TraceID:   "trace-src",
		AgentID:   "agent-src",
		Bee:       "scout",
		PID:       os.Getpid(),
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	assertResumeCode(t, err, sessions.ResumeErrStillActive, 409)
	_ = homestate.UnregisterSession(slug, "agent-src")
}

func TestManagerResumeAllowsStaleRegistryPID(t *testing.T) {
	repo := initSessionRepo(t)
	slug := setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")

	stalePID := 99999999
	if colony.ProcessAlive(stalePID) {
		t.Skip("stale pid unexpectedly alive")
	}
	if err := homestate.RegisterSession(slug, homestate.SessionEntry{
		SessionID: "agent-src",
		TraceID:   "trace-src",
		AgentID:   "agent-src",
		Bee:       "scout",
		PID:       stalePID,
		StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	rec := &recordingSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", rec)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	if err != nil {
		t.Fatal(err)
	}
	if res.SessionID == "" || res.SessionID == "agent-src" {
		t.Fatalf("new session id = %q", res.SessionID)
	}
	waitSessionDone(t, mgr)
}

func TestManagerResumeRejectsProviderBusy(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")

	busy := &recordingSessionAdapter{id: "cursor-uuid", script: "sleep 2; exit 0"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", busy)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	live, err := mgr.StartDetached(ctx, sessions.RunRequest{StartDir: repo, Bee: "scout", Task: "hello", TraceID: "other-trace"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(live.SessionID) })

	_, err = mgr.ResumeDetached(ctx, sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	assertResumeCode(t, err, sessions.ResumeErrProviderSessionBusy, 409)
}

func TestManagerResumeRejectsCommandOverride(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")
	beePath := filepath.Join(repo, ".paseka", "bees", "scout.yaml")
	if err := os.WriteFile(beePath, []byte("role: scout\nadapter: cursor\nprompt_template: scout.md\ncommand: [agent, --resume, x]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &recordingSessionAdapter{})
	_, err := mgr.ResumeDetached(context.Background(), sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	assertResumeCode(t, err, sessions.ResumeErrCommandOverride, 400)
}

func TestManagerResumeRejectsAdapterChanged(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeFinishedCursorSession(t, repo, "agent-src", "trace-src", "cursor-uuid", "")
	beePath := filepath.Join(repo, ".paseka", "bees", "scout.yaml")
	if err := os.WriteFile(beePath, []byte("role: scout\nadapter: pi\nprompt_template: scout.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &recordingSessionAdapter{})
	_, err := mgr.ResumeDetached(context.Background(), sessions.ResumeRequest{StartDir: repo, SessionID: "agent-src"})
	assertResumeCode(t, err, sessions.ResumeErrAdapterChanged, 400)
}

func TestManagerResumeOpenCodeHappyPath(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeScoutBee(t, repo, "opencode")
	writeFinishedSession(t, repo, "agent-oc", "trace-oc", "oc-id", "", "opencode")

	rec := &recordingSessionAdapter{id: "oc-id"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("opencode", rec)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{
		StartDir:  repo,
		SessionID: "agent-oc",
		Continue:  "keep going",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TraceID != "trace-oc" {
		t.Fatalf("trace = %q", res.TraceID)
	}
	if rec.lastReq.ResumeSessionID != "oc-id" {
		t.Fatalf("ResumeSessionID = %q", rec.lastReq.ResumeSessionID)
	}
	if rec.lastReq.InitialPrompt != "keep going" {
		t.Fatalf("continue = %q", rec.lastReq.InitialPrompt)
	}
	waitSessionDone(t, mgr)
	meta, err := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.ProviderSessionID != "oc-id" || meta.ResumedFrom != "agent-oc" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestManagerResumeRejectsAdapterMismatch(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	// scout bee keeps adapter: cursor; source session claims opencode -> adapter changed.
	writeFinishedSession(t, repo, "agent-oc", "trace-oc", "oc-id", "", "opencode")

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", &recordingSessionAdapter{})
	mgr.RegisterSessionAdapter("opencode", &recordingSessionAdapter{})
	_, err := mgr.ResumeDetached(context.Background(), sessions.ResumeRequest{StartDir: repo, SessionID: "agent-oc"})
	assertResumeCode(t, err, sessions.ResumeErrAdapterChanged, 400)
}

func writeFinishedSession(t *testing.T, repo, sessionID, traceID, providerID, resumedFrom, adapter string) runs.Dir {
	t.Helper()
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: sessionID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Hour)
	if err := d.WriteSession(runs.SessionMeta{
		SessionID:         sessionID,
		TraceID:           traceID,
		AgentID:           sessionID,
		Bee:               "scout",
		Adapter:           adapter,
		Workspace:         repo,
		ColonyRoot:        repo,
		State:             string(adapters.SessionCompleted),
		ProviderSessionID: providerID,
		ResumedFrom:       resumedFrom,
		StartedAt:         started,
		FinishedAt:        started.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	return d
}

func writeScoutBee(t *testing.T, repo, adapter string) {
	t.Helper()
	beePath := filepath.Join(repo, ".paseka", "bees", "scout.yaml")
	body := "role: scout\nadapter: " + adapter + "\nprompt_template: scout.md\n"
	if err := os.WriteFile(beePath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFinishedCursorSession(t *testing.T, repo, sessionID, traceID, providerID, resumedFrom string) runs.Dir {
	t.Helper()
	return writeFinishedSession(t, repo, sessionID, traceID, providerID, resumedFrom, "cursor")
}

func waitSessionDone(t *testing.T, mgr *sessions.Manager) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListActive()) == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("session still active")
}

func assertResumeCode(t *testing.T, err error, code string, status int) {
	t.Helper()
	re, ok := sessions.AsResumeError(err)
	if !ok {
		t.Fatalf("err = %v (want ResumeError %s)", err, code)
	}
	if re.Code != code || re.Status != status {
		t.Fatalf("resume error = %+v, want code=%s status=%d", re, code, status)
	}
}

type recordingSessionAdapter struct {
	lastReq adapters.SessionRequest
	id      string
	script  string
}

func (r *recordingSessionAdapter) Name() string { return "cursor" }

func (r *recordingSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	r.lastReq = req
	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	script := r.script
	if script == "" {
		script = "exit 0"
	}
	id := r.id
	if id == "" {
		id = strings.TrimSpace(req.ResumeSessionID)
	}
	return adapters.SessionCommand{
		Binary:            shell,
		Args:              []string{"-c", script},
		Env:               os.Environ(),
		Dir:               req.Workspace,
		ProviderSessionID: id,
	}, nil
}
