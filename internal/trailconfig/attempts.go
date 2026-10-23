package trailconfig

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/colony"
)

const (
	// AttemptsFileName is the durable ownership ledger inside the config directory.
	AttemptsFileName = "attempts.json"
	attemptsVersion  = 1

	// AttemptStateOpen marks an attempt whose work may still receive rework, guard,
	// review, or HITL continuations.
	AttemptStateOpen = "open"
	// AttemptStateClosed marks an attempt whose execution window has ended.
	AttemptStateClosed = "closed"

	// AttemptKindTask owns a ledger task and its task-scoped direct gates.
	AttemptKindTask = "task"
	// AttemptKindDirect owns one independent direct-dispatch chain.
	AttemptKindDirect = "direct"
)

// TaskInputEvidence is the accepted task input captured when an attempt is admitted.
// Digest covers the executable input but not execution identity or status.
type TaskInputEvidence struct {
	TaskID string `json:"taskId"`
	Title  string `json:"title,omitempty"`
	Body   string `json:"body,omitempty"`
	Bee    string `json:"bee,omitempty"`
	Sector string `json:"sector,omitempty"`
	Intent string `json:"intent,omitempty"`
	Review string `json:"review,omitempty"`
	Digest string `json:"digest"`
}

// Attempt is one durable execution ownership record. It binds a task attempt or an
// independent direct chain to the configuration snapshot and code base it was admitted
// against.
type Attempt struct {
	SchemaVersion        int                `json:"schemaVersion"`
	AttemptID            string             `json:"attemptId"`
	TraceID              string             `json:"traceId"`
	TaskID               string             `json:"taskId,omitempty"`
	ChainKey             string             `json:"chainKey,omitempty"`
	Kind                 string             `json:"kind"`
	ConfigRef            string             `json:"configRef"`
	ConfigRevision       int                `json:"configRevision"`
	InitialCodeBaseSHA   string             `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty bool               `json:"initialCodeBaseDirty,omitempty"`
	TaskInput            *TaskInputEvidence `json:"taskInput,omitempty"`
	SourceEventID        string             `json:"sourceEventId,omitempty"`
	LastDispatchKey      string             `json:"lastDispatchKey,omitempty"`
	DispatchCount        int                `json:"dispatchCount,omitempty"`
	State                string             `json:"state"`
	CloseReason          string             `json:"closeReason,omitempty"`
	Agents               []string           `json:"agents,omitempty"`
	CreatedAt            time.Time          `json:"createdAt"`
	ClosedAt             *time.Time         `json:"closedAt,omitempty"`
}

// Open reports whether the attempt can still receive dispatch and rework.
func (a Attempt) Open() bool { return a.State == AttemptStateOpen }

type attemptsDoc struct {
	SchemaVersion int       `json:"schemaVersion"`
	Attempts      []Attempt `json:"attempts"`
}

var attemptsMu sync.Mutex

// AttemptsPath returns .paseka/runs/<traceId>/config/attempts.json.
func AttemptsPath(colonyRoot, traceID string) string {
	return filepath.Join(ConfigDir(colonyRoot, traceID), AttemptsFileName)
}

// NewTaskInputEvidence builds the canonical task-input evidence for an admission.
func NewTaskInputEvidence(taskID, title, body, bee, sector, intent, review string) *TaskInputEvidence {
	input := &TaskInputEvidence{
		TaskID: strings.TrimSpace(taskID),
		Title:  strings.TrimSpace(title),
		Body:   strings.TrimSpace(body),
		Bee:    strings.TrimSpace(bee),
		Sector: strings.TrimSpace(sector),
		Intent: strings.TrimSpace(intent),
		Review: strings.TrimSpace(review),
	}
	data, err := json.Marshal(input)
	if err == nil {
		sum := sha256.Sum256(data)
		input.Digest = configRefPrefix + hex.EncodeToString(sum[:])
	}
	return input
}

// EnsureTaskAttempt returns the open attempt for taskID, admitting a new one when the
// previous attempt closed. A replayed admission event from the closed attempt returns
// that closed attempt so the caller can skip it instead of reopening work.
func EnsureTaskAttempt(ctx colony.Context, traceID, taskID, sourceEventID string, input *TaskInputEvidence) (Attempt, error) {
	if strings.TrimSpace(taskID) == "" {
		return Attempt{}, fmt.Errorf("trailconfig: task id is required")
	}
	m, err := Load(ctx.ColonyRoot, traceID)
	if err != nil {
		return Attempt{}, err
	}
	entry := Attempt{}
	err = withAttempts(ctx.ColonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		latest := latestTask(entries, taskID)
		if latest.AttemptID != "" {
			if latest.Open() {
				entry = latest
				if entry.TaskInput == nil {
					entry.TaskInput = input
					replaceAttempt(entries, entry)
				}
				return entries, nil
			}
			if sourceEventID != "" && (latest.SourceEventID == sourceEventID || latest.LastDispatchKey == sourceEventID) {
				entry = latest
				return entries, nil
			}
		}
		entry = newAttempt(m, traceID, taskID, "", AttemptKindTask, sourceEventID, input)
		return append(entries, entry), nil
	})
	return entry, err
}

// AdmitChainAttempt returns the existing ownership for chainKey or records a new one
// against the active configuration. A direct chain is admitted once; replaying the same
// event returns the same attempt.
func AdmitChainAttempt(ctx colony.Context, traceID, chainKey, taskID, sourceEventID string) (Attempt, bool, error) {
	if strings.TrimSpace(chainKey) == "" {
		return Attempt{}, false, fmt.Errorf("trailconfig: chain key is required")
	}
	m, err := Load(ctx.ColonyRoot, traceID)
	if err != nil {
		return Attempt{}, false, err
	}
	entry := Attempt{}
	created := false
	err = withAttempts(ctx.ColonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		for _, existing := range entries {
			if existing.ChainKey == chainKey {
				entry = existing
				return entries, nil
			}
		}
		entry = newAttempt(m, traceID, taskID, chainKey, AttemptKindDirect, sourceEventID, nil)
		created = true
		return append(entries, entry), nil
	})
	return entry, created, err
}

func newAttempt(m Manifest, traceID, taskID, chainKey, kind, sourceEventID string, input *TaskInputEvidence) Attempt {
	return Attempt{
		SchemaVersion:        attemptsVersion,
		AttemptID:            newAttemptID(),
		TraceID:              traceID,
		TaskID:               taskID,
		ChainKey:             chainKey,
		Kind:                 kind,
		ConfigRef:            m.ActiveConfigRef,
		ConfigRevision:       m.ConfigRevision,
		InitialCodeBaseSHA:   m.InitialCodeBaseSHA,
		InitialCodeBaseDirty: m.InitialCodeBaseDirty,
		TaskInput:            input,
		SourceEventID:        sourceEventID,
		State:                AttemptStateOpen,
		CreatedAt:            time.Now().UTC(),
	}
}

// MarkAttemptDispatched records an execution start. duplicate is true when the same
// dispatch key already ran for this attempt, which is how a replayed bus event stops
// reopening work after a restart.
func MarkAttemptDispatched(colonyRoot, traceID, attemptID, dispatchKey string) (Attempt, bool, error) {
	if attemptID == "" {
		return Attempt{}, false, fmt.Errorf("trailconfig: attempt id is required")
	}
	entry := Attempt{}
	duplicate := false
	err := withAttempts(colonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		for i, existing := range entries {
			if existing.AttemptID != attemptID {
				continue
			}
			if existing.State != AttemptStateOpen {
				entry = existing
				duplicate = true
				return entries, nil
			}
			if dispatchKey != "" && existing.LastDispatchKey == dispatchKey && existing.DispatchCount > 0 {
				entry = existing
				duplicate = true
				return entries, nil
			}
			existing.LastDispatchKey = dispatchKey
			existing.DispatchCount++
			entries[i] = existing
			entry = existing
			return entries, nil
		}
		return entries, fmt.Errorf("trailconfig: attempt %s not found", attemptID)
	})
	return entry, duplicate, err
}

// RecordAttemptAgent remembers an agent run that belongs to the attempt.
func RecordAttemptAgent(colonyRoot, traceID, attemptID, agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if attemptID == "" || agentID == "" {
		return nil
	}
	return withAttempts(colonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		for i, existing := range entries {
			if existing.AttemptID != attemptID {
				continue
			}
			for _, known := range existing.Agents {
				if known == agentID {
					return entries, nil
				}
			}
			existing.Agents = append(existing.Agents, agentID)
			entries[i] = existing
			return entries, nil
		}
		return entries, nil
	})
}

// CloseAttempt ends one attempt. Closing an already closed attempt is idempotent.
func CloseAttempt(colonyRoot, traceID, attemptID, reason string) error {
	if attemptID == "" {
		return nil
	}
	return withAttempts(colonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		for i, existing := range entries {
			if existing.AttemptID != attemptID || !existing.Open() {
				continue
			}
			now := time.Now().UTC()
			existing.State = AttemptStateClosed
			existing.CloseReason = strings.TrimSpace(reason)
			existing.ClosedAt = &now
			entries[i] = existing
		}
		return entries, nil
	})
}

// CloseTaskAttempts closes every open attempt for a task.
func CloseTaskAttempts(colonyRoot, traceID, taskID, reason string) error {
	if taskID == "" {
		return nil
	}
	return withAttempts(colonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		now := time.Now().UTC()
		for i, existing := range entries {
			if existing.TaskID != taskID || !existing.Open() {
				continue
			}
			existing.State = AttemptStateClosed
			existing.CloseReason = strings.TrimSpace(reason)
			existing.ClosedAt = &now
			entries[i] = existing
		}
		return entries, nil
	})
}

// CloseTraceAttempts closes every open attempt belonging to a killed trace.
func CloseTraceAttempts(colonyRoot, traceID, reason string) error {
	return withAttempts(colonyRoot, traceID, func(entries []Attempt) ([]Attempt, error) {
		now := time.Now().UTC()
		for i, existing := range entries {
			if !existing.Open() {
				continue
			}
			existing.State = AttemptStateClosed
			existing.CloseReason = strings.TrimSpace(reason)
			existing.ClosedAt = &now
			entries[i] = existing
		}
		return entries, nil
	})
}

// FindAttempt returns one attempt by id.
func FindAttempt(colonyRoot, traceID, attemptID string) (Attempt, bool, error) {
	entries, err := loadAttempts(colonyRoot, traceID)
	if err != nil {
		return Attempt{}, false, err
	}
	for _, entry := range entries {
		if entry.AttemptID == attemptID {
			return entry, true, nil
		}
	}
	return Attempt{}, false, nil
}

// LatestTaskAttempt returns the newest attempt recorded for a task.
func LatestTaskAttempt(colonyRoot, traceID, taskID string) (Attempt, bool, error) {
	entries, err := loadAttempts(colonyRoot, traceID)
	if err != nil {
		return Attempt{}, false, err
	}
	latest := latestTask(entries, taskID)
	return latest, latest.AttemptID != "", nil
}

// ChainAttempt returns the recorded ownership for one direct chain.
func ChainAttempt(colonyRoot, traceID, chainKey string) (Attempt, bool, error) {
	entries, err := loadAttempts(colonyRoot, traceID)
	if err != nil {
		return Attempt{}, false, err
	}
	for _, entry := range entries {
		if entry.ChainKey == chainKey {
			return entry, true, nil
		}
	}
	return Attempt{}, false, nil
}

// Attempts returns every recorded attempt, oldest first.
func Attempts(colonyRoot, traceID string) ([]Attempt, error) {
	entries, err := loadAttempts(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].AttemptID < entries[j].AttemptID
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
	return entries, nil
}

func latestTask(entries []Attempt, taskID string) Attempt {
	var latest Attempt
	for _, entry := range entries {
		if entry.TaskID != taskID || entry.Kind != AttemptKindTask {
			continue
		}
		if latest.AttemptID == "" || entry.CreatedAt.After(latest.CreatedAt) {
			latest = entry
		}
	}
	return latest
}

func replaceAttempt(entries []Attempt, entry Attempt) {
	for i, existing := range entries {
		if existing.AttemptID == entry.AttemptID {
			entries[i] = entry
			return
		}
	}
}

func loadAttempts(colonyRoot, traceID string) ([]Attempt, error) {
	data, err := os.ReadFile(AttemptsPath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("trailconfig: read attempts: %w", err)
	}
	var doc attemptsDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("trailconfig: parse attempts: %w", err)
	}
	return doc.Attempts, nil
}

func withAttempts(colonyRoot, traceID string, mutate func([]Attempt) ([]Attempt, error)) error {
	if colonyRoot == "" {
		return fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return err
	}
	dir := ConfigDir(colonyRoot, traceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
	}

	attemptsMu.Lock()
	defer attemptsMu.Unlock()

	release, err := acquireLock(filepath.Join(dir, ".attempts.lock"))
	if err != nil {
		return err
	}
	defer release()

	entries, err := loadAttempts(colonyRoot, traceID)
	if err != nil {
		return err
	}
	entries, err = mutate(entries)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	return saveAttempts(dir, attemptsDoc{SchemaVersion: attemptsVersion, Attempts: entries})
}

func saveAttempts(configDir string, doc attemptsDoc) error {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("trailconfig: encode attempts: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(configDir, ".attempts-*.json")
	if err != nil {
		return fmt.Errorf("trailconfig: create attempts temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailconfig: write attempts: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailconfig: sync attempts: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trailconfig: close attempts: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(configDir, AttemptsFileName)); err != nil {
		return fmt.Errorf("trailconfig: publish attempts: %w", err)
	}
	return nil
}

func newAttemptID() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("attempt-%d", time.Now().UnixNano())
	}
	return "attempt-" + hex.EncodeToString(buf[:])
}
