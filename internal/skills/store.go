// Package skills owns the colony's Agent Skills store.
//
// The store is one root in the repository, `.paseka/skills/`: every immediate
// directory is one skill, named for that directory, holding a spec-shaped
// `SKILL.md` with YAML frontmatter and a Markdown body, plus optional
// `scripts/`, `references/`, and `assets/` content. There is one rule set for
// what a valid skill is (validate.go) and one reference syntax for naming one
// (reference.go). Every later path — dispatch, a trail's frozen copy, approval —
// reuses this scan instead of re-deriving the rules, because a second copy of
// these rules would drift and two call sites would then disagree about what a
// bee was offered.
package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
)

const (
	// DirName is the store's directory under `.paseka/`.
	DirName = "skills"
	// ManifestName is the file every skill directory holds.
	ManifestName = "SKILL.md"
	// LedgerName is the committed enabled/disabled state file, which sits beside
	// the store rather than inside it so that a directory under the store is
	// always a skill and never a bookkeeping file a client might load.
	LedgerName = "skills.index.json"

	// MaxNameLength is the standard's bound on a skill name.
	MaxNameLength = 64
	// MaxDescriptionLength is the standard's bound on a description, in
	// characters rather than bytes.
	MaxDescriptionLength = 1024
	// MaxSkillFileBytes bounds one file inside a skill directory — the
	// `SKILL.md` or a supporting file.
	MaxSkillFileBytes = 256 << 10 // 256 KiB
	// MaxStoreBytes bounds the whole store, with the ledger excluded. The reason
	// is context, not disk: an advertised skill costs tokens in every run.
	MaxStoreBytes = 8 << 20 // 8 MiB
)

// Dir returns the repository store for a colony root.
func Dir(colonyRoot string) string {
	return colony.PasekaPath(colonyRoot, DirName)
}

// LedgerPath returns the committed state file beside the store.
func LedgerPath(colonyRoot string) string {
	return colony.PasekaPath(colonyRoot, LedgerName)
}

// Issue is one validation finding. Skill names the store entry it belongs to;
// an empty Skill means the finding is about the store itself (a budget, or an
// entry that is not a skill directory).
type Issue struct {
	Skill  string
	Reason string
}

// Error renders a finding the same way everywhere it is reported, so `doctor`
// and `skills validate` cannot describe the same problem differently. A
// store-wide reason reads as a sentence on its own, because it has no skill
// name to be prefixed to.
func (i Issue) Error() string {
	if i.Skill == "" {
		return i.Reason
	}
	return fmt.Sprintf("skill %q: %s", i.Skill, i.Reason)
}

// Skill is one immediate directory in the store.
type Skill struct {
	// Name is the directory name, which is the skill's identity in the store.
	Name string
	// DeclaredName is the `name` from the frontmatter, empty when absent. It
	// must equal Name; it is kept separate so a mismatch can be reported.
	DeclaredName string
	Description  string
	Metadata     map[string]string
	Body         string
	// Dir and Path are absolute so a later delivery path can advertise exactly
	// the file it resolved.
	Dir  string
	Path string
	// Bytes is the size of the `SKILL.md`; the store budget counts the whole
	// directory, which Scan reports separately.
	Bytes  int64
	Issues []Issue
}

// Valid reports whether this skill passed the rule set.
func (s Skill) Valid() bool {
	return len(s.Issues) == 0
}

// Report is one scan of one store root.
type Report struct {
	Root   string
	Skills []Skill
	Bytes  int64
	store  []Issue // findings about the store rather than one skill
}

// ScanColony reads the live repository store for a colony root.
func ScanColony(colonyRoot string) (Report, error) {
	return Scan(Dir(colonyRoot))
}

// Scan reads the skill store rooted at dir. A missing store is an empty store:
// a colony that has not written a skill is valid, not broken.
//
// The same scan is given the repository store or a trail's frozen copy, so the
// only thing that differs between the two is which bytes it reads.
func Scan(dir string) (Report, error) {
	report := Report{Root: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return report, fmt.Errorf("skills: read store %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.Name() == LedgerName {
			// The ledger is not part of the store. It normally lives beside the
			// store root; skipping it here keeps the budget honest if a copy is
			// ever placed inside.
			continue
		}
		isDir := entry.IsDir()
		if !isDir && entry.Type()&fs.ModeSymlink != 0 {
			// A skill directory may be linked into the store; the trail capture
			// follows such links, so the store scan does too rather than calling
			// a linked skill invalid.
			if info, statErr := os.Stat(filepath.Join(dir, entry.Name())); statErr == nil && info.IsDir() {
				isDir = true
			}
		}
		if !isDir {
			report.store = append(report.store, Issue{Reason: fmt.Sprintf(
				"entry %q is not a directory; every immediate directory under %s is one skill", entry.Name(), dir,
			)})
			continue
		}
		skill, dirBytes := scanSkill(filepath.Join(dir, entry.Name()), entry.Name())
		report.Skills = append(report.Skills, skill)
		report.Bytes += dirBytes
	}

	sort.Slice(report.Skills, func(i, j int) bool { return report.Skills[i].Name < report.Skills[j].Name })
	if report.Bytes > MaxStoreBytes {
		report.store = append(report.store, Issue{Reason: fmt.Sprintf(
			"store total is %d bytes; the limit is %d bytes (8 MiB)", report.Bytes, MaxStoreBytes,
		)})
	}
	return report, nil
}

