// Package trailconfig captures and reads a trail's immutable configuration
// binding.
//
// A new trail owns a complete copy of the configuration that was in effect when
// it started: committed bee and cue definitions, prompt templates and partials,
// the selected profile recipe and applicable local contributions, effective
// model aliases, and available skills. Machine-local secrets and runtime data are
// deliberately excluded. The content identity (ConfigRef) is derived from the
// captured bytes and executable modes only, never from the trail identity or its
// physical location.
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
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/filelock"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/runs"
	"gopkg.in/yaml.v3"
)

const (
	// ConfigDirName is the runtime-owned directory under a trail's run root.
	ConfigDirName = "config"
	// ManifestFileName is the binding manifest inside the config directory.
	ManifestFileName = "manifest.json"
	// SnapshotsDirName holds one immutable snapshot directory per ConfigRef.
	SnapshotsDirName = "snapshots"
	// SnapshotTreeDirName holds the captured files inside one snapshot.
	SnapshotTreeDirName = "tree"
	// InventoryFileName names the canonical digest inventory of a snapshot.
	InventoryFileName = "inventory.json"
	// ResolvedFileName names the effective non-secret settings of a snapshot.
	ResolvedFileName = "resolved.json"

	// PolicyEditable allows an ordinary trail to apply future configuration versions.
	PolicyEditable = "editable"
	// PolicyPinned rejects configuration application, as used by experiments.
	PolicyPinned = "pinned"

	manifestVersion   = 1
	inventoryVersion  = 1
	configRefPrefix   = "sha256:"
	lockFileName      = ".lock"
	lockStaleAfter    = 30 * time.Second
	lockAcquireBudget = 10 * time.Second
)

// ErrNoBinding reports that a trail has no captured configuration binding.
var ErrNoBinding = errors.New("trailconfig: no configuration binding")

// ErrNotColony reports that the directory does not carry an initialized colony.
var ErrNotColony = errors.New("trailconfig: colony is not initialized")

// ProfileLayers records which layers contributed to the selected profile.
type ProfileLayers struct {
	Colony bool `json:"colony,omitempty"`
	Home   bool `json:"home,omitempty"`
}

// Resolved is the effective non-secret configuration view frozen with a snapshot.
type Resolved struct {
	Version       int               `json:"version"`
	Profile       string            `json:"profile,omitempty"`
	ProfileLayers ProfileLayers     `json:"profileLayers,omitempty"`
	ModelAliases  map[string]string `json:"modelAliases,omitempty"`
}

// Entry is one captured file.
type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
	// Private marks a machine-local input that must not appear in shared Git
	// history or ordinary exports. The content is retained with the trail under
	// restrictive permissions and its identity still contributes to ConfigRef.
	Private bool `json:"private,omitempty"`
}

// FileContent is one retained configuration file together with its bytes.
type FileContent struct {
	Entry
	Data []byte `json:"-"`
}

// Inventory is the canonical content of one snapshot.
type Inventory struct {
	Version  int      `json:"version"`
	Entries  []Entry  `json:"entries"`
	Resolved Resolved `json:"resolved"`
}

