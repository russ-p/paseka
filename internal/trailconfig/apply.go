package trailconfig

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// RemainingWorkPreview reports how a candidate matches the unstarted accepted
// work that must dispatch after activation.
type RemainingWorkPreview struct {
	Evaluated  bool     `json:"evaluated"`
	Compatible bool     `json:"compatible"`
	Checked    []string `json:"checked,omitempty"`
	Problems   []string `json:"problems,omitempty"`
}

// ApplyResult is the preview and recorded outcome of one apply request.
type ApplyResult struct {
	Manifest Manifest
	Change   Change
	NewRef   string
	// Diff is the raw file-level inventory difference between the active and
	// candidate snapshots.
	Diff []string
	// EffectiveDiff is the resolved behaviour that actually changes, including
	// bee parameters and the frozen profile/model-alias view.
	EffectiveDiff []string
	// ShadowedDiff names requested overrides that do not change effective
	// behaviour, for example a setting superseded by a later override.
	ShadowedDiff []string
	// RemainingWork reports candidate compatibility with unstarted tasks.
	RemainingWork RemainingWorkPreview
	DryRun        bool
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
	killed, err := TraceKilled(opts.Context.ColonyRoot, opts.TraceID)
	if err != nil {
		return ApplyResult{}, err
	}
	if killed {
		return ApplyResult{}, fmt.Errorf("trailconfig: trail %s is killed; configuration application is refused", opts.TraceID)
	}

	dir := ConfigDir(opts.Context.ColonyRoot, opts.TraceID)
	// A dry run is a read-only preview: it must not create the configuration
	// directory or its lock file just to prove the candidate is valid. Writes
	// still serialize on the lock, and the manifest is replaced atomically, so
	// a preview can read it without taking part in that protocol.
	if !opts.DryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return ApplyResult{}, fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
		}
		release, err := acquireLock(filepath.Join(dir, lockFileName))
		if err != nil {
			return ApplyResult{}, err
		}
		defer release()
	}

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

	if opts.Source == SourceSnapshot && !retainsSnapshot(manifest, opts.SnapshotRef) {
		return ApplyResult{}, fmt.Errorf("trailconfig: snapshot %s is not retained by this trail", opts.SnapshotRef)
	}

	if opts.ExpectedRevision != nil && *opts.ExpectedRevision != manifest.ConfigRevision {
		return recordRejection(opts, manifest, fmt.Sprintf("expected revision %d, trail is at %d", *opts.ExpectedRevision, manifest.ConfigRevision))
	}
	if manifest.Pending != nil {
		return recordRejection(opts, manifest, "another configuration change is pending")
	}

	// Execution that still needs the old configuration holds activation at a
	// safe boundary. Explicit blockers from the caller are merged with the
	// durable evidence so a CLI request cannot silently cut over live work.
	blockers := append([]string(nil), opts.Blockers...)
	observed, err := Blockers(opts.Context.ColonyRoot, opts.TraceID)
	if err != nil {
		return ApplyResult{}, err
	}
	blockers = uniqueBlockers(append(blockers, observed...))

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
	baseSnapshotDir := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.TraceID), manifest.ActiveConfigRef)
	baseInventory, err := loadVerifiedSnapshot(baseSnapshotDir, manifest.ActiveConfigRef)
	if err != nil {
		return ApplyResult{}, err
	}
	baseTree := filepath.Join(baseSnapshotDir, SnapshotTreeDirName)
	baseColony, err := colony.LoadColony(baseTree)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("trailconfig: active configuration is unreadable: %w", err)
	}
	// Preparation validates the complete candidate procedure, not only the
	// roles needed by remaining work: a missing template, skill, adapter, or
	// policy mismatch must be refused before anything is recorded or activated.
	if err := ValidateProcedure(candidate.tree, baseColony, RemainingProcedureWork(snapshot)); err != nil {
		return ApplyResult{}, err
	}
	remaining := EvaluateRemaining(candidate.tree, snapshot)
	if !remaining.Compatible {
		return ApplyResult{}, fmt.Errorf("%s", strings.Join(remaining.Problems, "; "))
	}

	effective, shadowed := previewDiffs(opts, baseInventory, candidate.inventory, baseTree, candidate.tree)
	result := ApplyResult{
		Manifest:      manifest,
		NewRef:        candidate.ref,
		Diff:          diffInventories(baseInventory.Entries, candidate.inventory.Entries),
		EffectiveDiff: effective,
		ShadowedDiff:  shadowed,
		RemainingWork: remaining,
		DryRun:        opts.DryRun,
	}

	if opts.DryRun {
		change := newChange(opts, manifest, candidate.ref)
		switch {
		case candidate.ref == manifest.ActiveConfigRef:
			change.Outcome = OutcomeUnchanged
			change.Detail = "dry run: candidate is already active"
		case len(blockers) > 0:
			change.Outcome = OutcomePending
			change.Blockers = append([]string(nil), blockers...)
			change.Detail = "dry run: activation waits for blockers"
		default:
			// Report what the recorded action would become at the current
			// boundary. The result is not written, so AppliedAt stays empty and
			// Manifest keeps the active binding.
			change.Outcome = OutcomeApplied
			change.Detail = "dry run: activation would proceed immediately"
		}
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
	if len(blockers) > 0 {
		change.Outcome = OutcomePending
		change.Blockers = append([]string(nil), blockers...)
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
		if err := validateConfigRef(opts.SnapshotRef); err != nil {
			return err
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
	preview := EvaluateRemaining(candidateRoot, snapshot)
	if preview.Compatible {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(preview.Problems, "; "))
}

// EvaluateRemaining reports candidate compatibility with every unstarted
// accepted task and names the tasks that were checked. Incompatibility is a
// preview result rather than an early error so an operator can see the exact
// remaining work that blocks the candidate.
func EvaluateRemaining(candidateRoot string, snapshot taskledger.TraceSnapshot) RemainingWorkPreview {
	preview := RemainingWorkPreview{Evaluated: true, Compatible: true}
	problems := make([]string, 0)
	fail := func(problem string) {
		preview.Compatible = false
		problems = append(problems, problem)
	}

	manifest, err := colony.LoadColony(candidateRoot)
	if err != nil {
		fail(fmt.Sprintf("trailconfig: load candidate colony: %v", err))
		preview.Problems = problems
		return preview
	}
	bees, err := colony.LoadAllBees(candidateRoot)
	if err != nil {
		fail(fmt.Sprintf("trailconfig: load candidate bees: %v", err))
		preview.Problems = problems
		return preview
	}
	for _, work := range RemainingProcedureWork(snapshot) {
		preview.Checked = append(preview.Checked, work.TaskID)
		if err := validateProcedureWork(manifest, bees, work); err != nil {
			fail(err.Error())
		}
	}
	preview.Problems = problems
	return preview
}

// retainsSnapshot reports whether this trail still retains the named version.
func retainsSnapshot(manifest Manifest, ref string) bool {
	ref = strings.TrimSpace(ref)
	for _, snapshot := range manifest.Snapshots {
		if snapshot.ConfigRef == ref {
			return true
		}
	}
	return false
}

// previewDiffs reports the resolved candidate changes and the requested
// overrides that cannot affect them. Both are derived from the frozen trees,
// never from live colony files.
func previewDiffs(opts ApplyOptions, base, candidate Inventory, baseTree, candidateTree string) ([]string, []string) {
	effective := append([]string(nil), diffInventories(base.Entries, candidate.Entries)...)
	effective = append(effective, resolvedDiffs(base.Resolved, candidate.Resolved)...)

	baseBees, baseErr := colony.LoadAllBees(baseTree)
	candidateBees, candidateErr := colony.LoadAllBees(candidateTree)
	switch {
	case baseErr != nil:
		effective = append(effective, "bees: active configuration is unreadable: "+baseErr.Error())
	case candidateErr != nil:
		effective = append(effective, "bees: candidate is unreadable: "+candidateErr.Error())
	default:
		effective = append(effective, beeDiffs(baseBees, candidateBees)...)
	}

	return normalizePreviewLines(effective), shadowedOverrides(opts, baseBees, candidateBees)
}

func resolvedDiffs(base, candidate Resolved) []string {
	var out []string
	if base.Profile != candidate.Profile {
		out = append(out, fmt.Sprintf("profile: %s -> %s", previewValue(base.Profile), previewValue(candidate.Profile)))
	}
	seen := make(map[string]struct{}, len(base.ModelAliases)+len(candidate.ModelAliases))
	keys := make([]string, 0, len(base.ModelAliases)+len(candidate.ModelAliases))
	for key := range base.ModelAliases {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range candidate.ModelAliases {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		before := previewValue(base.ModelAliases[key])
		after := previewValue(candidate.ModelAliases[key])
		if before != after {
			out = append(out, fmt.Sprintf("model alias %s: %s -> %s", key, before, after))
		}
	}
	return out
}

func beeDiffs(base, candidate map[string]colony.Bee) []string {
	seen := make(map[string]struct{}, len(base)+len(candidate))
	roles := make([]string, 0, len(base)+len(candidate))
	for role := range base {
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	for role := range candidate {
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	sort.Strings(roles)

	var out []string
	for _, role := range roles {
		before, hasBefore := base[role]
		after, hasAfter := candidate[role]
		switch {
		case !hasAfter:
			out = append(out, "bee "+role+": removed")
		case !hasBefore:
			out = append(out, "bee "+role+": added")
		default:
			out = append(out, beeSettingDiffs(role, before, after)...)
		}
	}
	return out
}

func beeSettingDiffs(role string, before, after colony.Bee) []string {
	var out []string
	add := func(name, oldValue, newValue string) {
		if oldValue == newValue {
			return
		}
		out = append(out, fmt.Sprintf("bee %s %s: %s -> %s", role, name, oldValue, newValue))
	}

	add("adapter", before.Adapter, after.Adapter)
	add("prompt_template", before.PromptTemplate, after.PromptTemplate)
	add("system_template", before.SystemTemplate, after.SystemTemplate)
	add("sector", before.Sector, after.Sector)
	add("worktree", strconv.FormatBool(before.Worktree), strconv.FormatBool(after.Worktree))
	add("default_intent", before.DefaultIntent, after.DefaultIntent)
	add("intents", strings.Join(before.Intents, ","), strings.Join(after.Intents, ","))
	add("command", strings.Join(before.Command.Argv(), " "), strings.Join(after.Command.Argv(), " "))
	add("post_exec", strings.Join(before.PostExec.Argv(), " "), strings.Join(after.PostExec.Argv(), " "))
	add("subscribes", settingJSON(before.Subscribes), settingJSON(after.Subscribes))
	add("publishes", settingJSON(before.Publishes), settingJSON(after.Publishes))
	add("completion_contract", settingJSON(before.CompletionContract), settingJSON(after.CompletionContract))
	add("run_summary", string(before.RunSummary), string(after.RunSummary))

	seen := make(map[string]struct{}, len(before.Params)+len(after.Params))
	keys := make([]string, 0, len(before.Params)+len(after.Params))
	for key := range before.Params {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range after.Params {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		oldValue := previewValue(before.Params[key])
		newValue := previewValue(after.Params[key])
		if oldValue != newValue {
			out = append(out, fmt.Sprintf("bee %s param %s: %s -> %s", role, key, oldValue, newValue))
		}
	}
	return out
}

func shadowedOverrides(opts ApplyOptions, baseBees, candidateBees map[string]colony.Bee) []string {
	if opts.Source != SourceOverrides || len(opts.Overrides) == 0 {
		return nil
	}
	last := make(map[string]int, len(opts.Overrides))
	for i, override := range opts.Overrides {
		last[overrideIdentity(override)] = i
	}
	var out []string
	for i, override := range opts.Overrides {
		role := strings.TrimSpace(override.Role)
		key := strings.TrimSpace(override.Key)
		label := fmt.Sprintf("bee %s param %s", role, key)
		if last[overrideIdentity(override)] != i {
			out = append(out, label+": shadowed by a later override for the same setting")
			continue
		}
		before := previewValue(baseBees[role].Params[key])
		after := previewValue(candidateBees[role].Params[key])
		value := strings.TrimSpace(override.Value)
		switch {
		case before == after && after == value:
			out = append(out, label+": already "+value+" (no effective change)")
		case after != value:
			out = append(out, fmt.Sprintf("%s: requested %q but the effective value is %q", label, value, after))
		}
	}
	return out
}

func overrideIdentity(override Override) string {
	return strings.TrimSpace(override.Role) + "\x00" + strings.TrimSpace(override.Key)
}

func previewValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	default:
		return settingJSON(value)
	}
}

func settingJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	if string(data) == "null" {
		return ""
	}
	return string(data)
}

func normalizePreviewLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	sort.Strings(lines)
	out := lines[:0]
	var previous string
	for i, line := range lines {
		if i > 0 && line == previous {
			continue
		}
		out = append(out, line)
		previous = line
	}
	return out
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
	observed, err := Blockers(colonyRoot, traceID)
	if err != nil {
		return Manifest{}, false, err
	}
	blockers = uniqueBlockers(append(blockers, observed...))
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
	activeSnapshotDir := filepath.Join(SnapshotDir(colonyRoot, traceID), manifest.ActiveConfigRef)
	if _, err := loadVerifiedSnapshot(activeSnapshotDir, manifest.ActiveConfigRef); err != nil {
		return Manifest{}, false, err
	}
	activeColony, err := colony.LoadColony(filepath.Join(activeSnapshotDir, SnapshotTreeDirName))
	if err != nil {
		return Manifest{}, false, fmt.Errorf("trailconfig: active configuration is unreadable: %w", err)
	}
	tree := filepath.Join(SnapshotDir(colonyRoot, traceID), change.NewConfigRef, SnapshotTreeDirName)
	// The prepared content is revalidated in full: a candidate that was valid
	// when it was captured can lose a template, skill, role, or policy fit while
	// it waits for a safe boundary.
	if err := ValidateProcedure(tree, activeColony, RemainingProcedureWork(snapshot)); err != nil {
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

// CancelIfPending cancels a pending action when one exists. It is the
// idempotent form used by kill and recovery paths, where no pending action is a
// normal outcome rather than an operator error.
func CancelIfPending(colonyRoot, traceID, actor, reason string) (Manifest, bool, error) {
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
		if errors.Is(err, ErrNoBinding) {
			return Manifest{}, false, nil
		}
		return Manifest{}, false, err
	}
	if manifest.Pending == nil {
		return manifest, false, nil
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
		return Manifest{}, false, err
	}
	return manifest, true, nil
}

// HasPending reports whether a trail is holding new admissions for a pending
// configuration action.
func HasPending(colonyRoot, traceID string) (bool, error) {
	manifest, err := Load(colonyRoot, traceID)
	if err != nil {
		if errors.Is(err, ErrNoBinding) {
			return false, nil
		}
		return false, err
	}
	return manifest.Pending != nil, nil
}

// PendingTraceIDs lists trails whose configuration transition is still waiting
// for a safe boundary. It is used to recover the admission barrier after a
// Reactor restart.
func PendingTraceIDs(colonyRoot string) ([]string, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var traceIDs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := Load(colonyRoot, entry.Name())
		if err != nil {
			continue
		}
		if manifest.Pending != nil {
			traceIDs = append(traceIDs, manifest.TraceID)
		}
	}
	sort.Strings(traceIDs)
	return traceIDs, nil
}

// runEvidence is the durable status of one agent run directory. It is the
// post-restart substitute for in-memory Reactor state: status.json is the only
// record that says whether an invocation is still executing.
type runEvidence struct {
	agentID   string
	attemptID string
	state     string
	readable  bool
}

// scanRunEvidence reads every agent run record under the trail. Reserved
// subdirectories (tasks, artifacts, config, proposals, console, fork requests)
// are not invocations. A run whose status.json cannot be read stays in the
// result as unreadable evidence so Blockers can fail closed instead of
// unlocking cutover on missing data.
func scanRunEvidence(colonyRoot, traceID string) ([]runEvidence, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("trailconfig: read run evidence: %w", err)
	}
	evidence := make([]runEvidence, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || runs.IsReservedTraceSubdir(entry.Name()) {
			continue
		}
		record := runEvidence{agentID: entry.Name()}
		dir := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: entry.Name()}
		status, err := dir.ReadStatus()
		if err != nil {
			evidence = append(evidence, record)
			continue
		}
		record.attemptID = status.AttemptID
		record.state = string(status.State)
		record.readable = true
		evidence = append(evidence, record)
	}
	return evidence, nil
}

