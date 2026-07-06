package runs

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// ReadTraceEvents loads domain events from all agent runs under .paseka/runs/<traceId>/.
// Events are returned in chronological order (by createdAt, then seq).
func ReadTraceEvents(colonyRoot, traceID string) ([]protocol.Event, error) {
	if colonyRoot == "" || traceID == "" {
		return nil, fmt.Errorf("runs: colony root and traceId are required")
	}
	traceRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(traceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("runs: list trace %s: %w", traceID, err)
	}

	var all []protocol.Event
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		agentID := ent.Name()
		runDir := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
		events, err := runDir.ReadEvents()
		if err != nil {
			return nil, fmt.Errorf("runs: read events for agent %s: %w", agentID, err)
		}
		all = append(all, events...)
	}

	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		if a.Seq != b.Seq {
			return a.Seq < b.Seq
		}
		return strings.Compare(a.AgentID, b.AgentID) < 0
	})
	return all, nil
}
