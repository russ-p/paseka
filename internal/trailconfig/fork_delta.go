package trailconfig

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/protocol"
)

// ForkDelta is the resolved candidate produced by applying a recorded proposal
// patch to a retained source snapshot.
//
// Contents and Inventory are the complete local inputs and their identity; the
// caller retains them in the fork. RawDiff is the file-level difference between
// the source tree and the applied patch. EffectiveDiff is the resolved behaviour
// that actually changes. ShadowedDiff names requested changes that a frozen
// higher-priority layer keeps from taking effect.
type ForkDelta struct {
	BaseConfigRef string
	ConfigRef     string
	Inventory     Inventory
	Contents      []capturedFile
	RawDiff       []string
	EffectiveDiff []string
	ShadowedDiff  []string
	// TaskBee is the resolved bee role for the carried task under the candidate.
	TaskBee string
}

// ForkDeltaOptions selects one retained source snapshot and one recorded patch
// body. The patch is applied to the snapshot's portable tree, never to the live
// colony checkout.
type ForkDeltaOptions struct {
	Context         colony.Context
	SourceTraceID   string
	SourceConfigRef string
	Patch           []byte
	PatchName       string
	// TaskID and TaskInput, when set, validate the carried task against the
	// candidate roles, sectors, review policy, and templates.
	TaskID    string
	TaskInput *TaskInputEvidence
	// ResolvedTaskBee is the source attempt's captured effective role for an
	// omitted task bee. It must not be re-derived from the candidate defaults.
	ResolvedTaskBee string
}

// PrepareForkDelta applies patch bytes to the named source snapshot, validates
// the result, and computes its candidate ConfigRef.
//
// It never writes into the source trail or the live colony. The only writes are
// to a temporary tree that is removed before returning. The source's frozen
// resolved settings (profile and effective model aliases) are retained verbatim,
// so a patch cannot silently reselect the process profile or re-read machine-local
// overrides.
func PrepareForkDelta(opts ForkDeltaOptions) (ForkDelta, error) {
	if opts.Context.ColonyRoot == "" {
		return ForkDelta{}, fmt.Errorf("trailconfig: colony root is required")
	}
	if err := validateTraceID(opts.SourceTraceID); err != nil {
		return ForkDelta{}, err
	}
	if err := validateConfigRef(opts.SourceConfigRef); err != nil {
		return ForkDelta{}, err
	}
	if len(opts.Patch) == 0 {
		return ForkDelta{}, fmt.Errorf("trailconfig: delta patch is empty")
	}

	sourceDir := filepath.Join(SnapshotDir(opts.Context.ColonyRoot, opts.SourceTraceID), opts.SourceConfigRef)
	inventory, err := loadVerifiedSnapshot(sourceDir, opts.SourceConfigRef)
	if err != nil {
		return ForkDelta{}, err
	}
	contents, err := readSnapshotContents(sourceDir, inventory)
	if err != nil {
		return ForkDelta{}, err
	}
	sourceTree := filepath.Join(sourceDir, SnapshotTreeDirName)

	if err := validatePatchPaths(opts.Patch, opts.PatchName); err != nil {
		return ForkDelta{}, err
	}

	tree, cleanup, err := materializeCandidateTree(opts.SourceConfigRef, contents, inventory)
	if err != nil {
		return ForkDelta{}, err
	}
	defer cleanup()

	if err := applyPortablePatch(tree, opts.Patch, opts.PatchName); err != nil {
		return ForkDelta{}, err
	}
	if err := rejectUnsafeTreeEntries(tree); err != nil {
		return ForkDelta{}, err
	}
	candidateContents, err := readCapturedTree(tree)
	if err != nil {
		return ForkDelta{}, err
	}
	// The source's frozen resolved view remains authoritative. A patch may change
	// portable files but cannot reselect the profile or local alias overrides.
	candidateInventory := buildInventory(candidateContents, inventory.Resolved)
	candidateRef, err := configRef(candidateInventory)
	if err != nil {
		return ForkDelta{}, err
	}
	changed := diffInventories(inventory.Entries, candidateInventory.Entries)
	if err := validatePortableChanges(changed); err != nil {
		return ForkDelta{}, err
	}
	raw, effective, shadowed := compareForkCandidate(sourceTree, tree, inventory, candidateInventory)
	taskBee := ""
	if opts.TaskInput != nil {
		candidateColony, err := colony.LoadColony(tree)
		if err != nil {
			return ForkDelta{}, err
		}
		taskBee = strings.TrimSpace(opts.ResolvedTaskBee)
		if taskBee == "" {
			taskBee = colony.EffectiveTaskBee(opts.TaskInput.Bee, candidateColony.Defaults)
		}
		resolvedInput := *opts.TaskInput
		resolvedInput.Bee = taskBee
		if err := ValidateForkCandidateTree(sourceTree, tree, opts.TaskID, &resolvedInput); err != nil {
			return ForkDelta{}, err
		}
	} else if err := ValidateForkCandidateTree(sourceTree, tree, opts.TaskID, nil); err != nil {
		return ForkDelta{}, err
	}

	return ForkDelta{
		BaseConfigRef: opts.SourceConfigRef,
		ConfigRef:     candidateRef,
		Inventory:     candidateInventory,
		Contents:      candidateContents,
		RawDiff:       raw,
		EffectiveDiff: effective,
		ShadowedDiff:  shadowed,
		TaskBee:       taskBee,
	}, nil
}