// runTerminal reports whether a recorded run state proves the invocation
// ended. Anything else — queued, running, suspended, awaiting human input, or a
// state this build does not know — is treated as live work.
func runTerminal(state string) bool {
	switch protocol.RunStatus(state) {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		return true
	default:
		return false
	}
}

// Blockers reports execution that still needs the old configuration.
//
// Safe cutover means no executing or suspended invocation, live task attempt,
// outstanding verification, or human review still holds the old context. An
// open attempt is not by itself execution: direct chain and interactive session
// attempts stay open after their last run finishes so later events keep routing
// under the same configuration. Such an attempt therefore blocks only while it
// was admitted but never dispatched, or while it owns a run whose recorded
// status is not terminal. The rule reads durable attempt and run records, so it
// holds across a Reactor restart and never changes attempt state or routing.
func Blockers(colonyRoot, traceID string) ([]string, error) {
	var blockers []string

	snapshot, err := runs.LoadTraceTasksFromFS(colonyRoot, traceID)
	if err != nil {
		return nil, fmt.Errorf("trailconfig: read task evidence: %w", err)
	}
	for _, task := range snapshot.Tasks {
		switch task.Status {
		case protocol.TaskStatusRunning, protocol.TaskStatusWaitingReview:
			blockers = append(blockers, "task "+task.TaskID+" is "+string(task.Status))
		}
	}

	attempts, err := Attempts(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	evidence, err := scanRunEvidence(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	for _, run := range evidence {
		switch {
		case !run.readable:
			blockers = append(blockers, "run "+run.agentID+" has no readable status")
		case !runTerminal(run.state):
			blockers = append(blockers, "run "+run.agentID+" is "+run.state)
		}
	}

	for _, attempt := range attempts {
		if !attempt.Open() {
			continue
		}
		label := blockerLabel(attempt)
		// Admitted but not dispatched yet: the admission -> launch gap, a
		// feedback-driven ready state, and an energy-blocked ready task still
		// belong to the old configuration.
		if attempt.DispatchCount == 0 {
			blockers = append(blockers, label+" is open")
			continue
		}
		if detail, live := attemptLiveRun(attempt, evidence); live {
			blockers = append(blockers, label+" "+detail)
		}
	}
	return uniqueBlockers(blockers), nil
}

func blockerLabel(attempt Attempt) string {
	label := "attempt " + attempt.AttemptID
	if strings.TrimSpace(attempt.TaskID) != "" {
		label += " for task " + attempt.TaskID
	} else if strings.TrimSpace(attempt.ChainKey) != "" {
		label += " for chain " + attempt.ChainKey
	}
	return label
}

// attemptLiveRun reports whether an open attempt still owns a non-terminal
// invocation and describes the first such run in stable agent-id order. Runs
// are found both through the attempt's recorded agents and through run
// provenance, so a CLI run that was never linked back to the attempt record
// still counts.
func attemptLiveRun(attempt Attempt, evidence []runEvidence) (string, bool) {
	byAgent := make(map[string]runEvidence, len(evidence))
	for _, run := range evidence {
		byAgent[run.agentID] = run
	}
	owned := make(map[string]runEvidence, len(attempt.Agents))
	for _, agentID := range attempt.Agents {
		if strings.TrimSpace(agentID) == "" {
			continue
		}
		if run, ok := byAgent[agentID]; ok {
			owned[agentID] = run
			continue
		}
		owned[agentID] = runEvidence{agentID: agentID}
	}
	for _, run := range evidence {
		if run.attemptID == attempt.AttemptID {
			owned[run.agentID] = run
		}
	}
	agents := make([]string, 0, len(owned))
	for agentID := range owned {
		agents = append(agents, agentID)
	}
	sort.Strings(agents)
	for _, agentID := range agents {
		run := owned[agentID]
		if !run.readable {
			return "owns run " + agentID + " with no readable status", true
		}
		if !runTerminal(run.state) {
			return "owns run " + agentID + " which is " + run.state, true
		}
	}
	return "", false
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

// forbiddenOverrideKey blocks keys that would change colony identity,
// transport, credential sources, budget ceilings, delivery permissions, or
// configuration policy. These belong to canonical colony policy and cannot be
// redirected by a trail-local behavioural override.
func forbiddenOverrideKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "adapter", "binary",
		"slug", "identity", "trace", "trace_id",
		"nats", "nats_url", "subject_prefix", "transport",
		"home", "home_root", "colony_root",
		"energy", "energy_budget", "energy_default", "budget", "budget_ceiling",
		"delivery", "delivery_policy", "merge", "pull_request",
		"credentials", "credential", "api_key", "api_key_env", "token", "secret", "env", "env_mode",
		"policy", "config_policy":
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
