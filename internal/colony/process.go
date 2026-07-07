package colony

import (
	"os"
	"syscall"
)

// ProcessAlive reports whether pid refers to a running process on this machine.
func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 does not deliver a signal; on Unix it checks existence.
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