// ValidateForkCandidateTree validates a candidate portable tree against the
// source snapshot's policy and the task it must run.
//
// taskID and input may be empty when validating only the configuration as a
// whole. The check covers colony identity policy, bee roles, prompt templates,
// skills, adapters, event/completion contracts, sectors, and the carried task's
// review policy.
func ValidateForkCandidateTree(baseTree, candidateTree, taskID string, input *TaskInputEvidence) error {
	baseColony, err := colony.LoadColony(baseTree)
	if err != nil {
		return fmt.Errorf("trailconfig: source configuration is unreadable: %w", err)
	}

	var work []ProcedureWork
	if input != nil {
		beeRole := colony.EffectiveTaskBee(input.Bee, baseColony.Defaults)
		work = append(work, ProcedureWork{
			TaskID: taskID,
			Title:  input.Title,
			Body:   input.Body,
			Bee:    beeRole,
			Sector: input.Sector,
			Intent: input.Intent,
			Review: protocol.TaskReviewPolicy(input.Review),
		})
	}
	return ValidateProcedure(candidateTree, baseColony, work)
}

func validatePolicyUnchanged(base, candidate colony.Colony) error {
	if strings.TrimSpace(base.Slug) != strings.TrimSpace(candidate.Slug) {
		return fmt.Errorf("trailconfig: delta patch cannot change colony identity (slug)")
	}
	if base.NATS != candidate.NATS {
		return fmt.Errorf("trailconfig: delta patch cannot change colony transport (nats)")
	}
	if base.Defaults.EnergyBudget != candidate.Defaults.EnergyBudget {
		return fmt.Errorf("trailconfig: delta patch cannot change the colony energy budget")
	}
	if base.Defaults.ResolvedDelivery() != candidate.Defaults.ResolvedDelivery() {
		return fmt.Errorf("trailconfig: delta patch cannot change colony delivery policy")
	}
	return nil
}

func validatePromptTemplateFile(root, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	if filepath.IsAbs(ref) || strings.Contains(ref, "..") {
		return fmt.Errorf("prompt template %q escapes the prompts directory", ref)
	}
	promptsRoot := filepath.Join(root, ".paseka", "prompts")
	target := filepath.Join(promptsRoot, filepath.FromSlash(ref))
	rel, err := filepath.Rel(promptsRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("prompt template %q escapes the prompts directory", ref)
	}
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("prompt template %q is missing", ref)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("prompt template %q is not a regular file", ref)
	}
	return nil
}

