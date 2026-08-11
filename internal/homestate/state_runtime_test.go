package homestate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/homestate"
)

func TestRuntimeRegistry(t *testing.T) {
	slug, home := setupStateHome(t)
	_ = home

	started := time.Now().UTC().Truncate(time.Second)
	entry := homestate.RuntimeEntry{
		PID:             os.Getpid(),
		StartedAt:       started,
		ColonyRoot:      "/tmp/colony",
		SubjectPrefix:   "paseka.test",
		Status:          "running",
		LastHeartbeatAt: started,
	}
	if err := homestate.RegisterRuntime(slug, entry); err != nil {
		t.Fatal(err)
	}

	got, err := homestate.RuntimeRegistry(slug)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.PID != entry.PID || got.ColonyRoot != entry.ColonyRoot {
		t.Fatalf("registry = %+v", got)
	}

	later := started.Add(30 * time.Second)
	if err := homestate.TouchRuntimeHeartbeat(slug, entry.PID, later); err != nil {
		t.Fatal(err)
	}
	got, err = homestate.RuntimeRegistry(slug)
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastHeartbeatAt.Equal(later) {
		t.Fatalf("heartbeat = %v want %v", got.LastHeartbeatAt, later)
	}

	if err := homestate.UnregisterRuntimeIfPID(slug, entry.PID+1); err != nil {
		t.Fatal(err)
	}
	got, err = homestate.RuntimeRegistry(slug)
	if err != nil || got == nil {
		t.Fatalf("expected entry to remain: %+v err=%v", got, err)
	}

	if err := homestate.UnregisterRuntimeIfPID(slug, entry.PID); err != nil {
		t.Fatal(err)
	}
	got, err = homestate.RuntimeRegistry(slug)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected cleared registry, got %+v", got)
	}
}

func TestRuntimeRegistryStalePID(t *testing.T) {
	slug, _ := setupStateHome(t)
	stalePID := 99999999
	if err := homestate.RegisterRuntime(slug, homestate.RuntimeEntry{
		PID:        stalePID,
		StartedAt:  time.Now().UTC(),
		ColonyRoot: "/tmp/colony",
		Status:     "running",
	}); err != nil {
		t.Fatal(err)
	}
	if colony.ProcessAlive(stalePID) {
		t.Fatalf("expected stale pid %d to be absent", stalePID)
	}
	got, err := homestate.RuntimeRegistry(slug)
	if err != nil || got == nil || got.PID != stalePID {
		t.Fatalf("registry = %+v err=%v", got, err)
	}
	if err := homestate.ClearRuntime(slug); err != nil {
		t.Fatal(err)
	}
	got, err = homestate.RuntimeRegistry(slug)
	if err != nil || got != nil {
		t.Fatalf("expected cleared registry, got %+v err=%v", got, err)
	}
}

func setupStateHome(t *testing.T) (string, string) {
	t.Helper()
	slug := "state-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return slug, homeDir
}
