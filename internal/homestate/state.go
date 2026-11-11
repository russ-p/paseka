package homestate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
)

// State is persisted runtime state under ~/.config/paseka/<slug>/state.json.
type State struct {
	Worktrees    []WorktreeEntry    `json:"worktrees,omitempty"`
	PullRequests []PullRequestEntry `json:"pullRequests,omitempty"`
	Sessions     []SessionEntry     `json:"sessions,omitempty"`
	Invites      []InviteEntry      `json:"invites,omitempty"`
	Runtime      *RuntimeEntry      `json:"runtime,omitempty"`
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
	InviteID    string                 `json:"inviteId"`
	TraceID     string                 `json:"traceId"`
	Bee         string                 `json:"bee"`
	Intent      string                 `json:"intent,omitempty"`
	Task        string                 `json:"task"`
	Status      protocol.InviteStatus  `json:"status"`
	ArtifactRef string                 `json:"artifactRef,omitempty"`
	DoneWhen    *colony.InviteDoneWhen `json:"doneWhen,omitempty"`
	SessionID   string                 `json:"sessionId,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

// WorktreeEntry tracks one colony-managed git worktree.
type WorktreeEntry struct {
	TraceID   string    `json:"traceId"`
	Path      string    `json:"path"`
	BaseSHA   string    `json:"baseSha"`
	Branch    string    `json:"branch,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// PullRequestEntry is machine-local forge identity for one isolated trail (not ledger KV).
type PullRequestEntry struct {
	TraceID           string    `json:"traceId"`
	URL               string    `json:"url,omitempty"`
	Number            int       `json:"number,omitempty"`
	Head              string    `json:"head,omitempty"`
	Base              string    `json:"base,omitempty"`
	State             string    `json:"state,omitempty"`
	Draft             bool      `json:"draft,omitempty"`
	UpdatedAt         time.Time `json:"updatedAt,omitempty"`
	LastNotifiedState string    `json:"lastNotifiedState,omitempty"`
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
	// A zero-length file is a fresh registry, not a parse failure. Older
	// binaries could leave one behind by truncating the file in place, and the
	// writers below no longer do, but a read must not turn that history into a
	// hard error for every status command.
	if len(bytes.TrimSpace(data)) == 0 {
		return State{}, nil
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("homestate: parse state: %w", err)
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
	return writeStateFile(path, data)
}

// writeStateFile replaces state.json with a complete file in one step.
//
// The registry is read by other processes without a lock — status, the
// console dashboard, worktree listing — and an in-place truncate exposes an
// empty or half-written file to them. A same-directory temp file renamed over
// the target means a reader sees either the previous complete state or the next
// complete state, never a torn one.
func writeStateFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("homestate: mkdir %s: %w", dir, err)
	}
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	f, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("homestate: create temp state: %w", err)
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return fmt.Errorf("homestate: chmod temp state: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("homestate: write temp state: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("homestate: sync temp state: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("homestate: close temp state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("homestate: replace %s: %w", path, err)
	}
	return nil
}

// RegisterWorktree appends a worktree entry if the traceId is new.
func RegisterWorktree(slug string, entry WorktreeEntry) error {
	return withStateLock(slug, func(st *State) bool {
		for _, w := range st.Worktrees {
			if w.TraceID == entry.TraceID {
				return false
			}
		}
		st.Worktrees = append(st.Worktrees, entry)
		return true
	})
}

// UpdateWorktreeBranch sets the branch field for an existing worktree registry entry.
func UpdateWorktreeBranch(slug, traceID, branch string) error {
	return withStateLock(slug, func(st *State) bool {
		for i, w := range st.Worktrees {
			if w.TraceID == traceID {
				st.Worktrees[i].Branch = branch
				return true
			}
		}
		return false
	})
}

// FindPullRequest returns the machine-local PR identity for a trace, if any.
func FindPullRequest(slug, traceID string) (PullRequestEntry, bool, error) {
	st, err := LoadState(slug)
	if err != nil {
		return PullRequestEntry{}, false, err
	}
	for _, pr := range st.PullRequests {
		if pr.TraceID == traceID {
			return pr, true, nil
		}
	}
	return PullRequestEntry{}, false, nil
}

// UpsertPullRequest stores forge identity keyed by traceId.
func UpsertPullRequest(slug string, entry PullRequestEntry) error {
	if strings.TrimSpace(entry.TraceID) == "" {
		return fmt.Errorf("homestate: pull request traceId is required")
	}
	return withStateLock(slug, func(st *State) bool {
		if entry.UpdatedAt.IsZero() {
			entry.UpdatedAt = time.Now().UTC()
		}
		for i, pr := range st.PullRequests {
			if pr.TraceID == entry.TraceID {
				if entry.LastNotifiedState == "" {
					entry.LastNotifiedState = pr.LastNotifiedState
				}
				st.PullRequests[i] = entry
				return true
			}
		}
		st.PullRequests = append(st.PullRequests, entry)
		return true
	})
}

// ListPullRequests returns all machine-local PR identities.
func ListPullRequests(slug string) ([]PullRequestEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return nil, err
	}
	return st.PullRequests, nil
}

