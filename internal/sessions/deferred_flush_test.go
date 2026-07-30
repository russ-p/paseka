package sessions_test

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/sessions"
)

type pendingSessionAdapter struct {
	fail    bool
	lastReq adapters.SessionRequest
}

func (p *pendingSessionAdapter) Name() string { return "cursor" }

func (p *pendingSessionAdapter) SessionCommand(req adapters.SessionRequest) (adapters.SessionCommand, error) {
	p.lastReq = req
	runDir := runs.Dir{
		ColonyRoot: req.ColonyRoot,
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
	}
	ev, err := protocol.NewEvent(req.TraceID, req.AgentID, 0, protocol.EventInsight, map[string]any{
		"kind":    "context.note",
		"summary": "session deferred",
	})
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	if err := runDir.AppendPending(ev); err != nil {
		return adapters.SessionCommand{}, err
	}

	shell, err := exec.LookPath("sh")
	if err != nil {
		return adapters.SessionCommand{}, err
	}
	exitCode := "0"
	if p.fail {
		exitCode = "1"
	}
	return adapters.SessionCommand{
		Binary: shell,
		Args:   []string{"-c", "exit " + exitCode},
		Env:    os.Environ(),
		Dir:    req.Workspace,
	}, nil
}

func TestManagerFlushesDeferredOnSuccessfulSession(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	adapter := &pendingSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir: repo,
			Bee:      "scout",
			Task:     "hello deferred",
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session")
	}

	runDir := runs.Dir{
		ColonyRoot: repo,
		TraceID:    adapter.lastReq.TraceID,
		AgentID:    adapter.lastReq.AgentID,
	}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0 after successful session flush", len(pending))
	}
	audit, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 1 {
		t.Fatalf("audit events = %d, want 1 flushed deferred event", len(audit))
	}
}

func TestManagerLeavesPendingOnFailedSession(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)

	adapter := &pendingSessionAdapter{fail: true}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", adapter)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan struct {
		res *sessions.RunResult
		err error
	}, 1)
	go func() {
		res, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir: repo,
			Bee:      "scout",
			Task:     "fail with pending",
		})
		done <- struct {
			res *sessions.RunResult
			err error
		}{res, err}
	}()

	var result *sessions.RunResult
	select {
	case out := <-done:
		if out.err != nil {
			t.Fatal(out.err)
		}
		result = out.res
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for session")
	}
	if result.State != adapters.SessionFailed {
		t.Fatalf("state = %q, want failed", result.State)
	}

	runDir := runs.Dir{
		ColonyRoot: repo,
		TraceID:    adapter.lastReq.TraceID,
		AgentID:    adapter.lastReq.AgentID,
	}
	pending, err := runDir.ReadPending()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d, want 1", len(pending))
	}
	audit, err := runDir.ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(audit) != 0 {
		t.Fatalf("audit events = %d, want 0 without flush", len(audit))
	}

	status, err := runDir.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.Error, "deferred event") {
		t.Fatalf("status error = %q, want pending warning", status.Error)
	}
}
