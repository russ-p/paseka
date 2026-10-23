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
	ConfigRef     string
	// InitialCodeBaseSHA is the code base the fork worktree starts from.
	InitialCodeBaseSHA string
	// InitialCodeBaseDirty records whether that base matched a clean checkout.
	InitialCodeBaseDirty bool
	Actor                string
	RequestID            string
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
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = "cli"
	}

	sourceManifest, err := Load(opts.Context.ColonyRoot, opts.SourceTraceID)
	if err != nil {
		if errors.Is(err, ErrNoBinding) {
			return Manifest{}, fmt.Errorf("trailconfig: source trail %s has no configuration binding", opts.SourceTraceID)
		}
		return Manifest{}, err
	}
	if !retainsSnapshot(sourceManifest, opts.ConfigRef) {
		return Manifest{}, fmt.Errorf("trailconfig: source trail %s does not retain configuration %s", opts.SourceTraceID, opts.ConfigRef)
	}

	sourceDir := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.SourceTraceID), opts.ConfigRef)
	inventory, err := loadVerifiedSnapshot(sourceDir, opts.ConfigRef)
	if err != nil {
		return Manifest{}, err
	}
	contents, err := readSnapshotContents(sourceDir, inventory)
	if err != nil {
		return Manifest{}, err
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
		if existing.Policy == PolicyPinned && existing.ActiveConfigRef == opts.ConfigRef {
			return existing, nil
		}
		return Manifest{}, fmt.Errorf("trailconfig: trail %s already has a configuration binding", opts.TraceID)
	case !errors.Is(err, ErrNoBinding):
		return Manifest{}, err
	}

	if err := materializeSnapshot(dir, opts.ConfigRef, contents, inventory); err != nil {
		return Manifest{}, err
	}
	if err := verifySnapshot(filepath.Join(dir, SnapshotsDirName, opts.ConfigRef), opts.ConfigRef); err != nil {
		return Manifest{}, err
	}

	now := time.Now().UTC()
	manifest := Manifest{
		Version:                   manifestVersion,
		TraceID:                   opts.TraceID,
		InitialConfigRef:          opts.ConfigRef,
		ActiveConfigRef:           opts.ConfigRef,
		ConfigRevision:            1,
		Policy:                    PolicyPinned,
		InitialCodeBaseSHA:        strings.TrimSpace(opts.InitialCodeBaseSHA),
		InitialCodeBaseDirty:      opts.InitialCodeBaseDirty,
		InitialCodeBaseCapturedAt: now,
		CreatedAt:                 now,
		UpdatedAt:                 now,
		Snapshots: []Snapshot{{
			ConfigRef: opts.ConfigRef,
			Path:      filepath.ToSlash(filepath.Join(SnapshotsDirName, opts.ConfigRef)),
			Source:    "fork",
			CreatedAt: now,
			Files:     len(inventory.Entries),
		}},
		History: []Revision{{
			Revision:         1,
			InitialConfigRef: opts.ConfigRef,
			ActiveConfigRef:  opts.ConfigRef,
			Source:           "fork",
			ChangeID:         newChangeID(),
			RequestID:        opts.RequestID,
			Actor:            opts.Actor,
			Outcome:          OutcomeApplied,
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
