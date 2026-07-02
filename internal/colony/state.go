package colony

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State is persisted runtime state under ~/.config/paseka/<slug>/state.json.
type State struct {
	Worktrees []WorktreeEntry `json:"worktrees,omitempty"`
}

// WorktreeEntry tracks one colony-managed git worktree.
type WorktreeEntry struct {
	TraceID   string    `json:"traceId"`
	Path      string    `json:"path"`
	BaseSHA   string    `json:"baseSha"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// LoadState reads state.json for a colony slug.
func LoadState(slug string) (State, error) {
	path, err := statePath(slug)
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("colony: parse state: %w", err)
	}
	return st, nil
}

// SaveState writes state.json for a colony slug.
func SaveState(slug string, st State) error {
	path, err := statePath(slug)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// RegisterWorktree appends a worktree entry if the traceId is new.
func RegisterWorktree(slug string, entry WorktreeEntry) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	for _, w := range st.Worktrees {
		if w.TraceID == entry.TraceID {
			return nil
		}
	}
	st.Worktrees = append(st.Worktrees, entry)
	return SaveState(slug, st)
}

func statePath(slug string) (string, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "state.json"), nil
}
