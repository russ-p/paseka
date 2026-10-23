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
	"github.com/russ-p/paseka/internal/gitroot"
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

// Revision is one entry in the append-only binding history.
type Revision struct {
	Revision         int       `json:"revision"`
	InitialConfigRef string    `json:"initialConfigRef"`
	ActiveConfigRef  string    `json:"activeConfigRef"`
	Source           string    `json:"source,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

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
	InitialCodeBaseSHA        string     `json:"initialCodeBaseSha,omitempty"`
	InitialCodeBaseDirty      bool       `json:"initialCodeBaseDirty,omitempty"`
	InitialCodeBaseCapturedAt time.Time  `json:"initialCodeBaseCapturedAt,omitempty"`
	CreatedAt                 time.Time  `json:"createdAt,omitempty"`
	UpdatedAt                 time.Time  `json:"updatedAt,omitempty"`
	Snapshots                 []Snapshot `json:"snapshots"`
	History                   []Revision `json:"history"`
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
	if ctx.ColonyRoot == "" {
		return Manifest{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(traceID); err != nil {
		return Manifest{}, err
	}
	dir := ConfigDir(ctx.ColonyRoot, traceID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Manifest{}, fmt.Errorf("trailconfig: mkdir %s: %w", dir, err)
	}

	release, err := acquireLock(filepath.Join(dir, lockFileName))
	if err != nil {
		return Manifest{}, err
	}
	defer release()

	existing, err := Load(ctx.ColonyRoot, traceID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNoBinding) {
		return Manifest{}, err
	}

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
			Source:           "colony",
			CreatedAt:        now,
		}},
	}
	if err := writeManifest(dir, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
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
	}, nil
}

// Bind returns the trail's active configuration as an immutable execution
// context. It captures revision 1 when the trail has no binding yet.
func Bind(ctx colony.Context, traceID string) (colony.Context, Manifest, error) {
	manifest, err := Ensure(ctx, traceID)
	if err != nil {
		return colony.Context{}, Manifest{}, err
	}
	bound, err := BindRef(ctx, traceID, manifest.ActiveConfigRef, manifest.ConfigRevision)
	if err != nil {
		return colony.Context{}, Manifest{}, err
	}
	return bound, manifest, nil
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
	bound := ctx
	bound.ConfigRoot = filepath.Join(snapshotDir, SnapshotTreeDirName)
	bound.ConfigRef = ref
	bound.ConfigRevision = revision
	bound.WorkspaceRoot = ctx.WorkspaceRoot
	if bound.WorkspaceRoot == "" {
		bound.WorkspaceRoot = ctx.ColonyRoot
	}
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
			Path:   file.path,
			SHA256: hex.EncodeToString(sum[:]),
			Mode:   file.mode,
			Size:   int64(len(file.data)),
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
	deadline := time.Now().Add(lockAcquireBudget)
	for {
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("trailconfig: acquire lock: %w", err)
		}
		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > lockStaleAfter {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("trailconfig: timed out waiting for binding lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