// Snapshot records one immutable version retained by the trail.
type Snapshot struct {
	ConfigRef string    `json:"configRef"`
	Path      string    `json:"path"`
	Source    string    `json:"source,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	Files     int       `json:"files"`
}

// Revision is one entry in the append-only binding history. It doubles as the
// recorded operator configuration change, so a trail can show who applied which
// source and why without a second audit file.
type Revision struct {
	Revision         int    `json:"revision"`
	InitialConfigRef string `json:"initialConfigRef"`
	ActiveConfigRef  string `json:"activeConfigRef"`
	Source           string `json:"source,omitempty"`
	// ChangeID is the identity of one apply request. The initial capture also
	// receives one so every history entry is addressable.
	ChangeID string `json:"changeId,omitempty"`
	// RequestID is the idempotency key supplied by the operator/CLI.
	RequestID string `json:"requestId,omitempty"`
	// RequestFingerprint identifies the requested contents so a reused request
	// identity with different parameters can be refused.
	RequestFingerprint string `json:"requestFingerprint,omitempty"`
	Actor              string `json:"actor,omitempty"`
	Reason             string `json:"reason,omitempty"`
	ExpectedRevision   int    `json:"expectedRevision,omitempty"`
	OldConfigRef       string `json:"oldConfigRef,omitempty"`
	NewConfigRef       string `json:"newConfigRef,omitempty"`
	// Outcome is one of pending, applied, unchanged, rejected, or cancelled.
	Outcome     string     `json:"outcome,omitempty"`
	RequestedAt time.Time  `json:"requestedAt,omitempty"`
	AppliedAt   *time.Time `json:"appliedAt,omitempty"`
	Blockers    []string   `json:"blockers,omitempty"`
	Detail      string     `json:"detail,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Change is the recorded apply request stored in a trail's history.
type Change = Revision

// Manifest is the trail-owned configuration binding.
type Manifest struct {
	Version          int    `json:"version"`
	TraceID          string `json:"traceId"`
	InitialConfigRef string `json:"initialConfigRef"`
	ActiveConfigRef  string `json:"activeConfigRef"`
	ConfigRevision   int    `json:"configRevision"`
	Policy           string `json:"policy"`
	// InitialCodeBase records the repository state the trail started from. It is
	// captured once, beside the initial configuration, and never follows later
	// commits or worktree edits.
	InitialCodeBaseSHA        string    `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty      bool      `json:"initialCodeBaseDirty,omitempty"`
	InitialCodeBaseCapturedAt time.Time `json:"initialCodeBaseCapturedAt,omitempty"`
	// WorkspaceRoot is the immutable execution root for this trail. An
	// experimental fork records its own checkout so every later dispatch runs
	// there instead of the primary colony; ordinary trails leave it empty and
	// resolve to the colony root.
	WorkspaceRoot string     `json:"workspaceRoot,omitempty"`
	CreatedAt     time.Time  `json:"createdAt,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt,omitempty"`
	Snapshots     []Snapshot `json:"snapshots"`
	// Pending holds the only in-flight configuration action, when a caller
	// prepared a candidate that cannot activate at the current safe boundary.
	Pending *Change    `json:"pending,omitempty"`
	History []Revision `json:"history"`
}

// Binding is the minimal read surface for a trail's configuration.
type Binding struct {
	TraceID              string `json:"traceId"`
	InitialConfigRef     string `json:"initialConfigRef"`
	ActiveConfigRef      string `json:"activeConfigRef"`
	ConfigRevision       int    `json:"configRevision"`
	Policy               string `json:"policy"`
	ConfigDir            string `json:"configDir,omitempty"`
	InitialCodeBaseSHA   string `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty bool   `json:"initialCodeBaseDirty,omitempty"`
	WorkspaceRoot        string `json:"workspaceRoot,omitempty"`
}

// PrivatePath reports whether a captured logical path is a machine-local
// private input. Home-scoped contributions (adapter recipes and skills from
// ~/.config/paseka/<slug>) and gitignored *.local.yaml overlays never live in
// the colony's shared Git history, so they are retained privately instead of
// being exported like committed files.
func PrivatePath(logical string) bool {
	p := filepath.ToSlash(strings.TrimSpace(logical))
	if p == "home" || strings.HasPrefix(p, "home/") {
		return true
	}
	return strings.HasSuffix(p, ".local.yaml")
}

// ConfigDir returns .paseka/runs/<traceId>/config under colonyRoot.
func ConfigDir(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, ConfigDirName)
}

// ManifestPath returns the binding manifest path for one trail.
func ManifestPath(colonyRoot, traceID string) string {
	return filepath.Join(ConfigDir(colonyRoot, traceID), ManifestFileName)
}

// SnapshotDir returns the retained-snapshot root for one trail.
func SnapshotDir(colonyRoot, traceID string) string {
	return filepath.Join(ConfigDir(colonyRoot, traceID), SnapshotsDirName)
}

// EnsureForRoot captures the initial binding for traceID from colonyRoot.
//
// It is intended for entry points that only have a colony root in hand. The
// selected process profile is honoured when the colony can be resolved; a
// minimal colony that cannot be resolved (for example a fixture without a home
// config) is captured from its committed manifest alone. Directories without an
// initialized colony are left untouched.
func EnsureForRoot(colonyRoot, traceID string) (Manifest, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return Manifest{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if _, err := os.Stat(colony.PasekaPath(colonyRoot, "colony.yaml")); err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrNotColony
		}
		return Manifest{}, fmt.Errorf("trailconfig: inspect colony manifest: %w", err)
	}
	ctx, err := colony.ResolveContext(colonyRoot)
	if err != nil {
		manifest, loadErr := colony.LoadColony(colonyRoot)
		if loadErr != nil {
			return Manifest{}, loadErr
		}
		ctx = colony.Context{
			ColonyRoot:   colonyRoot,
			Slug:         strings.TrimSpace(manifest.Slug),
			ModelAliases: manifest.ModelAliases,
		}
		if ctx.Slug == "" {
			ctx.Slug = filepath.Base(filepath.Clean(colonyRoot))
		}
	}
	return Ensure(ctx, traceID)
}

