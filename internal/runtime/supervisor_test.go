package runtime_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

func TestResolveStatusStoppedAndRunning(t *testing.T) {
	ctx := setupSupervisorHome(t)

	st, err := runtime.ResolveStatus(ctx.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != runtime.RuntimeStatusStopped || st.Alive {
		t.Fatalf("stopped = %+v", st)
	}

	now := time.Now().UTC()
	if err := colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
		PID:             os.Getpid(),
		StartedAt:       now,
		ColonyRoot:      ctx.ColonyRoot,
		SubjectPrefix:   "paseka.test",
		Status:          runtime.RuntimeStatusRunning,
		LastHeartbeatAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = colony.ClearRuntime(ctx.Slug) })

	st, err = runtime.ResolveStatus(ctx.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != runtime.RuntimeStatusRunning || !st.Alive || st.PID != os.Getpid() {
		t.Fatalf("running = %+v", st)
	}
}

func TestSupervisorStartSingleton(t *testing.T) {
	ctx := setupSupervisorHome(t)
	pid := os.Getpid()
	if err := colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
		PID:        pid,
		StartedAt:  time.Now().UTC(),
		ColonyRoot: ctx.ColonyRoot,
		Status:     runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = colony.ClearRuntime(ctx.Slug) })

	spawned := 0
	sup := &runtime.Supervisor{
		Spawn: func(exe, colonyRoot string) (int, error) {
			spawned++
			return 424242, nil
		},
	}
	st, err := sup.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if spawned != 0 {
		t.Fatalf("expected no spawn when already running, spawned=%d", spawned)
	}
	if !st.Alive || st.PID != pid {
		t.Fatalf("status = %+v", st)
	}
}

func TestSupervisorStartClearsStale(t *testing.T) {
	ctx := setupSupervisorHome(t)
	stalePID := 99999999
	if err := colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
		PID:        stalePID,
		StartedAt:  time.Now().UTC(),
		ColonyRoot: ctx.ColonyRoot,
		Status:     runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	spawned := false
	sup := &runtime.Supervisor{
		Spawn: func(exe, colonyRoot string) (int, error) {
			spawned = true
			if err := colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
				PID:        os.Getpid(),
				StartedAt:  time.Now().UTC(),
				ColonyRoot: colonyRoot,
				Status:     runtime.RuntimeStatusRunning,
			}); err != nil {
				return 0, err
			}
			return os.Getpid(), nil
		},
	}
	st, err := sup.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !spawned {
		t.Fatal("expected spawn after stale registry")
	}
	if st.Status != runtime.RuntimeStatusRunning || !st.Alive {
		t.Fatalf("status = %+v", st)
	}
	_ = colony.ClearRuntime(ctx.Slug)
}

func TestSupervisorStopClearsStale(t *testing.T) {
	ctx := setupSupervisorHome(t)
	if err := colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
		PID:        99999999,
		StartedAt:  time.Now().UTC(),
		ColonyRoot: ctx.ColonyRoot,
		Status:     runtime.RuntimeStatusRunning,
	}); err != nil {
		t.Fatal(err)
	}

	sup := runtime.DefaultSupervisor()
	st, err := sup.Stop(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != runtime.RuntimeStatusStopped {
		t.Fatalf("status = %+v", st)
	}
	got, err := colony.RuntimeRegistry(ctx.Slug)
	if err != nil || got != nil {
		t.Fatalf("registry = %+v err=%v", got, err)
	}
}

func setupSupervisorHome(t *testing.T) colony.Context {
	t.Helper()
	dir := t.TempDir()
	slug := "runtime-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + dir + "\nslug: " + slug + "\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return colony.Context{ColonyRoot: dir, Slug: slug}
}
