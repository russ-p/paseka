package sessions_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/sessions"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

func TestSessionHoldsNewWorkWhenConfigurationApplyPending(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	traceID := "trace-pending-session"
	if _, err := trailconfig.Ensure(ctx, traceID); err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(repo, ".paseka", "prompts", "scout.md")
	if err := os.WriteFile(promptPath, []byte("NEW {{.Task}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	applied, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx,
		TraceID: traceID,
		Source:  trailconfig.SourceColony,
		Actor:   "operator",
		Blockers: []string{
			"task task-1 is running",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if applied.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("apply outcome = %q", applied.Change.Outcome)
	}

	adapter := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", adapter)
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = mgr.RunInteractive(runCtx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		TraceID:  traceID,
		TaskID:   "task-1",
		Task:     "do not launch",
	})
	if err == nil || !strings.Contains(err.Error(), "pending configuration change") {
		t.Fatalf("err = %v, want pending configuration refusal", err)
	}
	if adapter.lastReq.Bee != "" {
		t.Fatalf("adapter launched a session while configuration was pending: %+v", adapter.lastReq)
	}
}

func TestSessionRefusesDroppedFork(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	traceID := "trace-dropped-session"
	lineage := trailfork.Lineage{TraceID: traceID, SourceTrace: "trace-source", State: trailfork.StateDropped}
	data, err := json.Marshal(lineage)
	if err != nil {
		t.Fatal(err)
	}
	path := trailfork.LineagePath(repo, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	adapter := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", adapter)
	runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = mgr.RunInteractive(runCtx, sessions.RunRequest{
		StartDir: repo,
		Bee:      "scout",
		TraceID:  traceID,
		Task:     "should not launch",
	})
	if err == nil || !strings.Contains(err.Error(), "dropped") {
		t.Fatalf("err = %v, want dropped refusal", err)
	}
	if adapter.lastReq.Bee != "" {
		t.Fatalf("adapter launched a session for a dropped fork: %+v", adapter.lastReq)
	}
}