// MarkPullRequestNotified records the last Telegram/notify state for a published PR.
func MarkPullRequestNotified(slug, traceID, state string) error {
	return withStateLock(slug, func(st *State) bool {
		for i, pr := range st.PullRequests {
			if pr.TraceID == traceID {
				st.PullRequests[i].LastNotifiedState = state
				return true
			}
		}
		return false
	})
}

// RemovePullRequest drops machine-local PR identity after the trail is closed.
func RemovePullRequest(slug, traceID string) error {
	return withStateLock(slug, func(st *State) bool {
		out := make([]PullRequestEntry, 0, len(st.PullRequests))
		removed := false
		for _, pr := range st.PullRequests {
			if pr.TraceID == traceID {
				removed = true
				continue
			}
			out = append(out, pr)
		}
		if !removed {
			return false
		}
		st.PullRequests = out
		return true
	})
}

// UnregisterWorktree removes a worktree entry for a trace.
func UnregisterWorktree(slug, traceID string) error {
	return withStateLock(slug, func(st *State) bool {
		out := make([]WorktreeEntry, 0, len(st.Worktrees))
		removed := false
		for _, w := range st.Worktrees {
			if w.TraceID == traceID {
				removed = true
				continue
			}
			out = append(out, w)
		}
		if !removed {
			return false
		}
		st.Worktrees = out
		return true
	})
}

// RegisterSession records an active interactive session.
func RegisterSession(slug string, entry SessionEntry) error {
	return withStateLock(slug, func(st *State) bool {
		for i, s := range st.Sessions {
			if s.SessionID == entry.SessionID {
				st.Sessions[i] = entry
				return true
			}
		}
		st.Sessions = append(st.Sessions, entry)
		return true
	})
}

// UnregisterSession removes a session from the registry.
func UnregisterSession(slug, sessionID string) error {
	return withStateLock(slug, func(st *State) bool {
		out := make([]SessionEntry, 0, len(st.Sessions))
		removed := false
		for _, s := range st.Sessions {
			if s.SessionID == sessionID {
				removed = true
				continue
			}
			out = append(out, s)
		}
		if !removed {
			return false
		}
		st.Sessions = out
		return true
	})
}

// ListSessions returns persisted session entries.
func ListSessions(slug string) ([]SessionEntry, error) {
	st, err := LoadState(slug)
	if err != nil {
		return nil, err
	}
	return st.Sessions, nil
}

