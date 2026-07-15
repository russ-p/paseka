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
	Invites   []InviteEntry   `json:"invites,omitempty"`
	Runtime   *RuntimeEntry   `json:"runtime,omitempty"`
}

// RuntimeEntry tracks the hive runtime (`paseka run`) for this colony on this machine.
type RuntimeEntry struct {
	PID             int       `json:"pid"`
	StartedAt       time.Time `json:"startedAt"`
	ColonyRoot      string    `json:"colonyRoot"`
	SubjectPrefix   string    `json:"subjectPrefix,omitempty"`
	Status          string    `json:"status"`
	LastHeartbeatAt time.Time `json:"lastHeartbeatAt,omitempty"`
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

// InviteEntry tracks one Human Gateway session invite.
type InviteEntry struct {
	InviteID    string          `json:"inviteId"`
	TraceID     string          `json:"traceId"`
	Bee         string          `json:"bee"`
	Intent      string          `json:"intent,omitempty"`
	Task        string          `json:"task"`
	Status      string          `json:"status"`
	ArtifactRef string          `json:"artifactRef,omitempty"`
	DoneWhen    *InviteDoneWhen `json:"doneWhen,omitempty"`
	SessionID   string          `json:"sessionId,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

const (
	InviteStatusPending    = "pending"
	InviteStatusAccepted   = "accepted"
	InviteStatusCancelled  = "cancelled"
	InviteStatusCompleted  = "completed"
	InviteStatusIncomplete = "incomplete"
	InviteStatusDeferred   = "deferred"
)

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

// UnregisterWorktree removes a worktree entry for a trace.
func UnregisterWorktree(slug, traceID string) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	out := st.Worktrees[:0]
	for _, w := range st.Worktrees {
		if w.TraceID != traceID {
			out = append(out, w)
		}
	}
	st.Worktrees = out
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

// RegisterRuntime records the active hive runtime process.
func RegisterRuntime(slug string, entry RuntimeEntry) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	st.Runtime = &entry
	return SaveState(slug, st)
}

// TouchRuntimeHeartbeat updates lastHeartbeatAt for the registered runtime when pid matches.
func TouchRuntimeHeartbeat(slug string, pid int, at time.Time) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	if st.Runtime == nil || st.Runtime.PID != pid {
		return nil
	}
	st.Runtime.LastHeartbeatAt = at
	if st.Runtime.Status == "" {
		st.Runtime.Status = "running"
	}
	return SaveState(slug, st)
}

// UnregisterRuntimeIfPID clears the runtime registry when the stored pid matches.
func UnregisterRuntimeIfPID(slug string, pid int) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	if st.Runtime == nil || st.Runtime.PID != pid {
		return nil
	}
	st.Runtime = nil
	return SaveState(slug, st)
}

// ClearRuntime removes any runtime registry entry.
func ClearRuntime(slug string) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	if st.Runtime == nil {
		return nil
	}
	st.Runtime = nil
	return SaveState(slug, st)
}

// RuntimeRegistry returns the persisted runtime entry, if any.
func RuntimeRegistry(slug string) (*RuntimeEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return nil, err
	}
	return st.Runtime, nil
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

// UpsertInvite stores or updates an invite entry by inviteId.
func UpsertInvite(slug string, entry InviteEntry) error {
	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	for i, inv := range st.Invites {
		if inv.InviteID == entry.InviteID {
			if st.Invites[i].CreatedAt.IsZero() {
				entry.CreatedAt = now
			} else {
				entry.CreatedAt = st.Invites[i].CreatedAt
			}
			st.Invites[i] = entry
			return SaveState(slug, st)
		}
	}
	st.Invites = append(st.Invites, entry)
	return SaveState(slug, st)
}

// ListInvites returns invite entries, optionally filtered by status and traceId.
func ListInvites(slug, status, traceID string) ([]InviteEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return nil, err
	}
	out := make([]InviteEntry, 0, len(st.Invites))
	for _, inv := range st.Invites {
		if status != "" && inv.Status != status {
			continue
		}
		if traceID != "" && inv.TraceID != traceID {
			continue
		}
		out = append(out, inv)
	}
	return out, nil
}

// FindInviteBySessionID returns an invite linked to a session, if any.
func FindInviteBySessionID(slug, sessionID string) (InviteEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return InviteEntry{}, err
	}
	for _, inv := range st.Invites {
		if inv.SessionID == sessionID {
			return inv, nil
		}
	}
	return InviteEntry{}, fmt.Errorf("colony: invite for session %q not found", sessionID)
}

// MarkInviteIncompleteOnSessionEnd marks an accepted invite incomplete when its session ends.
func MarkInviteIncompleteOnSessionEnd(slug, sessionID string) error {
	inv, err := FindInviteBySessionID(slug, sessionID)
	if err != nil {
		return nil
	}
	if inv.Status != InviteStatusAccepted {
		return nil
	}
	inv.Status = InviteStatusIncomplete
	return UpsertInvite(slug, inv)
}

// FindInvite returns one invite by ID.
func FindInvite(slug, inviteID string) (InviteEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return InviteEntry{}, err
	}
	for _, inv := range st.Invites {
		if inv.InviteID == inviteID {
			return inv, nil
		}
	}
	return InviteEntry{}, fmt.Errorf("colony: invite %q not found", inviteID)
}

func statePath(slug string) (string, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "state.json"), nil
}
