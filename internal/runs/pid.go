package runs

import (
	"os"
	"syscall"
)

// PIDAlive reports whether pid names a running process on this host.
//
// The run projection owns this check because it must decide whether an
// in-flight status record still has an owner without importing colony, which
// already imports runs. colony.ProcessAlive delegates here.
//
// The answer proves absence on this machine only: a pid recorded by a run in
// another pid namespace (a container, a remote host) reads as dead here, and
// pid reuse can make a dead run read as alive. Neither limit is solved by this
// check; it is why a dead pid projects an interruption with its reason instead
// of silently erasing the record, and why a run is never failed merely for
// having no pid at all.
func PIDAlive(pid int) bool {
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
