package adapters

import "os/exec"

// RunCommand starts cmd, calls onStarted with the OS pid, then waits for exit.
func RunCommand(cmd *exec.Cmd, onStarted func(pid int) error) error {
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
