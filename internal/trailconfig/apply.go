package trailconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"gopkg.in/yaml.v3"
)

// Apply sources select where a candidate configuration comes from.
const (
	SourceColony    = "colony"
	SourceOverrides = "overrides"
	SourceSnapshot  = "snapshot"
)

// Apply outcomes distinguish a queued action from one that already changed the
// trail and from a request the colony refused.
const (
	OutcomePending   = "pending"
	OutcomeApplied   = "applied"
	OutcomeUnchanged = "unchanged"
	OutcomeRejected  = "rejected"
	OutcomeCancelled = "cancelled"
)

// Override is one validated trail-local behavioural setting applied on top of a
// retained snapshot. It cannot change colony identity, transport, credentials,
// budget, or delivery policy.
type Override struct {
	Role  string
	Key   string
	Value string
}

// ApplyOptions describes one operator apply request.
type ApplyOptions struct {
	Context     colony.Context
	TraceID     string
	Source      string
	SnapshotRef string
	Overrides   []Override
	Actor       string
	Reason      string
	RequestID   string
	// ExpectedRevision, when set, refuses the request unless the trail is still
	// at that revision.
	ExpectedRevision *int
	// Blockers hold activation when current execution still needs the old
	// configuration. They are recorded with a pending action.
	Blockers []string
	DryRun   bool
}

// ApplyResult is the preview and recorded outcome of one apply request.
type ApplyResult struct {
	Manifest Manifest
	Change   Change
	NewRef   string
	Diff     []string
	DryRun   bool
}

type preparedCandidate struct {
	ref       string
	inventory Inventory
	contents  []capturedFile
	tree      string
	cleanup   func()
}