func validateSkillReferences(root string) error {
	roots := []string{
		filepath.Join(root, ".paseka", "skills"),
		filepath.Join(root, ".agents", "skills"),
		filepath.Join(root, "home", "skills"),
	}
	for _, skillsRoot := range roots {
		entries, err := os.ReadDir(skillsRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("trailconfig: read skills: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			skill := filepath.Join(skillsRoot, entry.Name(), "SKILL.md")
			info, err := os.Stat(skill)
			if err != nil {
				return fmt.Errorf("trailconfig: skill %q is missing SKILL.md", entry.Name())
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("trailconfig: skill %q has a non-regular SKILL.md", entry.Name())
			}
		}
	}
	return nil
}

// validatePortableChanges refuses a patch outside the portable configuration
// tree. Application source, runtime data, worktrees, and machine-local
// credentials are not part of the delta's authority.
func validatePortableChanges(changed []string) error {
	for _, line := range changed {
		p := diffLinePath(line)
		if p == "" {
			continue
		}
		if !allowedPortablePath(p) {
			return fmt.Errorf("trailconfig: delta patch may not change %s; only portable .paseka configuration and shared skills are allowed", p)
		}
	}
	return nil
}

func allowedPortablePath(p string) bool {
	p = path.Clean(filepath.ToSlash(p))
	if p == "." || strings.HasPrefix(p, "../") || path.IsAbs(p) {
		return false
	}
	if strings.Contains(p, "/../") || strings.HasSuffix(p, "/..") {
		return false
	}
	if strings.HasPrefix(p, ".paseka/") {
		if strings.HasPrefix(p, ".paseka/runs/") || strings.HasPrefix(p, ".paseka/worktrees/") {
			return false
		}
		if strings.HasPrefix(p, ".paseka/credentials") || strings.HasPrefix(p, ".paseka/secrets") {
			return false
		}
		return true
	}
	return strings.HasPrefix(p, ".agents/skills/")
}

func diffLinePath(line string) string {
	for _, prefix := range []string{"added ", "changed ", "removed "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// patchGitEnv keeps the temporary candidate tree's git commands from targeting
// a repository selected by the caller's environment. A hook or agent shell can
// export GIT_DIR/GIT_WORK_TREE, and inheriting them would make `git apply` write
// into the live worktree instead of the candidate tree.
var patchGitEnv = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
}

func applyPortablePatch(tree string, patch []byte, name string) error {
	patchDir, err := os.MkdirTemp("", "paseka-fork-patch-")
	if err != nil {
		return fmt.Errorf("trailconfig: create patch temp: %w", err)
	}
	defer func() { _ = os.RemoveAll(patchDir) }()
	patchPath := filepath.Join(patchDir, "delta.patch")
	if err := os.WriteFile(patchPath, patch, 0o600); err != nil {
		return fmt.Errorf("trailconfig: write patch temp: %w", err)
	}

	if _, err := gitroot.Run(gitroot.RunOpts{Dir: tree, Args: []string{"init", "-q"}, UnsetEnv: patchGitEnv}); err != nil {
		return fmt.Errorf("trailconfig: prepare delta tree: %w", err)
	}
	if _, err := gitroot.Run(gitroot.RunOpts{
		Dir:      tree,
		Args:     []string{"apply", "--check", "--whitespace=nowarn", patchPath},
		UnsetEnv: patchGitEnv,
	}); err != nil {
		return fmt.Errorf("trailconfig: delta patch does not apply to the source configuration%s: %w", patchLabel(name), err)
	}
	if _, err := gitroot.Run(gitroot.RunOpts{
		Dir:      tree,
		Args:     []string{"apply", "--whitespace=nowarn", patchPath},
		UnsetEnv: patchGitEnv,
	}); err != nil {
		return fmt.Errorf("trailconfig: apply delta patch%s: %w", patchLabel(name), err)
	}
	if err := os.RemoveAll(filepath.Join(tree, ".git")); err != nil {
		return fmt.Errorf("trailconfig: clean delta tree: %w", err)
	}
	return nil
}

func patchLabel(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return " " + name
}

func rejectUnsafeTreeEntries(tree string) error {
	return filepath.WalkDir(tree, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(tree, current)
			return fmt.Errorf("trailconfig: delta patch may not create symbolic link %s", filepath.ToSlash(rel))
		}
		if !entry.Type().IsRegular() {
			rel, _ := filepath.Rel(tree, current)
			return fmt.Errorf("trailconfig: delta patch produced a non-regular file %s", filepath.ToSlash(rel))
		}
		return nil
	})
}

// validatePatchPaths rejects traversal and absolute paths in patch headers
// before git sees them. False positives inside hunks are avoided by tracking the
// hunk state.
func validatePatchPaths(patch []byte, name string) error {
	scanner := bufio.NewScanner(bytes.NewReader(patch))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	inHunk := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
			rest := strings.TrimSpace(strings.TrimPrefix(line, "diff --git "))
			for _, token := range strings.Fields(rest) {
				if err := validatePatchToken(token, name); err != nil {
					return err
				}
			}
			continue
		case strings.HasPrefix(line, "@@"):
			inHunk = true
			continue
		case inHunk:
			continue
		}
		for _, prefix := range []string{"--- ", "+++ ", "rename from ", "rename to ", "copy from ", "copy to "} {
			if strings.HasPrefix(line, prefix) {
				if err := validatePatchToken(strings.TrimSpace(strings.TrimPrefix(line, prefix)), name); err != nil {
					return err
				}
				break
			}
		}
		if strings.HasPrefix(line, "new mode 120000") || strings.HasPrefix(line, "old mode 120000") {
			return fmt.Errorf("trailconfig: delta patch%s may not create symbolic links", patchLabel(name))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("trailconfig: read delta patch: %w", err)
	}
	return nil
}

