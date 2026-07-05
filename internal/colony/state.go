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
	Sessions  []SessionEntry  `json:"sessions,omitempty"`
}

// SessionEntry tracks one interactive agent session.
type SessionEntry struct {
	SessionID string    `json:"sessionId"`
	TraceID   string    `json:"traceId"`
	AgentID   string    `json:"agentId"`
	RunDir    string    `json:"runDir"`
	Bee       string    `json:"bee"`
	PID       int       `json:"pid"`
	StartedAt time.Time `json:"startedAt"`
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

// RegisterSession records an active interactive session.
func RegisterSession(slug string, entry SessionEntry) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	for i, s := range st.Sessions {
		if s.SessionID == entry.SessionID {
			st.Sessions[i] = entry
			return SaveState(slug, st)
		}
	}
	st.Sessions = append(st.Sessions, entry)
	return SaveState(slug, st)
}

// UnregisterSession removes a session from the registry.
func UnregisterSession(slug, sessionID string) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	out := st.Sessions[:0]
	for _, s := range st.Sessions {
		if s.SessionID != sessionID {
			out = append(out, s)
		}
	}
	st.Sessions = out
	return SaveState(slug, st)
}

// ListSessions returns persisted session entries.
func ListSessions(slug string) ([]SessionEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return nil, err
	}
	return st.Sessions, nil
}

// FindSession returns a session entry by ID.
func FindSession(slug, sessionID string) (SessionEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return SessionEntry{}, err
	}
	for _, s := range st.Sessions {
		if s.SessionID == sessionID {
			return s, nil
		}
	}
	return SessionEntry{}, fmt.Errorf("colony: session %q not found", sessionID)
}

func statePath(slug string) (string, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "state.json"), nil
}
