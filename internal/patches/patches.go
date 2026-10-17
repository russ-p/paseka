// Package patches owns the per-run proposal patch archive: one immutable diff file
// plus a metadata sidecar per runtime-generated proposal attempt.
//
// A patch is a snapshot of collector output at proposal finalization, kept on disk under
// the trail's run tree. It is deliberately not a trail comb artifact: comb artifacts are
// handoffs a Bee writes, while a patch is captured code history the runtime owns and only
// the runtime writes.
//
// The store is the archive of collector output. It is not a proof of the repository state
// a Guard approved, not a strict delta from one run's starting dirty state, and not a
// workspace backup. See docs/specs/039-proposal-patch-records.md.
package patches

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
)

// DirName is the reserved subdirectory under .paseka/runs/<traceId>/ that holds
// proposal patches. It is registered in the runs package reserved-subdirectory set and
// its no-per-agent-events set, so it is never read as a Bee run directory and never
// merged into trail event reads.
const DirName = "proposals"

// SchemaVersion is the sidecar schema version, bumped when its shape changes.
const SchemaVersion = 1

// MaxPreviewBytes caps a patch body the Console will render on one screen.
//
// Separate from the export cap on purpose: an operator who hits "too large to inline
// preview" and raises the one constant they can find has raised the size of every export
// report, which is the wrong lever for the symptom.
const MaxPreviewBytes = 512 * 1024

const (
	patchSuffix = ".patch"
	metaSuffix  = ".json"
)

// ExecutionMode records which runtime path produced the record.
const (
	ModeAFK  = "afk"
	ModeHITL = "hitl"
)

// PublicationState says what was confirmed about the synthesized event at capture time.
// It is not an authority on the bus after the fact: a later purge removes the event
// while the record keeps the state it was given.
const (
	// PublicationUnpublished means the complete record exists but no broker attempt was made.
	PublicationUnpublished = "unpublished"
	// PublicationUnconfirmed means an attempt was made and its outcome is not known.
	PublicationUnconfirmed = "unconfirmed"
	// PublicationPublished means a broker acknowledged the event.
	PublicationPublished = "published"
)

// Store is one trail's proposal patch store.
type Store struct {
	ColonyRoot string
	TraceID    string
}

// NewStore returns the proposal store for one trail. Neither path is normalized here;
// callers pass the same colony root the rest of the runtime uses.
func NewStore(colonyRoot, traceID string) Store {
	return Store{ColonyRoot: colonyRoot, TraceID: traceID}
}

// Root returns the absolute proposal store directory. The directory is not created.
func (s Store) Root() string {
	return filepath.Join(colony.PasekaPath(s.ColonyRoot, "runs", s.TraceID), DirName)
}

// PatchPath returns the absolute immutable diff path for one agent run.
func (s Store) PatchPath(agentID string) string {
	return filepath.Join(s.Root(), validAgentID(agentID)+patchSuffix)
}

// MetaPath returns the absolute sidecar path for one agent run.
func (s Store) MetaPath(agentID string) string {
	return filepath.Join(s.Root(), validAgentID(agentID)+metaSuffix)
}

// Ref returns the colony-root-relative ref a synthesized MUTATION carries for one agent
// run. The ref resolves against ColonyRoot, never against a Guard cwd, a sector, or a
// worktree root.
func (s Store) Ref(agentID string) string {
	return filepath.ToSlash(filepath.Join(
		".paseka", "runs", s.TraceID, DirName, validAgentID(agentID)+patchSuffix))
}