func validatePatchToken(token, name string) error {
	token = strings.TrimSpace(token)
	if token == "" || token == "/dev/null" {
		return nil
	}
	if strings.HasPrefix(token, `"`) {
		unquoted, err := strconv.Unquote(token)
		if err != nil {
			return fmt.Errorf("trailconfig: delta patch%s has an invalid quoted path", patchLabel(name))
		}
		token = unquoted
	}
	token = strings.TrimPrefix(token, "a/")
	token = strings.TrimPrefix(token, "b/")
	cleaned := path.Clean(token)
	if path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return fmt.Errorf("trailconfig: delta patch%s path %q escapes the configuration tree", patchLabel(name), token)
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git/") {
		return fmt.Errorf("trailconfig: delta patch%s may not change git metadata", patchLabel(name))
	}
	return nil
}

// compareForkCandidate reports the raw file delta, the effective behavioural
// delta, and the changes shadowed by the source's frozen resolution.
func compareForkCandidate(baseTree, candidateTree string, base, candidate Inventory) ([]string, []string, []string) {
	raw := diffInventories(base.Entries, candidate.Entries)

	var effective, shadowed []string
	baseBees, baseErr := effectiveBees(baseTree)
	candidateBees, candidateErr := effectiveBees(candidateTree)
	if baseErr == nil && candidateErr == nil {
		effective = append(effective, beeDiffs(baseBees, candidateBees)...)
	}
	effectiveRoles := beeDiffRoles(effective)

	baseColony, baseColonyErr := colony.LoadColony(baseTree)
	candidateColony, candidateColonyErr := colony.LoadColony(candidateTree)
	var colonyDiffs []string
	if baseColonyErr == nil && candidateColonyErr == nil {
		colonyDiffs = colonyEffectiveDiffs(baseColony, candidateColony)
		shadowed = append(shadowed, shadowedAliasDiffs(baseColony, candidateColony, base.Resolved.ModelAliases)...)
	}
	effective = append(effective, colonyDiffs...)

	for _, line := range raw {
		p := diffLinePath(line)
		switch {
		case isUsedContentPath(p):
			effective = append(effective, line)
		case isBeePath(p):
			role := beeRoleFromPath(p)
			if effectiveRoles[role] {
				effective = append(effective, line)
			} else {
				shadowed = append(shadowed, line+": effective bee settings are unchanged")
			}
		case strings.HasPrefix(p, ".paseka/profiles/"):
			shadowed = append(shadowed, line+": shadowed by the source's frozen profile resolution")
		case p == ".paseka/colony.yaml":
			if len(colonyDiffs) > 0 {
				effective = append(effective, line)
			} else {
				shadowed = append(shadowed, line+": effective colony settings are unchanged; identity, transport, budget, delivery, and frozen aliases keep precedence")
			}
		default:
			effective = append(effective, line)
		}
	}

	return normalizePreviewLines(raw), normalizePreviewLines(effective), normalizePreviewLines(shadowed)
}

// effectiveBees returns the bee settings that execution actually sees, including
// the source's machine-local prompt/system overlays. LoadAllBees alone ignores
// those overlays, which would report a base-bee change as effective even when a
// frozen local overlay still wins.
func effectiveBees(root string) (map[string]colony.Bee, error) {
	bees, err := colony.LoadAllBees(root)
	if err != nil {
		return nil, err
	}
	for role, bee := range bees {
		_, overlay, err := colony.LoadBee(root, role)
		if err != nil {
			return nil, err
		}
		if prompt := strings.TrimSpace(overlay.PromptTemplate); prompt != "" {
			bee.PromptTemplate = prompt
		}
		if system := strings.TrimSpace(overlay.SystemTemplate); system != "" {
			bee.SystemTemplate = system
		}
		bees[role] = bee
	}
	return bees, nil
}

