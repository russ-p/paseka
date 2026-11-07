package nuc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/skills"
)

type fileAction string

const (
	actionCreated     fileAction = "created"
	actionSkipped     fileAction = "skipped"
	actionOverwritten fileAction = "overwritten"
)

// Import applies a nuc document to a colony directory.
func Import(doc Document, opts ImportOptions) (ImportResult, error) {
	if err := doc.Validate(); err != nil {
		return ImportResult{}, err
	}
	if opts.ColonyRoot == "" {
		return ImportResult{}, fmt.Errorf("nuc: colony root is required")
	}

	// Skills are validated as a whole before the first byte lands, so one bad
	// carried skill refuses the pack without leaving a half-imported colony.
	carried, err := prepareCarriedSkills(doc)
	if err != nil {
		return ImportResult{}, err
	}

	var res ImportResult
	beesDir := colony.BeesDir(opts.ColonyRoot)
	promptsDir := colony.PasekaPath(opts.ColonyRoot, "prompts")

	roles := make([]string, 0, len(doc.Spec.Bees))
	for role := range doc.Spec.Bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	for _, role := range roles {
		body := doc.Spec.Bees[role]
		dest := filepath.Join(beesDir, role+".yaml")
		action, err := writeColonyFile(dest, []byte(body), opts.Force, opts.DryRun)
		if err != nil {
			return res, fmt.Errorf("nuc: bee %q: %w", role, err)
		}
		appendAction(&res, action, dest)
	}

	promptPaths := make([]string, 0, len(doc.Spec.Prompts))
	for path := range doc.Spec.Prompts {
		promptPaths = append(promptPaths, path)
	}
	sort.Strings(promptPaths)

	for _, ref := range promptPaths {
		content := doc.Spec.Prompts[ref]
		dest := filepath.Join(promptsDir, filepath.FromSlash(ref))
		action, err := writeColonyFile(dest, []byte(content), opts.Force, opts.DryRun)
		if err != nil {
			return res, fmt.Errorf("nuc: prompt %q: %w", ref, err)
		}
		appendAction(&res, action, dest)
	}

	cuesDir := cues.Dir(opts.ColonyRoot)
	cueIDs := make([]string, 0, len(doc.Spec.Cues))
	for id := range doc.Spec.Cues {
		cueIDs = append(cueIDs, id)
	}
	sort.Strings(cueIDs)

	for _, id := range cueIDs {
		body := doc.Spec.Cues[id]
		dest := filepath.Join(cuesDir, id+".yaml")
		action, err := writeColonyFile(dest, []byte(body), opts.Force, opts.DryRun)
		if err != nil {
			return res, fmt.Errorf("nuc: cue %q: %w", id, err)
		}
		appendAction(&res, action, dest)
	}

	if err := importCarriedSkills(opts.ColonyRoot, carried, opts, &res); err != nil {
		return res, err
	}

	if !opts.DryRun {
		for _, role := range roles {
			if _, _, err := colony.LoadBee(opts.ColonyRoot, role); err != nil {
				return res, fmt.Errorf("nuc: validate bee %q after import: %w", role, err)
			}
		}
		for _, id := range cueIDs {
			if _, err := cues.Load(opts.ColonyRoot, id); err != nil {
				return res, fmt.Errorf("nuc: validate cue %q after import: %w", id, err)
			}
		}
	}

	return res, nil
}

// carriedSkill is one validated, rewritten skill ready to install.
type carriedSkill struct {
	name  string
	files map[string][]byte
	hash  string
}

// prepareCarriedSkills validates every carried skill with the shared rule set
// and stages the rewritten bytes outside the colony, so an invalid skill
// refuses the pack before any bee, prompt, cue, or skill is written.
func prepareCarriedSkills(doc Document) ([]carriedSkill, error) {
	if len(doc.Spec.Skills) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(doc.Spec.Skills))
	for name := range doc.Spec.Skills {
		names = append(names, name)
	}
	sort.Strings(names)

	stagingRoot, err := os.MkdirTemp("", "paseka-nuc-skills-*")
	if err != nil {
		return nil, fmt.Errorf("nuc: create skill staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingRoot) }()

	carried := make([]carriedSkill, 0, len(names))
	for _, name := range names {
		files := make(map[string][]byte, len(doc.Spec.Skills[name]))
		for rel, body := range doc.Spec.Skills[name] {
			files[rel] = []byte(body)
		}
		prepared, err := skills.PrepareNucSkill(name, files)
		if err != nil {
			return nil, err
		}
		stagedDir := filepath.Join(stagingRoot, name)
		for rel, data := range prepared.Files {
			dest, err := joinUnder(stagedDir, rel)
			if err != nil {
				return nil, fmt.Errorf("nuc: skill %q: %w", name, err)
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return nil, fmt.Errorf("nuc: stage skill %q: %w", name, err)
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return nil, fmt.Errorf("nuc: stage skill %q: %w", name, err)
			}
		}
		carried = append(carried, carriedSkill{
			name:  prepared.Name,
			files: prepared.Files,
			hash:  prepared.ProcedureHash,
		})
	}

	report, err := skills.Scan(stagingRoot)
	if err != nil {
		return nil, fmt.Errorf("nuc: validate carried skills: %w", err)
	}
	if issues := report.Issues(); len(issues) > 0 {
		reasons := make([]string, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, issue.Error())
		}
		return nil, fmt.Errorf("nuc: carried skills are invalid: %s", strings.Join(reasons, "; "))
	}
	return carried, nil
}

