package trailconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
)

// ForkBindingOptions describes the source evidence a fork adopts as its own
// pinned configuration binding.
type ForkBindingOptions struct {
	Context       colony.Context
	SourceTraceID string
	TraceID       string
	// ConfigRef is the fork's active candidate configuration.
	ConfigRef string
	// BaseConfigRef is the source attempt's retained configuration. It defaults
	// to ConfigRef, which is the control-arm case.
	BaseConfigRef string
	// Candidate is the resolved patch-delta candidate. When nil, the source
	// snapshot itself is retained as the active configuration.
	Candidate *ForkDelta
	// InitialCodeBaseSHA is the code base the fork worktree starts from.
	InitialCodeBaseSHA string
	// InitialCodeBaseDirty records whether that base matched a clean checkout.
	InitialCodeBaseDirty bool
	// WorkspaceRoot is the fork's own checkout. It becomes the immutable
	// execution root for every run in the trail.
	WorkspaceRoot string
	Actor         string
	RequestID     string
}

// CreateForkBinding copies a retained source snapshot into a new trail and
// writes a pinned revision-1 manifest for it.
//
// The copy is published atomically and verified against the source content
// identity before the binding becomes readable. The fork never links back into
// the source trail, so deleting the source cannot invalidate it.
func CreateForkBinding(opts ForkBindingOptions) (Manifest, error) {
	if opts.Context.ColonyRoot == "" {
		return Manifest{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if strings.TrimSpace(opts.SourceTraceID) == "" {
		return Manifest{}, fmt.Errorf("trailconfig: source traceId is required")
	}
	if err := validateTraceID(opts.SourceTraceID); err != nil {
		return Manifest{}, err
	}
	if err := validateTraceID(opts.TraceID); err != nil {
		return Manifest{}, err
	}
	if opts.TraceID == opts.SourceTraceID {
		return Manifest{}, fmt.Errorf("trailconfig: fork traceId must differ from its source")
	}
	if err := validateConfigRef(opts.ConfigRef); err != nil {
		return Manifest{}, err
	}
	baseRef := strings.TrimSpace(opts.BaseConfigRef)
	if baseRef == "" {
		baseRef = opts.ConfigRef
	}
	if err := validateConfigRef(baseRef); err != nil {
		return Manifest{}, err
	}
	if opts.Candidate != nil && opts.Candidate.ConfigRef != opts.ConfigRef {
		return Manifest{}, fmt.Errorf("trailconfig: fork candidate %s does not match the requested configuration %s", opts.Candidate.ConfigRef, opts.ConfigRef)
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = "cli"
	}
	workspaceRoot := strings.TrimSpace(opts.WorkspaceRoot)
	if workspaceRoot == "" {
		return Manifest{}, fmt.Errorf("trailconfig: fork workspace root is required")
	}

	sourceManifest, err := Load(opts.Context.ColonyRoot, opts.SourceTraceID)
	if err != nil {
		if errors.Is(err, ErrNoBinding) {
			return Manifest{}, fmt.Errorf("trailconfig: source trail %s has no configuration binding", opts.SourceTraceID)
		}
		return Manifest{}, err
	}
	if !retainsSnapshot(sourceManifest, baseRef) {
		return Manifest{}, fmt.Errorf("trailconfig: source trail %s does not retain configuration %s", opts.SourceTraceID, baseRef)
	}

	sourceDir := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.SourceTraceID), baseRef)
	baseInventory, err := loadVerifiedSnapshot(sourceDir, baseRef)
	if err != nil {
		return Manifest{}, err
	}
	baseContents, err := readSnapshotContents(sourceDir, baseInventory)
	if err != nil {
		return Manifest{}, err
	}
	candidateInventory := baseInventory
	candidateContents := baseContents
	if opts.Candidate != nil {
		candidateInventory = opts.Candidate.Inventory
		candidateContents = opts.Candidate.Contents
	}

	dir := ConfigDir(opts.Context.ColonyRoot, opts.TraceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
	}
	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Manifest{}, err
	}
	defer release()

	existing, err := Load(opts.Context.ColonyRoot, opts.TraceID)
	switch {
	case err == nil:
		if existing.Policy == PolicyPinned && existing.InitialConfigRef == baseRef && existing.ActiveConfigRef == opts.ConfigRef {
			return existing, nil
		}
		return Manifest{}, fmt.Errorf("trailconfig: trail %s already has a configuration binding", opts.TraceID)
	case !errors.Is(err, ErrNoBinding):
		return Manifest{}, err
	}

	if err := materializeSnapshot(dir, baseRef, baseContents, baseInventory); err != nil {
		return Manifest{}, err
	}
	if err := verifySnapshot(filepath.Join(dir, SnapshotsDirName, baseRef), baseRef); err != nil {
		return Manifest{}, err
	}
	if opts.ConfigRef != baseRef {
		if err := materializeSnapshot(dir, opts.ConfigRef, candidateContents, candidateInventory); err != nil {
			return Manifest{}, err
		}
		if err := verifySnapshot(filepath.Join(dir, SnapshotsDirName, opts.ConfigRef), opts.ConfigRef); err != nil {
			return Manifest{}, err
		}
	}

	now := time.Now().UTC()
	snapshots := []Snapshot{{
		ConfigRef: baseRef,
		Path:      filepath.ToSlash(filepath.Join(SnapshotsDirName, baseRef)),
		Source:    "fork-base",
		CreatedAt: now,
		Files:     len(baseInventory.Entries),
	}}
	if opts.ConfigRef != baseRef {
		snapshots = append(snapshots, Snapshot{
			ConfigRef: opts.ConfigRef,
			Path:      filepath.ToSlash(filepath.Join(SnapshotsDirName, opts.ConfigRef)),
			Source:    "fork-delta",
			CreatedAt: now,
			Files:     len(candidateInventory.Entries),
		})
	}
	manifest := Manifest{
		Version:                   manifestVersion,
		TraceID:                   opts.TraceID,
		InitialConfigRef:          baseRef,
		ActiveConfigRef:           opts.ConfigRef,
		ConfigRevision:            1,
		Policy:                    PolicyPinned,
		InitialCodeBaseSHA:        strings.TrimSpace(opts.InitialCodeBaseSHA),
		InitialCodeBaseDirty:      opts.InitialCodeBaseDirty,
		InitialCodeBaseCapturedAt: now,
		WorkspaceRoot:             workspaceRoot,
		CreatedAt:                 now,
		UpdatedAt:                 now,
		Snapshots:                 snapshots,
		History: []Revision{{
			Revision:         1,
			InitialConfigRef: baseRef,
			ActiveConfigRef:  opts.ConfigRef,
			Source:           "fork",
			ChangeID:         newChangeID(),
			RequestID:        opts.RequestID,
			Actor:            opts.Actor,
			Outcome:          OutcomeApplied,
			OldConfigRef:     baseRef,
			RequestedAt:      now,
			CreatedAt:        now,
			NewConfigRef:     opts.ConfigRef,
		}},
	}
	if err := writeManifest(dir, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
