package runtime

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
)

const (
	RuntimeStatusRunning  = "running"
	RuntimeStatusStopped  = "stopped"
	RuntimeStatusStale    = "stale"
	RuntimeStatusStopping = "stopping"

	runtimeHeartbeatInterval = 15 * time.Second
)

// RuntimeStatus is the resolved hive runtime lifecycle state for operators.
type RuntimeStatus struct {
	Status          string    `json:"status"`
	PID             int       `json:"pid,omitempty"`
	StartedAt       time.Time `json:"startedAt,omitempty"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt,omitempty"`
	ColonyRoot      string    `json:"colonyRoot,omitempty"`
	SubjectPrefix   string    `json:"subjectPrefix,omitempty"`
	Alive           bool      `json:"alive"`
}

// RegisterSelf records the current process as the hive runtime for the colony.
func RegisterSelf(ctx colony.Context) error {
	manifest, err := colony.LoadColony(ctx.ColonyRoot)
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(manifest.NATS.SubjectPrefix)
	if prefix == "" {
		prefix = "paseka." + ctx.Slug
	}
	now := time.Now().UTC()
	return colony.RegisterRuntime(ctx.Slug, colony.RuntimeEntry{
		PID:             os.Getpid(),
		StartedAt:       now,
		ColonyRoot:      ctx.ColonyRoot,
		SubjectPrefix:   prefix,
		Status:          RuntimeStatusRunning,
		LastHeartbeatAt: now,
	})
}

// UnregisterSelf removes the runtime registry entry when it still refers to this process.
func UnregisterSelf(ctx colony.Context) error {
	return colony.UnregisterRuntimeIfPID(ctx.Slug, os.Getpid())
}

// TouchHeartbeat updates the runtime heartbeat timestamp for this process.
func TouchHeartbeat(ctx colony.Context) error {
	return colony.TouchRuntimeHeartbeat(ctx.Slug, os.Getpid(), time.Now().UTC())
}

// RunHeartbeat periodically updates runtime heartbeat in state.json until ctx is cancelled.
func RunHeartbeat(ctx context.Context, ctxColony colony.Context) {
	ticker := time.NewTicker(runtimeHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = TouchHeartbeat(ctxColony)
		}
	}
}

// ResolveStatus inspects the runtime registry and OS process table.
func ResolveStatus(slug string) (RuntimeStatus, error) {
	entry, err := colony.RuntimeRegistry(slug)
	if err != nil {
		return RuntimeStatus{}, err
	}
	if entry == nil || entry.PID <= 0 {
		return RuntimeStatus{Status: RuntimeStatusStopped}, nil
	}

	alive := colony.ProcessAlive(entry.PID)
	status := entry.Status
	if status == "" {
		status = RuntimeStatusRunning
	}
	if !alive {
		status = RuntimeStatusStale
	} else if status == RuntimeStatusStopping {
		// keep stopping while process is still alive
	} else {
		status = RuntimeStatusRunning
	}

	return RuntimeStatus{
		Status:          status,
		PID:             entry.PID,
		StartedAt:       entry.StartedAt,
		LastHeartbeatAt: entry.LastHeartbeatAt,
		ColonyRoot:      entry.ColonyRoot,
		SubjectPrefix:   entry.SubjectPrefix,
		Alive:           alive,
	}, nil
}