// Apply prepares a candidate from the selected source, validates remaining
// unstarted work, records the request, and advances the active configuration.
//
// Code, task progress, workspace, and honey are never touched: the operation
// owns only the trail's configuration manifest and immutable snapshots.
func Apply(opts ApplyOptions) (ApplyResult, error) {
	if opts.Context.ColonyRoot == "" {
		return ApplyResult{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(opts.TraceID); err != nil {
		return ApplyResult{}, err
	}
	if strings.TrimSpace(opts.Actor) == "" {
		opts.Actor = "cli"
	}
	if opts.Source == "" {
		return ApplyResult{}, fmt.Errorf("trailconfig: apply source is required")
	}
	if err := validateApplySource(opts); err != nil {
		return ApplyResult{}, err
	}

	dir := ConfigDir(opts.Context.ColonyRoot, opts.TraceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ApplyResult{}, fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
	}
	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return ApplyResult{}, err
	}
	defer release()

	manifest, err := Load(opts.Context.ColonyRoot, opts.TraceID)
	if err != nil {
		if err == ErrNoBinding {
			return ApplyResult{}, fmt.Errorf("trailconfig: trail %s has no configuration binding", opts.TraceID)
		}
		return ApplyResult{}, err
	}
	if manifest.Policy == PolicyPinned {
		return ApplyResult{}, fmt.Errorf("trailconfig: trail %s is pinned; configuration application is refused", opts.TraceID)
	}

	// Duplicate delivery returns the recorded result. Reusing the same request
	// identity for different contents is refused instead of silently rewriting
	// an audited action.
	if opts.RequestID != "" {
		if old, ok := findChangeByRequest(manifest, opts.RequestID); ok {
			if old.RequestFingerprint != "" && old.RequestFingerprint != opts.requestFingerprint() {
				return ApplyResult{}, fmt.Errorf("trailconfig: request id %s was already used for a different configuration request", opts.RequestID)
			}
			return ApplyResult{Manifest: manifest, Change: old, NewRef: old.NewConfigRef, DryRun: opts.DryRun}, nil
		}
	}

	if opts.ExpectedRevision != nil && *opts.ExpectedRevision != manifest.ConfigRevision {
		return recordRejection(opts, manifest, fmt.Sprintf("expected revision %d, trail is at %d", *opts.ExpectedRevision, manifest.ConfigRevision))
	}
	if manifest.Pending != nil {
		return recordRejection(opts, manifest, "another configuration change is pending")
	}

	candidate, err := prepareCandidate(opts, manifest.ActiveConfigRef)
	if err != nil {
		return ApplyResult{}, err
	}
	if candidate.cleanup != nil {
		defer candidate.cleanup()
	}

	snapshot, err := runs.LoadTraceTasksFromFS(opts.Context.ColonyRoot, opts.TraceID)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("trailconfig: load remaining tasks: %w", err)
	}
	if err := ValidateRemaining(candidate.tree, snapshot); err != nil {
		return ApplyResult{}, err
	}

	baseInventory, err := loadVerifiedSnapshot(filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.TraceID), manifest.ActiveConfigRef), manifest.ActiveConfigRef)
	if err != nil {
		return ApplyResult{}, err
	}
	diff := diffInventories(baseInventory.Entries, candidate.inventory.Entries)
	result := ApplyResult{
		Manifest: manifest,
		NewRef:   candidate.ref,
		Diff:     diff,
		DryRun:   opts.DryRun,
	}

	if opts.DryRun {
		change := newChange(opts, manifest, candidate.ref)
		change.Outcome = OutcomePending
		change.Detail = "dry run"
		result.Change = change
		return result, nil
	}

	if candidate.ref == manifest.ActiveConfigRef {
		change := newChange(opts, manifest, candidate.ref)
		change.Outcome = OutcomeUnchanged
		change.Revision = manifest.ConfigRevision
		change.ActiveConfigRef = manifest.ActiveConfigRef
		manifest.History = append(manifest.History, change)
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(dir, manifest); err != nil {
			return ApplyResult{}, err
		}
		result.Manifest = manifest
		result.Change = change
		return result, nil
	}

	if candidate.contents != nil {
		if err := materializeSnapshot(dir, candidate.ref, candidate.contents, candidate.inventory); err != nil {
			return ApplyResult{}, err
		}
	}
	if err := verifySnapshot(filepath.Join(dir, SnapshotsDirName, candidate.ref), candidate.ref); err != nil {
		return ApplyResult{}, err
	}
	addSnapshot(&manifest, Snapshot{
		ConfigRef: candidate.ref,
		Path:      filepath.ToSlash(filepath.Join(SnapshotsDirName, candidate.ref)),
		Source:    opts.Source,
		CreatedAt: time.Now().UTC(),
		Files:     len(candidate.inventory.Entries),
	})

	change := newChange(opts, manifest, candidate.ref)
	if len(opts.Blockers) > 0 {
		change.Outcome = OutcomePending
		change.Blockers = append([]string(nil), opts.Blockers...)
		manifest.Pending = &change
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(dir, manifest); err != nil {
			return ApplyResult{}, err
		}
		result.Manifest = manifest
		result.Change = change
		return result, nil
	}

	activate(&manifest, &change)
	if err := writeManifest(dir, manifest); err != nil {
		return ApplyResult{}, err
	}
	result.Manifest = manifest
	result.Change = change
	return result, nil
}

func validateApplySource(opts ApplyOptions) error {
	switch opts.Source {
	case SourceColony:
		if opts.SnapshotRef != "" || len(opts.Overrides) > 0 {
			return fmt.Errorf("trailconfig: colony capture cannot be combined with a snapshot or overrides")
		}
	case SourceSnapshot:
		if strings.TrimSpace(opts.SnapshotRef) == "" {
			return fmt.Errorf("trailconfig: snapshot ref is required")
		}
		if len(opts.Overrides) > 0 {
			return fmt.Errorf("trailconfig: overrides cannot be combined with a retained snapshot")
		}
	case SourceOverrides:
		if len(opts.Overrides) == 0 {
			return fmt.Errorf("trailconfig: at least one override is required")
		}
		if opts.SnapshotRef != "" {
			return fmt.Errorf("trailconfig: overrides derive from the active snapshot and cannot name another snapshot")
		}
	default:
		return fmt.Errorf("trailconfig: unknown apply source %q", opts.Source)
	}
	return nil
}

