package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

type implementationSessionAdapter struct {
	lastReq    adapters.SessionRequest
	cleanup    atomic.Int32
	binary     string
	script     string
	impl       string
	providerID string
}

func (a *implementationSessionAdapter) Name() string { return "opencode" }

func (a *implementationSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	a.lastReq = req
	binary := a.binary
	if binary == "" {
		shell, err := exec.LookPath("sh")
		if err != nil {
			return adapters.SessionCommand{}, err
		}
		binary = shell
	}
	script := a.script
	if script == "" {
		script = "exit 0"
	}
	args := []string{"-c", script}
	if binary != "" && a.binary != "" {
		args = nil
	}
	return adapters.SessionCommand{
		Binary:            binary,
		Args:              args,
		Env:               os.Environ(),
		Dir:               req.Workspace,
		ProviderSessionID: a.providerID,
		Implementation:    a.impl,
		Cleanup:           func() { a.cleanup.Add(1) },
	}, nil
}

func TestManagerRecordsImplementationAndCleansUp(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeScoutBee(t, repo, "opencode")

	stub := &implementationSessionAdapter{impl: "opencodev2", providerID: "ses_impl"}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("opencode", stub)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.StartDetached(ctx, sessions.RunRequest{StartDir: repo, Bee: "scout", Task: "hello", TraceID: "trace-impl"})
	if err != nil {
		t.Fatal(err)
	}
	waitSessionDone(t, mgr)

	if stub.lastReq.AdapterName != "opencode" {
		t.Fatalf("session request adapter = %q", stub.lastReq.AdapterName)
	}
	meta, err := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Adapter != "opencode" || meta.Implementation != "opencodev2" {
		t.Fatalf("session identity = %q/%q", meta.Adapter, meta.Implementation)
	}
	if stub.cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", stub.cleanup.Load())
	}
}

func TestManagerCleansUpWhenPTYFailsToStart(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeScoutBee(t, repo, "opencode")

	stub := &implementationSessionAdapter{
		impl:   "opencodev2",
		binary: repo + "/does-not-exist",
	}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("opencode", stub)

	_, err := mgr.StartDetached(context.Background(), sessions.RunRequest{
		StartDir: repo, Bee: "scout", Task: "hello", TraceID: "trace-impl",
	})
	if err == nil {
		t.Fatal("expected PTY start failure")
	}
	if stub.cleanup.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", stub.cleanup.Load())
	}
}

type resolvingSessionAdapter struct {
	implementationSessionAdapter
	next string
}

func (a *resolvingSessionAdapter) ResolveImplementation(adapters.RunParams) (string, error) {
	return a.next, nil
}

func TestManagerResumeRefusesCrossMajorGeneration(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeScoutBee(t, repo, "opencode")

	writeFinishedSession(t, repo, "agent-impl", "trace-impl", "ses_impl", "", "opencode")
	d := runs.Dir{ColonyRoot: repo, TraceID: "trace-impl", AgentID: "agent-impl"}
	meta, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	meta.Implementation = "opencodev1"
	if err := d.WriteSession(meta); err != nil {
		t.Fatal(err)
	}

	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("opencode", &resolvingSessionAdapter{next: "opencodev2"})

	_, err = mgr.ResumeDetached(context.Background(), sessions.ResumeRequest{StartDir: repo, SessionID: "agent-impl"})
	assertResumeCode(t, err, sessions.ResumeErrGenerationMismatch, 400)
}

func TestManagerResumeAllowsSessionWithoutRecordedGeneration(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	writeScoutBee(t, repo, "opencode")
	writeFinishedSession(t, repo, "agent-legacy", "trace-impl", "ses_impl", "", "opencode")

	mgr := sessions.NewManager()
	stub := &resolvingSessionAdapter{next: "opencodev2"}
	stub.impl = "opencodev2"
	mgr.RegisterSessionAdapter("opencode", stub)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := mgr.ResumeDetached(ctx, sessions.ResumeRequest{
		StartDir:  repo,
		SessionID: "agent-legacy",
		Continue:  "keep going",
	})
	if err != nil {
		t.Fatal(err)
	}
	waitSessionDone(t, mgr)
	meta, err := runs.Dir{ColonyRoot: repo, TraceID: res.TraceID, AgentID: res.AgentID}.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if meta.Implementation != "opencodev2" {
		t.Fatalf("implementation = %q", meta.Implementation)
	}
}