// Ensure returns the trail's binding, capturing revision 1 on first use.
//
// Capture is serialized per trail and the manifest is published by atomic
// rename, so concurrent first events converge on one durable binding rather than
// appending duplicate revisions or exposing a partially copied snapshot.
func Ensure(ctx colony.Context, traceID string) (Manifest, error) {
	manifest, _, err := EnsureInitial(ctx, traceID)
	return manifest, err
}

// EnsureInitial captures revision 1 for a trail that has no binding. It is
// idempotent: an existing binding is returned with created=false.
func EnsureInitial(ctx colony.Context, traceID string) (Manifest, bool, error) {
	return EnsureInitialExpected(ctx, traceID, nil)
}

// EnsureInitialExpected captures revision 1 only when the caller's expected
// revision matches. Explicit legacy capture uses expected revision 0; nil keeps
// the idempotent behaviour used by automatic first-event binding.
func EnsureInitialExpected(ctx colony.Context, traceID string, expected *int) (Manifest, bool, error) {
	if ctx.ColonyRoot == "" {
		return Manifest{}, false, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return Manifest{}, false, err
	}
	if expected != nil && *expected != 0 {
		return Manifest{}, false, fmt.Errorf("trailconfig: expected revision %d, but initial capture establishes revision 1 from 0", *expected)
	}
	killed, err := TraceKilled(ctx.ColonyRoot, traceID)
	if err != nil {
		return Manifest{}, false, err
	}
	if killed {
		return Manifest{}, false, fmt.Errorf("trailconfig: trail %s is killed; configuration capture is refused", traceID)
	}
	dir := ConfigDir(ctx.ColonyRoot, traceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, false, fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
	}

	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Manifest{}, false, err
	}
	defer release()

	existing, err := Load(ctx.ColonyRoot, traceID)
	if err == nil {
		if expected != nil {
			return Manifest{}, false, fmt.Errorf("trailconfig: trail %s is already at revision %d, expected %d", traceID, existing.ConfigRevision, *expected)
		}
		return existing, false, nil
	}
	if !errors.Is(err, ErrNoBinding) {
		return Manifest{}, false, err
	}
	if expected != nil {
		blockers, err := Blockers(ctx.ColonyRoot, traceID)
		if err != nil {
			return Manifest{}, false, err
		}
		if len(blockers) > 0 {
			return Manifest{}, false, fmt.Errorf("trailconfig: trail %s cannot capture configuration at an unsafe boundary: %s", traceID, strings.Join(blockers, "; "))
		}
	}

	manifest, err := captureInitialLocked(ctx, traceID, dir)
	if err != nil {
		return Manifest{}, false, err
	}
	return manifest, true, nil
}

