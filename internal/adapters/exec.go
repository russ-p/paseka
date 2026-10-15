package adapters

import (
	"os/exec"
	"syscall"
	"time"
)

// TerminateGrace is the window between SIGTERM and SIGKILL when an AFK adapter
// process is being brought down (params.timeout expiry or a manual cancel). It
// gives a well-behaved agent a chance to flush its output before it is force
// killed.
const TerminateGrace = 5 * time.Second

// configureTermination makes ctx cancellation graceful: the child receives
// SIGTERM first and is SIGKILLed only if it is still alive after
// TerminateGrace. exec.CommandContext seeds Cmd.Cancel with a bare
// Process.Kill, so this overrides that default; without it an agent loses its
// chance to flush a summary.
func configureTermination(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	cmd.WaitDelay = TerminateGrace
}

// RunCommand starts cmd, calls onStarted with the OS pid, then waits for exit.
func RunCommand(cmd *exec.Cmd, onStarted func(pid int) error) error {
	configureTermination(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	if onStarted != nil {
		if err := onStarted(pid); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return err
		}
	}
	return cmd.Wait()
}