// Issues returns every finding of the scan — store-wide and per skill — in a
// stable order, with store-wide findings first.
func (r Report) Issues() []Issue {
	issues := make([]Issue, 0, len(r.store)+len(r.Skills))
	issues = append(issues, r.store...)
	for _, skill := range r.Skills {
		issues = append(issues, skill.Issues...)
	}
	sortIssues(issues)
	return issues
}

// Valid reports whether the whole store passed the rule set.
func (r Report) Valid() bool {
	return len(r.Issues()) == 0
}

// Find returns the verdict for one reference: the store-wide findings plus the
// findings of every skill it names. A non-wildcard reference that matches
// nothing is itself a finding, because a role or a command that names a skill
// that is not there must not silently receive nothing.
func (r Report) Find(ref Reference) []Issue {
	issues := make([]Issue, 0, len(r.store))
	issues = append(issues, r.store...)
	matched := r.matching(ref)
	if len(matched) == 0 && !ref.IsWildcard() {
		issues = append(issues, Issue{Reason: fmt.Sprintf(
			"reference %q matches no skill in %s", ref, r.Root,
		)})
	}
	for _, skill := range matched {
		issues = append(issues, skill.Issues...)
	}
	sortIssues(issues)
	return issues
}

// Select returns the skills a reference names, sorted by name. A non-wildcard
// reference that matches nothing is an error rather than an empty result, so a
// resolver that uses this cannot drop a declared entry.
func (r Report) Select(ref Reference) ([]Skill, error) {
	matched := r.matching(ref)
	if len(matched) == 0 && !ref.IsWildcard() {
		return nil, fmt.Errorf("skills: reference %q matches no skill in %s", ref, r.Root)
	}
	return matched, nil
}

// matching returns every skill a reference names. The rule lives on Reference
// so a listing, a resolution, and a command answer the same question the same
// way.
func (r Report) matching(ref Reference) []Skill {
	var out []Skill
	for _, skill := range r.Skills {
		if ref.Names(skill) {
			out = append(out, skill)
		}
	}
	return out
}

// scanSkill reads one skill directory and returns it with the directory's total
// size. I/O trouble is a finding, not a scan error: one unreadable skill must
// not stop the store from being described.
func scanSkill(dir, name string) (Skill, int64) {
	skill := Skill{Name: name, Dir: dir, Path: filepath.Join(dir, ManifestName)}
	var dirBytes int64
	fileIssues, bytes := scanFiles(dir, name)
	dirBytes += bytes
	skill.Issues = append(skill.Issues, fileIssues...)

	data, err := os.ReadFile(skill.Path)
	if err != nil {
		reason := fmt.Sprintf("%s is missing", ManifestName)
		if !os.IsNotExist(err) {
			reason = fmt.Sprintf("read %s: %v", ManifestName, err)
		}
		skill.Issues = append(skill.Issues, Issue{Skill: name, Reason: reason})
		return skill, dirBytes
	}
	skill.Bytes = int64(len(data))

	doc, body, reason := parseDocument(data)
	skill.Body = body
	if reason != "" {
		skill.Issues = append(skill.Issues, Issue{Skill: name, Reason: reason})
		return skill, dirBytes
	}
	skill.DeclaredName = doc.name
	skill.Description = strings.TrimSpace(doc.description)
	skill.Metadata = doc.metadata
	skill.Issues = append(skill.Issues, validateDocument(name, doc)...)
	return skill, dirBytes
}

// scanFiles enforces the fixed budgets over one skill directory: every file is
// counted towards the store bound, and one file over its own bound is a
// finding. The walk covers the whole directory, so a skill cannot hide a large
// procedure in a nested folder, and it follows links the way the trail capture
// does — with a cycle guard, because a link back up the tree must not hang a
// scan.
func scanFiles(dir, name string) ([]Issue, int64) {
	var issues []Issue
	var total int64
	seen := map[string]bool{}
	var walk func(path, rel string)
	walk = func(path, rel string) {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			issues = append(issues, Issue{Skill: name, Reason: fmt.Sprintf("read %s: %v", displayRel(rel), err)})
			return
		}
		if seen[resolved] {
			return
		}
		seen[resolved] = true

		entries, err := os.ReadDir(path)
		if err != nil {
			issues = append(issues, Issue{Skill: name, Reason: fmt.Sprintf("read %s: %v", displayRel(rel), err)})
			return
		}
		for _, entry := range entries {
			childPath := filepath.Join(path, entry.Name())
			childRel := entry.Name()
			if rel != "" {
				childRel = rel + "/" + entry.Name()
			}
			info, err := entry.Info()
			if err != nil {
				issues = append(issues, Issue{Skill: name, Reason: fmt.Sprintf("read %s: %v", childRel, err)})
				continue
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				if info, err = os.Stat(childPath); err != nil {
					issues = append(issues, Issue{Skill: name, Reason: fmt.Sprintf("read %s: %v", childRel, err)})
					continue
				}
			}
			if info.IsDir() {
				walk(childPath, childRel)
				continue
			}
			if !info.Mode().IsRegular() {
				continue
			}
			total += info.Size()
			if info.Size() > MaxSkillFileBytes {
				issues = append(issues, Issue{Skill: name, Reason: fmt.Sprintf(
					"file %q is %d bytes; a skill file may not exceed %d bytes (256 KiB)",
					childRel, info.Size(), MaxSkillFileBytes,
				)})
			}
		}
	}
	walk(dir, "")
	return issues, total
}

func displayRel(rel string) string {
	if rel == "" {
		return "."
	}
	return rel
}

func sortIssues(issues []Issue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Skill != issues[j].Skill {
			return issues[i].Skill < issues[j].Skill
		}
		return issues[i].Reason < issues[j].Reason
	})
}