func captureInitialLocked(ctx colony.Context, traceID, dir string) (Manifest, error) {
	contents, resolved, err := capture(ctx)
	if err != nil {
		return Manifest{}, err
	}
	inventory := buildInventory(contents, resolved)
	ref, err := configRef(inventory)
	if err != nil {
		return Manifest{}, err
	}
	if err := materializeSnapshot(dir, ref, contents, inventory); err != nil {
		return Manifest{}, err
	}

	now := time.Now().UTC()
	codeBaseSHA, codeBaseDirty := captureCodeBase(ctx.ColonyRoot)
	manifest := Manifest{
		Version:                   manifestVersion,
		TraceID:                   traceID,
		InitialConfigRef:          ref,
		ActiveConfigRef:           ref,
		ConfigRevision:            1,
		Policy:                    PolicyEditable,
		InitialCodeBaseSHA:        codeBaseSHA,
		InitialCodeBaseDirty:      codeBaseDirty,
		InitialCodeBaseCapturedAt: now,
		CreatedAt:                 now,
		UpdatedAt:                 now,
		Snapshots: []Snapshot{{
			ConfigRef: ref,
			Path:      filepath.ToSlash(filepath.Join(SnapshotsDirName, ref)),
			Source:    "colony",
			CreatedAt: now,
			Files:     len(contents),
		}},
		History: []Revision{{
			Revision:         1,
			InitialConfigRef: ref,
			ActiveConfigRef:  ref,
			Source:           SourceColony,
			ChangeID:         newChangeID(),
			Actor:            "runtime",
			Outcome:          OutcomeApplied,
			RequestedAt:      now,
			CreatedAt:        now,
			NewConfigRef:     ref,
		}},
	}
	if err := writeManifest(dir, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// IsNewTrail reports whether a trail has started execution. The publisher seam
// creates the trail root and records the first event before the bus attempt, so
// the root can exist when the Reactor sees that event. A trail is still new
// while the root holds only its event log: any run, task, artifact, proposal, or
// console record means execution already began.
func IsNewTrail(colonyRoot, traceID string) bool {
	if strings.TrimSpace(traceID) == "" {
		return false
	}
	root := colony.PasekaPath(colonyRoot, "runs", traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return os.IsNotExist(err)
	}
	for _, ent := range entries {
		if ent.IsDir() && ent.Name() == ConfigDirName {
			continue
		}
		if ent.IsDir() || ent.Name() != runs.TrailEventsFileName {
			return false
		}
	}
	return true
}

// LoadManifestIfExists reads a trail's manifest without treating an absent
// binding as an error.
func LoadManifestIfExists(colonyRoot, traceID string) (Manifest, bool, error) {
	manifest, err := Load(colonyRoot, traceID)
	if err == nil {
		return manifest, true, nil
	}
	if errors.Is(err, ErrNoBinding) {
		return Manifest{}, false, nil
	}
	return Manifest{}, false, err
}

// Load reads a trail's configuration manifest.
func Load(colonyRoot, traceID string) (Manifest, error) {
	if colonyRoot == "" {
		return Manifest{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return Manifest{}, err
	}
	data, err := os.ReadFile(ManifestPath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{}, ErrNoBinding
		}
		return Manifest{}, fmt.Errorf("trailconfig: read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("trailconfig: parse manifest: %w", err)
	}
	if manifest.TraceID != "" && manifest.TraceID != traceID {
		return Manifest{}, fmt.Errorf("trailconfig: manifest belongs to %q, not %q", manifest.TraceID, traceID)
	}
	if err := validateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Read returns the minimal binding for a trail.
func Read(colonyRoot, traceID string) (Binding, error) {
	manifest, err := Load(colonyRoot, traceID)
	if err != nil {
		return Binding{}, err
	}
	return Binding{
		TraceID:              manifest.TraceID,
		InitialConfigRef:     manifest.InitialConfigRef,
		ActiveConfigRef:      manifest.ActiveConfigRef,
		ConfigRevision:       manifest.ConfigRevision,
		Policy:               manifest.Policy,
		ConfigDir:            ConfigDir(colonyRoot, traceID),
		InitialCodeBaseSHA:   manifest.InitialCodeBaseSHA,
		InitialCodeBaseDirty: manifest.InitialCodeBaseDirty,
		WorkspaceRoot:        manifest.WorkspaceRoot,
	}, nil
}

// Bind resolves a trail's active immutable configuration. It never captures:
// unbound trails stay legacy until an operator explicitly captures them.
func Bind(ctx colony.Context, traceID string) (colony.Context, Manifest, error) {
	manifest, ok, err := LoadManifestIfExists(ctx.ColonyRoot, traceID)
	if err != nil {
		return ctx, Manifest{}, err
	}
	if !ok {
		return ctx, Manifest{}, fmt.Errorf("trailconfig: trail %s has no configuration binding", traceID)
	}
	bound, err := BindRef(ctx, traceID, manifest.ActiveConfigRef, manifest.ConfigRevision)
	if err != nil {
		return ctx, Manifest{}, err
	}
	return bound, manifest, nil
}

// BindNewWork binds a trail for newly admitted work. An existing binding is
// used; a brand-new trail captures revision 1 first. A pre-existing unbound
// trail keeps legacy behaviour and is returned untouched.
func BindNewWork(ctx colony.Context, traceID string) (colony.Context, Manifest, bool, error) {
	if manifest, ok, err := LoadManifestIfExists(ctx.ColonyRoot, traceID); err != nil {
		return ctx, Manifest{}, false, err
	} else if ok {
		bound, _, err := Bind(ctx, traceID)
		return bound, manifest, true, err
	}
	if !IsNewTrail(ctx.ColonyRoot, traceID) {
		return ctx, Manifest{}, false, nil
	}
	manifest, _, err := EnsureInitial(ctx, traceID)
	if err != nil {
		return ctx, Manifest{}, false, err
	}
	bound, err := BindRef(ctx, traceID, manifest.ActiveConfigRef, manifest.ConfigRevision)
	if err != nil {
		return ctx, Manifest{}, false, err
	}
	return bound, manifest, true, nil
}

// BindRef resolves one retained snapshot as an immutable execution context.
// Missing or corrupt content blocks binding rather than falling back to live
// colony files.
func BindRef(ctx colony.Context, traceID, ref string, revision int) (colony.Context, error) {
	if ctx.ColonyRoot == "" {
		return ctx, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateConfigRef(ref); err != nil {
		return ctx, err
	}
	snapshotDir := filepath.Join(SnapshotDir(ctx.ColonyRoot, traceID), ref)
	inventory, err := loadVerifiedSnapshot(snapshotDir, ref)
	if err != nil {
		return ctx, err
	}
	workspaceRoot := strings.TrimSpace(ctx.WorkspaceRoot)
	if manifest, loadErr := Load(ctx.ColonyRoot, traceID); loadErr == nil {
		if trailRoot := strings.TrimSpace(manifest.WorkspaceRoot); trailRoot != "" {
			workspaceRoot = trailRoot
		}
	} else if !errors.Is(loadErr, ErrNoBinding) {
		return ctx, loadErr
	}
	if workspaceRoot == "" {
		workspaceRoot = ctx.ColonyRoot
	}

	bound := ctx
	bound.ConfigRoot = filepath.Join(snapshotDir, SnapshotTreeDirName)
	bound.ConfigRef = ref
	bound.ConfigRevision = revision
	bound.WorkspaceRoot = workspaceRoot
	bound.Profile = inventory.Resolved.Profile
	bound.ProfileLayers = colony.ProfileLayers{
		Colony: inventory.Resolved.ProfileLayers.Colony,
		Home:   inventory.Resolved.ProfileLayers.Home,
	}
	bound.ModelAliases = cloneAliases(inventory.Resolved.ModelAliases)
	return bound, nil
}

// captureCodeBase is best-effort: a colony fixture without git still gets a
// configuration binding, but cannot claim an initial commit.
func captureCodeBase(colonyRoot string) (string, bool) {
	sha, err := gitroot.Run(gitroot.RunOpts{
		Dir:     colonyRoot,
		Timeout: gitroot.StatusTimeout,
		Args:    []string{"rev-parse", "HEAD"},
	})
	if err != nil {
		return "", false
	}
	dirty, err := gitroot.Dirty(colonyRoot)
	if err != nil {
		dirty = false
	}
	return strings.TrimSpace(sha), dirty
}

func validateConfigRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if !strings.HasPrefix(ref, configRefPrefix) || len(ref) != len(configRefPrefix)+sha256.Size*2 {
		return fmt.Errorf("trailconfig: invalid configuration ref %q", ref)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(ref, configRefPrefix)); err != nil {
		return fmt.Errorf("trailconfig: invalid configuration ref %q", ref)
	}
	return nil
}

func cloneAliases(aliases map[string]string) map[string]string {
	if len(aliases) == 0 {
		return nil
	}
	out := make(map[string]string, len(aliases))
	for key, value := range aliases {
		out[key] = value
	}
	return out
}

// VerifySnapshot checks that a trail still retains one complete immutable
// snapshot and that its content still matches the recorded ConfigRef. Read
// models use it to report missing or corrupt evidence instead of assuming a
// manifest reference is still readable.
func VerifySnapshot(colonyRoot, traceID, ref string) error {
	if colonyRoot == "" {
		return fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return err
	}
	if err := validateConfigRef(ref); err != nil {
		return err
	}
	snapshotDir := filepath.Join(SnapshotDir(colonyRoot, traceID), ref)
	if _, err := loadVerifiedSnapshot(snapshotDir, ref); err != nil {
		return err
	}
	return nil
}

func loadVerifiedSnapshot(snapshotDir, expectedRef string) (Inventory, error) {
	data, err := os.ReadFile(filepath.Join(snapshotDir, InventoryFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return Inventory{}, fmt.Errorf("trailconfig: configuration %s is unavailable", expectedRef)
		}
		return Inventory{}, fmt.Errorf("trailconfig: retained snapshot is unreadable: %w", err)
	}
	var inventory Inventory
	if err := json.Unmarshal(data, &inventory); err != nil {
		return Inventory{}, fmt.Errorf("trailconfig: retained snapshot inventory is corrupt: %w", err)
	}
	ref, err := configRef(inventory)
	if err != nil {
		return Inventory{}, err
	}
	if ref != expectedRef {
		return Inventory{}, fmt.Errorf("trailconfig: retained snapshot %s does not match its identity", expectedRef)
	}
	tree := filepath.Join(snapshotDir, SnapshotTreeDirName)
	for _, entry := range inventory.Entries {
		path := filepath.Join(tree, filepath.FromSlash(entry.Path))
		fileData, err := os.ReadFile(path)
		if err != nil {
			return Inventory{}, fmt.Errorf("trailconfig: configuration %s is missing %s: %w", expectedRef, entry.Path, err)
		}
		sum := sha256.Sum256(fileData)
		if hex.EncodeToString(sum[:]) != entry.SHA256 {
			return Inventory{}, fmt.Errorf("trailconfig: configuration %s has corrupt content at %s", expectedRef, entry.Path)
		}
	}
	return inventory, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.ConfigRevision < 1 {
		return fmt.Errorf("trailconfig: manifest configRevision must be positive")
	}
	if strings.TrimSpace(manifest.InitialConfigRef) == "" || strings.TrimSpace(manifest.ActiveConfigRef) == "" {
		return fmt.Errorf("trailconfig: manifest is missing configuration references")
	}
	if manifest.Policy != PolicyEditable && manifest.Policy != PolicyPinned {
		return fmt.Errorf("trailconfig: unknown configuration policy %q", manifest.Policy)
	}
	return nil
}

func validateTraceID(traceID string) error {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return fmt.Errorf("trailconfig: traceId is required")
	}
	if traceID == "." || traceID == ".." || strings.ContainsAny(traceID, `/\`) {
		return fmt.Errorf("trailconfig: invalid traceId %q", traceID)
	}
	return nil
}

type capturedFile struct {
	path string
	mode uint32
	data []byte
}

type sourceFile struct {
	logical string
	path    string
}

func capture(ctx colony.Context) ([]capturedFile, Resolved, error) {
	sources, err := collectSources(ctx)
	if err != nil {
		return nil, Resolved{}, err
	}

	var contents []capturedFile
	for _, src := range sources {
		data, err := os.ReadFile(src.path)
		if err != nil {
			return nil, Resolved{}, fmt.Errorf("trailconfig: read %s: %w", src.logical, err)
		}
		info, err := os.Stat(src.path)
		if err != nil {
			return nil, Resolved{}, fmt.Errorf("trailconfig: stat %s: %w", src.logical, err)
		}
		contents = append(contents, capturedFile{
			path: src.logical,
			mode: uint32(info.Mode().Perm()),
			data: data,
		})
	}
	contents, err = materializeEffectiveBees(ctx, contents)
	if err != nil {
		return nil, Resolved{}, err
	}
	sort.Slice(contents, func(i, j int) bool { return contents[i].path < contents[j].path })

	resolved := Resolved{
		Version: inventoryVersion,
		Profile: strings.TrimSpace(ctx.Profile),
		ProfileLayers: ProfileLayers{
			Colony: ctx.ProfileLayers.Colony,
			Home:   ctx.ProfileLayers.Home,
		},
		ModelAliases: normalizedAliases(ctx.ModelAliases),
	}
	return contents, resolved, nil
}

// materializeEffectiveBees replaces the committed bee definitions with the
// resolved execution view (process profile applied) so a bound snapshot never
// re-reads the live profile to decide how a role runs.
func materializeEffectiveBees(ctx colony.Context, contents []capturedFile) ([]capturedFile, error) {
	bees, err := ctx.LoadAllBees()
	if err != nil {
		return nil, fmt.Errorf("trailconfig: resolve effective bees: %w", err)
	}
	if len(bees) == 0 {
		return contents, nil
	}
	index := make(map[string]int, len(contents))
	for i, file := range contents {
		index[file.path] = i
	}
	for role, bee := range bees {
		data, err := yaml.Marshal(bee)
		if err != nil {
			return nil, fmt.Errorf("trailconfig: encode effective bee %q: %w", role, err)
		}
		logical := filepath.ToSlash(filepath.Join(".paseka", "bees", role+".yaml"))
		if i, ok := index[logical]; ok {
			contents[i].data = data
			contents[i].mode = 0o644
			continue
		}
		contents = append(contents, capturedFile{path: logical, mode: 0o644, data: data})
		index[logical] = len(contents) - 1
	}
	return contents, nil
}

func normalizedAliases(aliases map[string]string) map[string]string {
	if len(aliases) == 0 {
		return nil
	}
	out := make(map[string]string, len(aliases))
	for key, value := range aliases {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" && value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collectSources(ctx colony.Context) ([]sourceFile, error) {
	root := ctx.ColonyRoot
	seen := map[string]struct{}{}
	var sources []sourceFile
	addFile := func(logical, path string) {
		logical = filepath.ToSlash(logical)
		if _, ok := seen[logical]; ok {
			return
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return
		}
		seen[logical] = struct{}{}
		sources = append(sources, sourceFile{logical: logical, path: path})
	}
	addTree := func(logicalRoot, path string, include func(string) bool) error {
		err := filepath.WalkDir(path, func(current string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			if include != nil && !include(entry.Name()) {
				return nil
			}
			rel, err := filepath.Rel(path, current)
			if err != nil {
				return err
			}
			addFile(filepath.Join(logicalRoot, rel), current)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("trailconfig: walk %s: %w", path, err)
		}
		return nil
	}

	addFile(".paseka/colony.yaml", colony.PasekaPath(root, "colony.yaml"))
	if err := addTree(".paseka/bees", colony.PasekaPath(root, "bees"), yamlFile); err != nil {
		return nil, err
	}
	if err := addTree(".paseka/prompts", colony.PasekaPath(root, "prompts"), nil); err != nil {
		return nil, err
	}
	if err := addTree(".paseka/cues", colony.PasekaPath(root, "cues"), yamlFile); err != nil {
		return nil, err
	}
	if err := addTree(".paseka/skills", colony.PasekaPath(root, "skills"), nil); err != nil {
		return nil, err
	}
	if err := addTree(".agents/skills", filepath.Join(root, ".agents", "skills"), nil); err != nil {
		return nil, err
	}

	profile := strings.TrimSpace(ctx.Profile)
	if profile != "" {
		addFile(filepath.Join(".paseka", "profiles", profile+".yaml"), colony.PasekaPath(root, "profiles", profile+".yaml"))
	}

	if slug := strings.TrimSpace(ctx.Slug); slug != "" {
		home, err := colony.HomeDir(slug)
		if err == nil {
			if profile != "" {
				adapters := filepath.Join(home, "profiles", profile, "adapters")
				if err := addTree(filepath.Join("home", "profiles", profile, "adapters"), adapters, yamlFile); err != nil {
					return nil, err
				}
			}
			if err := addTree(filepath.Join("home", "skills"), filepath.Join(home, "skills"), nil); err != nil {
				return nil, err
			}
		}
	}

	return sources, nil
}

func yamlFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")
}

func buildInventory(contents []capturedFile, resolved Resolved) Inventory {
	entries := make([]Entry, 0, len(contents))
	for _, file := range contents {
		sum := sha256.Sum256(file.data)
		entries = append(entries, Entry{
			Path:    file.path,
			SHA256:  hex.EncodeToString(sum[:]),
			Mode:    file.mode,
			Size:    int64(len(file.data)),
			Private: PrivatePath(file.path),
		})
	}
	return Inventory{Version: inventoryVersion, Entries: entries, Resolved: resolved}
}

func configRef(inventory Inventory) (string, error) {
	// Layer participation is resolution provenance, not executed content. It
	// stays in the retained inventory but must not distinguish two snapshots
	// whose captured files and effective settings are equal.
	inventory.Resolved.ProfileLayers = ProfileLayers{}
	data, err := json.Marshal(inventory)
	if err != nil {
		return "", fmt.Errorf("trailconfig: encode inventory: %w", err)
	}
	sum := sha256.Sum256(data)
	return configRefPrefix + hex.EncodeToString(sum[:]), nil
}

func materializeSnapshot(configDir, ref string, contents []capturedFile, inventory Inventory) error {
	snapshotsDir := filepath.Join(configDir, SnapshotsDirName)
	if err := os.MkdirAll(snapshotsDir, 0o755); err != nil {
		return fmt.Errorf("trailconfig: mkdir snapshots: %w", err)
	}
	target := filepath.Join(snapshotsDir, ref)
	if _, err := os.Stat(target); err == nil {
		if verifyErr := verifySnapshot(target, ref); verifyErr != nil {
			return verifyErr
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("trailconfig: inspect snapshot: %w", err)
	}

	tmp, err := os.MkdirTemp(snapshotsDir, ".tmp-")
	if err != nil {
		return fmt.Errorf("trailconfig: create snapshot temp: %w", err)
	}
	renamed := false
	defer func() {
		if !renamed {
			_ = os.RemoveAll(tmp)
		}
	}()

	tree := filepath.Join(tmp, SnapshotTreeDirName)
	for _, file := range contents {
		dest := filepath.Join(tree, filepath.FromSlash(file.path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("trailconfig: mkdir snapshot tree: %w", err)
		}
		if err := os.WriteFile(dest, file.data, os.FileMode(file.mode)); err != nil {
			return fmt.Errorf("trailconfig: write snapshot file %s: %w", file.path, err)
		}
		if err := os.Chmod(dest, os.FileMode(file.mode)); err != nil {
			return fmt.Errorf("trailconfig: chmod snapshot file %s: %w", file.path, err)
		}
		if PrivatePath(file.path) {
			if err := restrictPrivateDirs(tree, dest); err != nil {
				return fmt.Errorf("trailconfig: restrict private input %s: %w", file.path, err)
			}
		}
	}
	if err := writeJSON(filepath.Join(tmp, InventoryFileName), inventory); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(tmp, ResolvedFileName), inventory.Resolved); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		if _, statErr := os.Stat(target); statErr == nil {
			if verifyErr := verifySnapshot(target, ref); verifyErr != nil {
				return verifyErr
			}
			return nil
		}
		return fmt.Errorf("trailconfig: publish snapshot: %w", err)
	}
	renamed = true
	return nil
}

// restrictPrivateDirs narrows traversal into a private input subtree without
// changing the file modes recorded in the inventory.
func restrictPrivateDirs(tree, dest string) error {
	for dir := filepath.Dir(dest); dir != tree && strings.HasPrefix(dir, tree+string(filepath.Separator)); dir = filepath.Dir(dir) {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

// ReadSnapshot verifies one retained snapshot and returns its inventory and
// file contents. Callers that expose configuration read through this seam so
// private classification travels with the bytes.
func ReadSnapshot(colonyRoot, traceID, ref string) (Inventory, []FileContent, error) {
	if colonyRoot == "" {
		return Inventory{}, nil, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return Inventory{}, nil, err
	}
	if err := validateConfigRef(ref); err != nil {
		return Inventory{}, nil, err
	}
	snapshotDir := filepath.Join(SnapshotDir(colonyRoot, traceID), ref)
	inventory, err := loadVerifiedSnapshot(snapshotDir, ref)
	if err != nil {
		return Inventory{}, nil, err
	}
	contents := make([]FileContent, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		data, err := os.ReadFile(filepath.Join(snapshotDir, SnapshotTreeDirName, filepath.FromSlash(entry.Path)))
		if err != nil {
			return Inventory{}, nil, fmt.Errorf("trailconfig: configuration %s is missing %s: %w", ref, entry.Path, err)
		}
		contents = append(contents, FileContent{Entry: entry, Data: data})
	}
	return inventory, contents, nil
}

func verifySnapshot(dir, expectedRef string) error {
	_, err := loadVerifiedSnapshot(dir, expectedRef)
	return err
}

func writeManifest(configDir string, manifest Manifest) error {
	tmp, err := os.CreateTemp(configDir, ".manifest-*.json")
	if err != nil {
		return fmt.Errorf("trailconfig: create manifest temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := writeJSONFile(tmp, manifest); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trailconfig: sync manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trailconfig: close manifest: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(configDir, ManifestFileName)); err != nil {
		return fmt.Errorf("trailconfig: publish manifest: %w", err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("trailconfig: encode %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("trailconfig: write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeJSONFile(file *os.File, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("trailconfig: encode manifest: %w", err)
	}
	data = append(data, '\n')
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("trailconfig: write manifest: %w", err)
	}
	return nil
}

func acquireLock(path string) (func(), error) {
	lock, err := filelock.Acquire(path, filelock.Options{
		Budget:     lockAcquireBudget,
		StaleAfter: lockStaleAfter,
	})
	if err != nil {
		return nil, fmt.Errorf("trailconfig: %w", err)
	}
	return lock.Release, nil
}
