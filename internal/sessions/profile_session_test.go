package sessions_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

func TestManagerSessionRecordsProfileName(t *testing.T) {
	repo := initSessionRepo(t)
	setupSessionHome(t, repo)
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "profiles", "pi.yaml"), []byte("params:\n  model: high\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	colony.SetProcessProfile(colony.ProfileSelection{Name: "pi", FlagSet: true})
	t.Cleanup(func() { colony.SetProcessProfile(colony.ProfileSelection{}) })

	fast := &instantSessionAdapter{}
	mgr := sessions.NewManager()
	mgr.RegisterSessionAdapter("cursor", fast)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunInteractive(ctx, sessions.RunRequest{
			StartDir: repo,
			Bee:      "scout",
			Task:     "hello profile",
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

	runDirs, _ := filepath.Glob(filepath.Join(repo, ".paseka", "runs", "*", "*"))
	if len(runDirs) == 0 {
		t.Fatal("expected run dir")
	}
	d := runs.Dir{
		ColonyRoot: repo,
		TraceID:    filepath.Base(filepath.Dir(runDirs[0])),
		AgentID:    filepath.Base(runDirs[0]),
	}
	sess, err := d.ReadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Profile != "pi" {
		t.Fatalf("session profile = %q", sess.Profile)
	}
}