// validAgentID returns agentID when it is already a bare identifier and "" otherwise.
//
// Agent ids are minted per run and are plain hex, so a separator or a traversal is a
// caller bug rather than a value to normalize. Taking the basename instead would let two
// different ids address one record, which is worse than refusing both.
func validAgentID(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || agentID == "." || agentID == ".." {
		return ""
	}
	if strings.ContainsAny(agentID, `/\`) {
		return ""
	}
	return agentID
}

// ArchiveRef identifies an archive ref string: trace id, agent id, and whether it is a
// well-formed patch ref. Historical object-store refs and explicitly authored refs do
// not parse here, which is what keeps them from being reinterpreted as archive files.
type ArchiveRef struct {
	TraceID string
	AgentID string
}

// IsArchiveRef reports whether ref is a colony-root-relative path into a trail's
// proposal store. It is the single discriminator between a new synthesized archive ref
// and every other ref a MUTATION may carry.
func IsArchiveRef(ref string) bool {
	_, ok := ParseArchiveRef(ref)
	return ok
}

// ParseArchiveRef splits a colony-root-relative patch ref into its trace and agent ids.
func ParseArchiveRef(ref string) (ArchiveRef, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ArchiveRef{}, false
	}
	norm := filepath.ToSlash(strings.TrimPrefix(ref, "/"))
	parts := strings.Split(norm, "/")
	if len(parts) != 5 {
		return ArchiveRef{}, false
	}
	if parts[0] != ".paseka" || parts[1] != "runs" || parts[3] != DirName {
		return ArchiveRef{}, false
	}
	if !strings.HasSuffix(parts[4], patchSuffix) {
		return ArchiveRef{}, false
	}
	traceID := strings.TrimSpace(parts[2])
	agentID := strings.TrimSuffix(parts[4], patchSuffix)
	if traceID == "" || agentID == "" || validAgentID(agentID) != agentID {
		return ArchiveRef{}, false
	}
	if strings.ContainsAny(traceID, "/\\") || traceID == "." || traceID == ".." {
		return ArchiveRef{}, false
	}
	return ArchiveRef{TraceID: traceID, AgentID: agentID}, true
}

// ResolveRef maps a colony-root-relative ref to an absolute path inside its own trail's
// proposal store. A ref naming another trail, a different reserved directory, or a path
// that escapes through a symlink is refused.
func ResolveRef(colonyRoot, ref string) (string, error) {
	parsed, ok := ParseArchiveRef(ref)
	if !ok {
		return "", fmt.Errorf("patches: not a proposal archive ref: %q", ref)
	}
	abs, err := Store{ColonyRoot: colonyRoot, TraceID: parsed.TraceID}.resolveAgent(parsed.AgentID)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// traceRoot is the trace's run tree, the boundary the proposal store must stay inside.
func (s Store) traceRoot() string {
	return colony.PasekaPath(s.ColonyRoot, "runs", s.TraceID)
}

// resolveRoot returns the absolute store directory after checking that the directory
// itself has not been redirected outside its trace's run tree. Components that do not
// exist yet are allowed, so the first write can create the directory.
func (s Store) resolveRoot() (string, error) {
	root, err := filepath.Abs(s.Root())
	if err != nil {
		return "", err
	}
	traceRoot, err := filepath.Abs(s.traceRoot())
	if err != nil {
		return "", err
	}
	if err := containPath(traceRoot, root); err != nil {
		return "", fmt.Errorf("patches: proposal store escapes the trace run tree: %w", err)
	}
	return root, nil
}

// resolveFile returns an absolute path checked against the store root, so a symlink
// anywhere below the store cannot redirect a read or a write outside it.
func (s Store) resolveFile(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	root, err := s.resolveRoot()
	if err != nil {
		return "", err
	}
	if err := containPath(root, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// resolveAgent returns the absolute patch path for one agent id, rejecting anything that
// leaves the store directory.
func (s Store) resolveAgent(agentID string) (string, error) {
	if validAgentID(agentID) == "" {
		return "", fmt.Errorf("patches: agent id is required")
	}
	return s.resolveFile(s.PatchPath(agentID))
}

// resolveMeta returns the absolute sidecar path for one agent id under the same
// containment rules as the patch body.
func (s Store) resolveMeta(agentID string) (string, error) {
	if validAgentID(agentID) == "" {
		return "", fmt.Errorf("patches: agent id is required")
	}
	return s.resolveFile(s.MetaPath(agentID))
}