func prepareCandidate(opts ApplyOptions, activeRef string) (preparedCandidate, error) {
	switch opts.Source {
	case SourceColony:
		contents, resolved, err := capture(opts.Context)
		if err != nil {
			return preparedCandidate{}, err
		}
		inventory := buildInventory(contents, resolved)
		ref, err := configRef(inventory)
		if err != nil {
			return preparedCandidate{}, err
		}
		tree, cleanup, err := materializeCandidateTree(ref, contents, inventory)
		if err != nil {
			return preparedCandidate{}, err
		}
		return preparedCandidate{ref: ref, inventory: inventory, contents: contents, tree: tree, cleanup: cleanup}, nil
	case SourceSnapshot:
		snapshotPath := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.TraceID), opts.SnapshotRef)
		inventory, err := loadVerifiedSnapshot(snapshotPath, opts.SnapshotRef)
		if err != nil {
			return preparedCandidate{}, err
		}
		return preparedCandidate{
			ref:       opts.SnapshotRef,
			inventory: inventory,
			tree:      filepath.Join(snapshotPath, SnapshotTreeDirName),
		}, nil
	case SourceOverrides:
		baseDir := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.TraceID), activeRef)
		inventory, err := loadVerifiedSnapshot(baseDir, activeRef)
		if err != nil {
			return preparedCandidate{}, err
		}
		contents, err := readSnapshotContents(baseDir, inventory)
		if err != nil {
			return preparedCandidate{}, err
		}
		tmpDir, err := os.MkdirTemp("", "paseka-config-overrides-")
		if err != nil {
			return preparedCandidate{}, fmt.Errorf("trailconfig: create candidate temp: %w", err)
		}
		cleanup := func() { _ = os.RemoveAll(tmpDir) }
		if err := materializeSnapshot(tmpDir, activeRef, contents, inventory); err != nil {
			cleanup()
			return preparedCandidate{}, err
		}
		tree := filepath.Join(tmpDir, SnapshotsDirName, activeRef, SnapshotTreeDirName)
		if err := applyOverrides(tree, opts.Overrides); err != nil {
			cleanup()
			return preparedCandidate{}, err
		}
		updated, err := readCapturedTree(tree)
		if err != nil {
			cleanup()
			return preparedCandidate{}, err
		}
		updatedInventory := buildInventory(updated, inventory.Resolved)
		ref, err := configRef(updatedInventory)
		if err != nil {
			cleanup()
			return preparedCandidate{}, err
		}
		return preparedCandidate{ref: ref, inventory: updatedInventory, contents: updated, tree: tree, cleanup: cleanup}, nil
	default:
		return preparedCandidate{}, fmt.Errorf("trailconfig: unknown apply source %q", opts.Source)
	}
}

func materializeCandidateTree(ref string, contents []capturedFile, inventory Inventory) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "paseka-config-candidate-")
	if err != nil {
		return "", nil, fmt.Errorf("trailconfig: create candidate temp: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	if err := materializeSnapshot(tmpDir, ref, contents, inventory); err != nil {
		cleanup()
		return "", nil, err
	}
	return filepath.Join(tmpDir, SnapshotsDirName, ref, SnapshotTreeDirName), cleanup, nil
}

// ValidateRemaining checks that a candidate still satisfies every unstarted
// accepted task that must dispatch after activation.
func ValidateRemaining(candidateRoot string, snapshot taskledger.TraceSnapshot) error {
	manifest, err := colony.LoadColony(candidateRoot)
	if err != nil {
		return fmt.Errorf("trailconfig: load candidate colony: %w", err)
	}
	bees, err := colony.LoadAllBees(candidateRoot)
	if err != nil {
		return fmt.Errorf("trailconfig: load candidate bees: %w", err)
	}
	for _, task := range snapshot.Tasks {
		if taskledger.IsFinalReviewTask(task) {
			continue
		}
		switch task.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady:
		default:
			continue
		}
		beeRole := colony.EffectiveTaskBee(task.Bee, manifest.Defaults)
		bee, ok := bees[beeRole]
		if !ok {
			return fmt.Errorf("trailconfig: candidate removes bee %q needed by remaining task %s", beeRole, task.TaskID)
		}
		if err := bee.ValidateAdapterRequirements(); err != nil {
			return fmt.Errorf("trailconfig: candidate invalid for remaining task %s: %w", task.TaskID, err)
		}
		if task.Sector != "" {
			if _, err := manifest.ResolveSector(task.Sector); err != nil {
				return fmt.Errorf("trailconfig: candidate invalid for remaining task %s: %w", task.TaskID, err)
			}
		}
	}
	return nil
}

