package colony

import "github.com/russ-p/paseka/internal/runs"

// ProcessAlive reports whether pid refers to a running process on this machine.
// The check itself lives in runs, because the run projection must decide
// whether an in-flight status record still has an owner without importing this
// package; this name stays colony's vocabulary for it.
func ProcessAlive(pid int) bool {
	return runs.PIDAlive(pid)
}