func isUsedContentPath(p string) bool {
	for _, prefix := range []string{".paseka/prompts/", ".paseka/skills/", ".paseka/cues/", ".agents/skills/"} {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func isBeePath(p string) bool {
	return strings.HasPrefix(p, ".paseka/bees/")
}

func beeRoleFromPath(p string) string {
	name := strings.TrimPrefix(p, ".paseka/bees/")
	if strings.Contains(name, "/") {
		return ""
	}
	name = strings.TrimSuffix(name, ".local.yaml")
	name = strings.TrimSuffix(name, ".yaml")
	return name
}

func beeDiffRoles(lines []string) map[string]bool {
	out := make(map[string]bool)
	for _, line := range lines {
		if !strings.HasPrefix(line, "bee ") {
			continue
		}
		rest := strings.TrimPrefix(line, "bee ")
		role := rest
		if i := strings.IndexAny(rest, " :"); i >= 0 {
			role = rest[:i]
		}
		if role != "" {
			out[role] = true
		}
	}
	return out
}

// colonyEffectiveDiffs reports behavioural manifest changes that survive the
// fork's frozen policy. Identity, transport, budget, and delivery are refused
// before this point; model aliases stay frozen and are reported as shadowed.
func colonyEffectiveDiffs(base, candidate colony.Colony) []string {
	var out []string
	if strings.TrimSpace(base.Defaults.DefaultBee) != strings.TrimSpace(candidate.Defaults.DefaultBee) {
		out = append(out, fmt.Sprintf("default bee: %s -> %s", previewValue(base.Defaults.DefaultBee), previewValue(candidate.Defaults.DefaultBee)))
	}
	if strings.TrimSpace(base.Defaults.PromptTemplate) != strings.TrimSpace(candidate.Defaults.PromptTemplate) {
		out = append(out, fmt.Sprintf("default prompt template: %s -> %s", previewValue(base.Defaults.PromptTemplate), previewValue(candidate.Defaults.PromptTemplate)))
	}
	if strings.TrimSpace(base.Defaults.SystemTemplate) != strings.TrimSpace(candidate.Defaults.SystemTemplate) {
		out = append(out, fmt.Sprintf("default system template: %s -> %s", previewValue(base.Defaults.SystemTemplate), previewValue(candidate.Defaults.SystemTemplate)))
	}
	out = append(out, sectorDiffs(base, candidate)...)
	if settingJSON(base.AutoInvites) != settingJSON(candidate.AutoInvites) {
		out = append(out, "auto_invites: changed")
	}
	return out
}

func sectorDiffs(base, candidate colony.Colony) []string {
	names := make(map[string]struct{}, len(base.Sectors)+len(candidate.Sectors))
	for name := range base.Sectors {
		names[name] = struct{}{}
	}
	for name := range candidate.Sectors {
		names[name] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	var out []string
	for _, name := range sorted {
		before, hasBefore := base.Sectors[name]
		after, hasAfter := candidate.Sectors[name]
		switch {
		case !hasAfter:
			out = append(out, "sector "+name+": removed")
		case !hasBefore:
			out = append(out, "sector "+name+": added")
		default:
			beforeJSON, _ := json.Marshal(before)
			afterJSON, _ := json.Marshal(after)
			if string(beforeJSON) != string(afterJSON) {
				out = append(out, "sector "+name+": changed")
			}
		}
	}
	return out
}

func shadowedAliasDiffs(base, candidate colony.Colony, frozen map[string]string) []string {
	keys := make(map[string]struct{}, len(base.ModelAliases)+len(candidate.ModelAliases))
	for key := range base.ModelAliases {
		keys[key] = struct{}{}
	}
	for key := range candidate.ModelAliases {
		keys[key] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for key := range keys {
		sorted = append(sorted, key)
	}
	sort.Strings(sorted)

	var out []string
	for _, key := range sorted {
		before := strings.TrimSpace(base.ModelAliases[key])
		after := strings.TrimSpace(candidate.ModelAliases[key])
		if before == after {
			continue
		}
		effective, ok := frozen[key]
		if !ok {
			out = append(out, fmt.Sprintf("model alias %s: patch %s -> %s is shadowed by the source's frozen resolution (no effective alias)", key, previewValue(before), previewValue(after)))
			continue
		}
		out = append(out, fmt.Sprintf("model alias %s: patch %s -> %s is shadowed by the source's frozen resolution (%s)", key, previewValue(before), previewValue(after), effective))
	}
	return out
}