// TryActivate advances a pending change when its blockers have cleared.
func TryActivate(colonyRoot, traceID string, blockers []string) (Manifest, bool, error) {
	dir := ConfigDir(colonyRoot, traceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, false, err
	}
	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Manifest{}, false, err
	}
	defer release()

	manifest, err := Load(colonyRoot, traceID)
	if err != nil {
		return Manifest{}, false, err
	}
	if manifest.Pending == nil {
		return manifest, false, nil
	}
	if len(blockers) > 0 {
		manifest.Pending.Blockers = append([]string(nil), blockers...)
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(dir, manifest); err != nil {
			return Manifest{}, false, err
		}
		return manifest, false, nil
	}
	change := *manifest.Pending
	if change.ExpectedRevision != manifest.ConfigRevision {
		change.Outcome = OutcomeRejected
		change.Detail = fmt.Sprintf("revision moved to %d while waiting", manifest.ConfigRevision)
		manifest.Pending = nil
		manifest.History = append(manifest.History, change)
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(dir, manifest); err != nil {
			return Manifest{}, false, err
		}
		return manifest, false, nil
	}
	snapshot, err := runs.LoadTraceTasksFromFS(colonyRoot, traceID)
	if err != nil {
		return Manifest{}, false, err
	}
	tree := filepath.Join(SnapshotDir(colonyRoot, traceID), change.NewConfigRef, SnapshotTreeDirName)
	if err := ValidateRemaining(tree, snapshot); err != nil {
		change.Outcome = OutcomeRejected
		change.Detail = err.Error()
		manifest.Pending = nil
		manifest.History = append(manifest.History, change)
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(dir, manifest); err != nil {
			return Manifest{}, false, err
		}
		return manifest, false, nil
	}
	manifest.Pending = nil
	activate(&manifest, &change)
	if err := writeManifest(dir, manifest); err != nil {
		return Manifest{}, false, err
	}
	return manifest, true, nil
}

// Cancel records a pending change as cancelled without changing the active
// configuration.
func Cancel(colonyRoot, traceID, changeID, actor, reason string) (Manifest, error) {
	dir := ConfigDir(colonyRoot, traceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, err
	}
	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Manifest{}, err
	}
	defer release()

	manifest, err := Load(colonyRoot, traceID)
	if err != nil {
		return Manifest{}, err
	}
	if manifest.Pending == nil {
		return Manifest{}, fmt.Errorf("trailconfig: trail %s has no pending configuration change", traceID)
	}
	if changeID != "" && manifest.Pending.ChangeID != changeID {
		return Manifest{}, fmt.Errorf("trailconfig: pending change is %s, not %s", manifest.Pending.ChangeID, changeID)
	}
	change := *manifest.Pending
	change.Outcome = OutcomeCancelled
	if strings.TrimSpace(actor) != "" {
		change.Actor = actor
	}
	if strings.TrimSpace(reason) != "" {
		change.Reason = reason
	}
	manifest.Pending = nil
	manifest.History = append(manifest.History, change)
	manifest.UpdatedAt = time.Now().UTC()
	if err := writeManifest(dir, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Blockers reports execution that still needs the old configuration.
func Blockers(colonyRoot, traceID string) ([]string, error) {
	var blockers []string
	if snapshot, err := runs.LoadTraceTasksFromFS(colonyRoot, traceID); err == nil {
		for _, task := range snapshot.Tasks {
			switch task.Status {
			case protocol.TaskStatusRunning, protocol.TaskStatusWaitingReview:
				blockers = append(blockers, "task "+task.TaskID+" is "+string(task.Status))
			}
		}
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		switch entry.Name() {
		case ConfigDirName, "tasks", "artifacts", "proposals", "console":
			continue
		}
		data, err := os.ReadFile(filepath.Join(runsRoot, entry.Name(), runs.StatusFileName))
		if err != nil {
			continue
		}
		var status struct {
			State string `json:"state"`
		}
		if err := json.Unmarshal(data, &status); err != nil {
			continue
		}
		if status.State == "queued" || status.State == "running" {
			blockers = append(blockers, "run "+entry.Name()+" is "+status.State)
		}
	}
	return uniqueBlockers(blockers), nil
}

func uniqueBlockers(blockers []string) []string {
	seen := make(map[string]struct{}, len(blockers))
	out := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		if _, ok := seen[blocker]; ok {
			continue
		}
		seen[blocker] = struct{}{}
		out = append(out, blocker)
	}
	return out
}

