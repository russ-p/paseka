package runtime

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/paseka/paseka/internal/colony"
)

// SpawnFunc launches a detached `paseka run` and returns the child PID.
type SpawnFunc func(exe, colonyRoot string) (int, error)

// Supervisor controls an external hive runtime process for one colony.
type Supervisor struct {
	Spawn SpawnFunc
}

// DefaultSupervisor returns a supervisor that spawns the current paseka binary.
func DefaultSupervisor() *Supervisor {
	return &Supervisor{Spawn: spawnDetachedRun}
}

// Status returns the resolved runtime lifecycle state.
func (s *Supervisor) Status(ctx colony.Context) (RuntimeStatus, error) {
	return ResolveStatus(ctx.Slug)
}

// Start launches `paseka run` when no alive runtime is registered.
func (s *Supervisor) Start(ctx colony.Context) (RuntimeStatus, error) {
	current, err := ResolveStatus(ctx.Slug)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if current.Alive && current.Status == RuntimeStatusRunning {
		return current, nil
	}
	if current.Status == RuntimeStatusStale {
		if err := colony.ClearRuntime(ctx.Slug); err != nil {
			return RuntimeStatus{}, err
		}
	}

	spawn := s.Spawn
	if spawn == nil {
		spawn = spawnDetachedRun
	}
	exe, err := os.Executable()
	if err != nil {
		return RuntimeStatus{}, err
	}
	pid, err := spawn(exe, ctx.ColonyRoot)
	if err != nil {
		return RuntimeStatus{}, fmt.Errorf("runtime: start: %w", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		st, err := ResolveStatus(ctx.Slug)
		if err != nil {
			return RuntimeStatus{}, err
		}
		if st.Alive && st.PID == pid {
			return st, nil
		}
		if st.Alive && st.PID > 0 && st.PID != pid {
			return st, fmt.Errorf("runtime: another reactor is already running (pid %d)", st.PID)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if colony.ProcessAlive(pid) {
		now := time.Now().UTC()
		entry := colony.RuntimeEntry{
			PID:             pid,
			StartedAt:       now,
			ColonyRoot:      ctx.ColonyRoot,
			Status:          RuntimeStatusRunning,
			LastHeartbeatAt: now,
		}
		if err := colony.RegisterRuntime(ctx.Slug, entry); err != nil {
			return RuntimeStatus{}, err
		}
		return ResolveStatus(ctx.Slug)
	}
	return RuntimeStatus{}, fmt.Errorf("runtime: process %d exited before registration", pid)
}

// Stop sends SIGTERM to the registered runtime and waits for exit.
func (s *Supervisor) Stop(ctx colony.Context) (RuntimeStatus, error) {
	entry, err := colony.RuntimeRegistry(ctx.Slug)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if entry == nil || entry.PID <= 0 {
		return RuntimeStatus{Status: RuntimeStatusStopped}, nil
	}
	if !colony.ProcessAlive(entry.PID) {
		_ = colony.ClearRuntime(ctx.Slug)
		return RuntimeStatus{Status: RuntimeStatusStopped}, nil
	}

	entry.Status = RuntimeStatusStopping
	if err := colony.RegisterRuntime(ctx.Slug, *entry); err != nil {
		return RuntimeStatus{}, err
	}

	proc, err := os.FindProcess(entry.PID)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return RuntimeStatus{}, fmt.Errorf("runtime: signal pid %d: %w", entry.PID, err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if !colony.ProcessAlive(entry.PID) {
			_ = colony.ClearRuntime(ctx.Slug)
			return RuntimeStatus{Status: RuntimeStatusStopped}, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ResolveStatus(ctx.Slug)
}

func spawnDetachedRun(exe, colonyRoot string) (int, error) {
	cmd := exec.Command(exe, "run", "-C", colonyRoot)
	cmd.Dir = colonyRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}
