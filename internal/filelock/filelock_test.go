package filelock

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireDoesNotStealLiveStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	first, err := Acquire(path, Options{Budget: time.Second, StaleAfter: 20 * time.Millisecond, Heartbeat: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Acquire(path, Options{Budget: 80 * time.Millisecond, StaleAfter: 20 * time.Millisecond, Heartbeat: time.Hour}); err == nil {
		t.Fatal("stole a stale lock whose owner is still alive")
	}
}

func TestAcquireReclaimsDeadOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	if err := os.WriteFile(path, []byte("99999999 deadbeef\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	lock, err := Acquire(path, Options{Budget: time.Second, StaleAfter: 20 * time.Millisecond, Heartbeat: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()
}

func TestReleaseLeavesReplacedLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, err := Acquire(path, Options{Budget: time.Second, StaleAfter: time.Minute, Heartbeat: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("123 other-owner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock.Release()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("release removed another owner's lock: %v", err)
	}
}

func TestHeartbeatKeepsHeldLockFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	lock, err := Acquire(path, Options{Budget: time.Second, StaleAfter: 60 * time.Millisecond, Heartbeat: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()

	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > 40*time.Millisecond {
		t.Fatalf("heartbeat did not refresh lock mtime: %v", time.Since(info.ModTime()))
	}
}