func newChange(opts ApplyOptions, manifest Manifest, newRef string) Change {
	revision := manifest.ConfigRevision
	if opts.ExpectedRevision != nil {
		revision = *opts.ExpectedRevision
	}
	now := time.Now().UTC()
	return Change{
		Revision:           manifest.ConfigRevision,
		InitialConfigRef:   manifest.InitialConfigRef,
		ActiveConfigRef:    manifest.ActiveConfigRef,
		Source:             opts.Source,
		ChangeID:           newChangeID(),
		RequestID:          opts.RequestID,
		RequestFingerprint: opts.requestFingerprint(),
		Actor:              opts.Actor,
		Reason:             opts.Reason,
		ExpectedRevision:   revision,
		OldConfigRef:       manifest.ActiveConfigRef,
		NewConfigRef:       newRef,
		RequestedAt:        now,
		CreatedAt:          now,
	}
}

func recordRejection(opts ApplyOptions, manifest Manifest, detail string) (ApplyResult, error) {
	change := newChange(opts, manifest, "")
	change.Outcome = OutcomeRejected
	change.Detail = detail
	change.Revision = manifest.ConfigRevision
	if !opts.DryRun {
		manifest.History = append(manifest.History, change)
		manifest.UpdatedAt = time.Now().UTC()
		if err := writeManifest(ConfigDir(opts.Context.ColonyRoot, opts.TraceID), manifest); err != nil {
			return ApplyResult{}, err
		}
	}
	return ApplyResult{Manifest: manifest, Change: change, DryRun: opts.DryRun}, nil
}

func activate(manifest *Manifest, change *Change) {
	now := time.Now().UTC()
	manifest.ActiveConfigRef = change.NewConfigRef
	manifest.ConfigRevision++
	change.Revision = manifest.ConfigRevision
	change.ActiveConfigRef = change.NewConfigRef
	change.InitialConfigRef = manifest.InitialConfigRef
	change.Outcome = OutcomeApplied
	change.AppliedAt = &now
	change.Blockers = nil
	manifest.Pending = nil
	manifest.History = append(manifest.History, *change)
	manifest.UpdatedAt = now
}

func findChangeByRequest(manifest Manifest, requestID string) (Change, bool) {
	if manifest.Pending != nil && manifest.Pending.RequestID == requestID {
		return *manifest.Pending, true
	}
	for _, change := range manifest.History {
		if change.RequestID == requestID {
			return change, true
		}
	}
	return Change{}, false
}

func (o ApplyOptions) requestFingerprint() string {
	if o.RequestID == "" {
		return ""
	}
	// Expected revision and actor/reason are not part of the requested
	// contents: a retried delivery may arrive after the revision advanced and
	// still must return the recorded result.
	payload := struct {
		Source      string     `json:"source"`
		SnapshotRef string     `json:"snapshotRef,omitempty"`
		Overrides   []Override `json:"overrides,omitempty"`
	}{
		Source:      o.Source,
		SnapshotRef: o.SnapshotRef,
		Overrides:   o.Overrides,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return configRefPrefix + hex.EncodeToString(sum[:])
}

func newChangeID() string {
	id, err := colony.NewTraceID()
	if err != nil {
		return fmt.Sprintf("change-%d", time.Now().UnixNano())
	}
	return "change-" + strings.TrimPrefix(id, "trace-")
}

func addSnapshot(manifest *Manifest, snapshot Snapshot) {
	for i, existing := range manifest.Snapshots {
		if existing.ConfigRef != snapshot.ConfigRef {
			continue
		}
		if manifest.Snapshots[i].Source == "" {
			manifest.Snapshots[i].Source = snapshot.Source
		}
		return
	}
	manifest.Snapshots = append(manifest.Snapshots, snapshot)
}

func readSnapshotContents(snapshotDir string, inventory Inventory) ([]capturedFile, error) {
	contents := make([]capturedFile, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		path := filepath.Join(snapshotDir, SnapshotTreeDirName, filepath.FromSlash(entry.Path))
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("trailconfig: read retained %s: %w", entry.Path, err)
		}
		contents = append(contents, capturedFile{path: entry.Path, mode: entry.Mode, data: data})
	}
	sort.Slice(contents, func(i, j int) bool { return contents[i].path < contents[j].path })
	return contents, nil
}

