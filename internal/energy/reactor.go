package energy

import "github.com/paseka/paseka/internal/colony"

const runtimeStatusRunning = "running"

// ReactorAlive reports whether the hive reactor process is alive and running.
func ReactorAlive(slug string) (bool, error) {
	entry, err := colony.RuntimeRegistry(slug)
	if err != nil {
		return false, err
	}
	if entry == nil || entry.PID <= 0 {
		return false, nil
	}
	if !colony.ProcessAlive(entry.PID) {
		return false, nil
	}
	status := entry.Status
	if status == "" {
		status = runtimeStatusRunning
	}
	if status == "stopping" {
		return false, nil
	}
	return status == runtimeStatusRunning, nil
}
