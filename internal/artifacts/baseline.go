package artifacts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paseka/paseka/internal/runs"
)

// BaselineSnapshot is persisted under the producing run directory.
type BaselineSnapshot struct {
	Hashes map[string]string `json:"hashes,omitempty"`
	Error  string            `json:"error,omitempty"`
}

// BaselinePath returns the path to artifacts-baseline.json for a run.
func BaselinePath(colonyRoot, traceID, agentID string) string {
	return filepath.Join(runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}.Root(), BaselineFileName)
}

// CaptureBaseline walks the comb and writes the baseline snapshot for a run.
func CaptureBaseline(colonyRoot, traceID, agentID string) error {
	hashes, err := Scan(colonyRoot, traceID)
	snap := BaselineSnapshot{Hashes: hashes}
	if err != nil {
		snap = BaselineSnapshot{Error: err.Error()}
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	path := BaselinePath(colonyRoot, traceID, agentID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("artifacts: mkdir baseline dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("artifacts: write baseline: %w", err)
	}
	if snap.Error != "" {
		return fmt.Errorf("%s", snap.Error)
	}
	return nil
}

// LoadBaseline reads a run's baseline snapshot. When capture failed, ok is false.
func LoadBaseline(colonyRoot, traceID, agentID string) (HashMap, bool, error) {
	path := BaselinePath(colonyRoot, traceID, agentID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Missing snapshot is not an empty baseline (that would announce the whole comb).
			return nil, false, nil
		}
		return nil, false, err
	}
	var snap BaselineSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, false, err
	}
	if snap.Error != "" {
		return nil, false, nil
	}
	if snap.Hashes == nil {
		return HashMap{}, true, nil
	}
	return HashMap(snap.Hashes), true, nil
}