// withStateLock serializes read-modify-write access to state.json across
// processes. The hive runtime heartbeat (paseka run), the console supervisor,
// and CLI commands mutate the shared registry concurrently; without a lock a
// lost update can resurrect status:"running" in the middle of a stop or drop a
// worktree entry. mutate reports whether the state changed and must be written
// back.
//
// The lock lives on a dedicated state.lock file, not on state.json itself: the
// writer replaces state.json by rename, and a lock held on the replaced inode
// would stop excluding the next writer that opened the old file. Readers stay
// lock-free because every write publishes a complete file.
func withStateLock(slug string, mutate func(st *State) bool) error {
	path, err := statePath(slug)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("homestate: mkdir %s: %w", filepath.Dir(path), err)
	}
	lockPath := path + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("homestate: lock %s: %w", lockPath, err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("homestate: lock %s: %w", lockPath, err)
	}
	defer func() { _ = syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) }()

	st, err := LoadState(slug)
	if err != nil {
		return err
	}
	if !mutate(&st) {
		return nil
	}
	return SaveState(slug, st)
}

// RegisterRuntime records the active hive runtime process.
func RegisterRuntime(slug string, entry RuntimeEntry) error {
	return withStateLock(slug, func(st *State) bool {
		st.Runtime = &entry
		return true
	})
}

// TouchRuntimeHeartbeat updates lastHeartbeatAt for the registered runtime when pid matches.
// The heartbeat never owns lifecycle status: once a stop has been recorded, it
// must not re-introduce "running".
func TouchRuntimeHeartbeat(slug string, pid int, at time.Time) error {
	return withStateLock(slug, func(st *State) bool {
		if st.Runtime == nil || st.Runtime.PID != pid {
			return false
		}
		st.Runtime.LastHeartbeatAt = at
		if st.Runtime.Status == "" {
			st.Runtime.Status = "running"
		}
		return true
	})
}

// UnregisterRuntimeIfPID clears the runtime registry when the stored pid matches.
func UnregisterRuntimeIfPID(slug string, pid int) error {
	return withStateLock(slug, func(st *State) bool {
		if st.Runtime == nil || st.Runtime.PID != pid {
			return false
		}
		st.Runtime = nil
		return true
	})
}

// ClearRuntime removes any runtime registry entry.
func ClearRuntime(slug string) error {
	return withStateLock(slug, func(st *State) bool {
		if st.Runtime == nil {
			return false
		}
		st.Runtime = nil
		return true
	})
}

// RuntimeRegistry returns the persisted runtime entry, if any. It reads
// without the write lock so a missing registry resolves to nil, matching
// LoadState's tolerance of a yet-uncreated state.json.
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
	return SessionEntry{}, fmt.Errorf("homestate: session %q not found", sessionID)
}

// UpsertInvite stores or updates an invite entry by inviteId.
func UpsertInvite(slug string, entry InviteEntry) error {
	return withStateLock(slug, func(st *State) bool {
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
				return true
			}
		}
		st.Invites = append(st.Invites, entry)
		return true
	})
}

// ListInvites returns invite entries, optionally filtered by status and traceId.
func ListInvites(slug string, status protocol.InviteStatus, traceID string) ([]InviteEntry, error) {
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
	return InviteEntry{}, fmt.Errorf("homestate: invite for session %q not found", sessionID)
}

// MarkInviteIncompleteOnSessionEnd marks an accepted invite incomplete when its session ends.
func MarkInviteIncompleteOnSessionEnd(slug, sessionID string) error {
	return withStateLock(slug, func(st *State) bool {
		for i, inv := range st.Invites {
			if inv.SessionID != sessionID {
				continue
			}
			if inv.Status != protocol.InviteStatusAccepted {
				return false
			}
			st.Invites[i].Status = protocol.InviteStatusIncomplete
			st.Invites[i].UpdatedAt = time.Now().UTC()
			return true
		}
		return false
	})
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
	return InviteEntry{}, fmt.Errorf("homestate: invite %q not found", inviteID)
}

func statePath(slug string) (string, error) {
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "state.json"), nil
}