func readCapturedTree(root string) ([]capturedFile, error) {
	var contents []capturedFile
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		contents = append(contents, capturedFile{
			path: filepath.ToSlash(rel),
			mode: uint32(info.Mode().Perm()),
			data: data,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("trailconfig: read candidate tree: %w", err)
	}
	sort.Slice(contents, func(i, j int) bool { return contents[i].path < contents[j].path })
	return contents, nil
}

func applyOverrides(treeRoot string, overrides []Override) error {
	bees, err := colony.LoadAllBees(treeRoot)
	if err != nil {
		return fmt.Errorf("trailconfig: load candidate bees: %w", err)
	}
	for _, override := range overrides {
		role := strings.TrimSpace(override.Role)
		if role == "" {
			return fmt.Errorf("trailconfig: override role is required")
		}
		key := strings.TrimSpace(override.Key)
		if key == "" {
			return fmt.Errorf("trailconfig: override key is required")
		}
		if forbiddenOverrideKey(key) {
			return fmt.Errorf("trailconfig: override %q cannot change colony policy or identity", key)
		}
		if _, ok := bees[role]; !ok {
			return fmt.Errorf("trailconfig: override role %q is not configured", role)
		}
		path := filepath.Join(treeRoot, ".paseka", "bees", role+".yaml")
		if err := setBeeParam(path, role, key, override.Value); err != nil {
			return err
		}
	}
	if _, err := colony.LoadAllBees(treeRoot); err != nil {
		return fmt.Errorf("trailconfig: candidate overrides are invalid: %w", err)
	}
	return nil
}

func forbiddenOverrideKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "adapter", "slug", "nats", "energy", "energy_budget", "energy_default", "delivery", "credentials", "credential":
		return true
	default:
		return false
	}
}

func setBeeParam(path, role, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("trailconfig: read bee %q: %w", role, err)
	}
	var bee colony.Bee
	if err := yaml.Unmarshal(data, &bee); err != nil {
		return fmt.Errorf("trailconfig: parse bee %q: %w", role, err)
	}
	if bee.Params == nil {
		bee.Params = map[string]any{}
	}
	bee.Params[key] = value
	if bee.Role == "" {
		bee.Role = role
	}
	out, err := yaml.Marshal(bee)
	if err != nil {
		return fmt.Errorf("trailconfig: encode bee %q: %w", role, err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("trailconfig: write bee %q: %w", role, err)
	}
	return nil
}

func diffInventories(base, candidate []Entry) []string {
	baseByPath := make(map[string]Entry, len(base))
	candidateByPath := make(map[string]Entry, len(candidate))
	for _, entry := range base {
		baseByPath[entry.Path] = entry
	}
	for _, entry := range candidate {
		candidateByPath[entry.Path] = entry
	}
	var diff []string
	for path, before := range baseByPath {
		after, ok := candidateByPath[path]
		if !ok {
			diff = append(diff, "removed "+path)
			continue
		}
		if before.SHA256 != after.SHA256 || before.Mode != after.Mode {
			diff = append(diff, "changed "+path)
		}
	}
	for path := range candidateByPath {
		if _, ok := baseByPath[path]; !ok {
			diff = append(diff, "added "+path)
		}
	}
	sort.Strings(diff)
	return diff
}