// importCarriedSkills installs each carried skill directory. A skill is one
// unit: an existing entry is skipped without force and replaced whole with
// force, never merged file by file. The planned bytes were validated before
// any write; the installed procedure hash is checked after the write.
func importCarriedSkills(colonyRoot string, carried []carriedSkill, opts ImportOptions, res *ImportResult) error {
	if len(carried) == 0 {
		return nil
	}
	storeDir := skills.Dir(colonyRoot)
	for _, plan := range carried {
		target := filepath.Join(storeDir, plan.name)
		exists, err := pathExists(target)
		if err != nil {
			return fmt.Errorf("nuc: skill %q: %w", plan.name, err)
		}
		if exists && !opts.Force {
			res.Skipped = append(res.Skipped, target)
			continue
		}
		action := actionCreated
		if exists {
			action = actionOverwritten
		}

		rels := make([]string, 0, len(plan.files))
		for rel := range plan.files {
			rels = append(rels, rel)
		}
		sort.Strings(rels)

		if opts.DryRun {
			for _, rel := range rels {
				dest, err := joinUnder(target, rel)
				if err != nil {
					return fmt.Errorf("nuc: skill %q: %w", plan.name, err)
				}
				appendAction(res, action, dest)
			}
			continue
		}

		if exists {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("nuc: replace skill %q: %w", plan.name, err)
			}
		}
		written := make([]string, 0, len(rels))
		for _, rel := range rels {
			dest, err := joinUnder(target, rel)
			if err != nil {
				return fmt.Errorf("nuc: skill %q: %w", plan.name, err)
			}
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return fmt.Errorf("nuc: install skill %q: %w", plan.name, err)
			}
			if err := os.WriteFile(dest, plan.files[rel], 0o644); err != nil {
				return fmt.Errorf("nuc: install skill %q: %w", plan.name, err)
			}
			written = append(written, dest)
		}

		installed, err := skills.ProcedureHash(target)
		if err != nil {
			return fmt.Errorf("nuc: verify skill %q: %w", plan.name, err)
		}
		if installed != plan.hash {
			return fmt.Errorf(
				"nuc: installed skill %q hashes to %s, not the validated %s", plan.name, installed, plan.hash,
			)
		}
		for _, dest := range written {
			appendAction(res, action, dest)
		}
	}
	return nil
}

// joinUnder resolves a slash-form relative path under root and refuses a result
// that leaves it, even though the carried path was validated earlier.
func joinUnder(root, rel string) (string, error) {
	dest := filepath.Join(root, filepath.FromSlash(rel))
	if dest == root || !strings.HasPrefix(dest, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes %s", rel, root)
	}
	return dest, nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func writeColonyFile(path string, data []byte, force, dryRun bool) (fileAction, error) {
	_, err := os.Stat(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if exists && !force {
		return actionSkipped, nil
	}

	if dryRun {
		if exists {
			return actionOverwritten, nil
		}
		return actionCreated, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	if exists {
		return actionOverwritten, nil
	}
	return actionCreated, nil
}

func appendAction(res *ImportResult, action fileAction, path string) {
	switch action {
	case actionCreated:
		res.Created = append(res.Created, path)
	case actionSkipped:
		res.Skipped = append(res.Skipped, path)
	case actionOverwritten:
		res.Overwritten = append(res.Overwritten, path)
	}
}

// FormatImportSummary returns a human-readable import summary.
func FormatImportSummary(res ImportResult, verbose bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "created: %d, skipped: %d, overwritten: %d\n", len(res.Created), len(res.Skipped), len(res.Overwritten))
	if verbose {
		writePaths(&b, "created", res.Created)
		writePaths(&b, "skipped", res.Skipped)
		writePaths(&b, "overwritten", res.Overwritten)
	}
	return b.String()
}

func writePaths(b *strings.Builder, label string, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, p := range paths {
		fmt.Fprintf(b, "  %s\n", p)
	}
}
