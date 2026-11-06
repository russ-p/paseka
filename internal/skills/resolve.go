package skills

import (
	"fmt"
	"sort"
)

// MaxSkillsPerRole is the fixed per-role budget. It is context, not disk: an
// advertised skill costs tokens in every run, and a profile may reduce a role's
// set but cannot raise this bound.
const MaxSkillsPerRole = 32

// DisabledSkill is a reference the store matched but the ledger has
// neutralised. It is skipped and reported; it is not a configuration error.
type DisabledSkill struct {
	Name   string
	Reason string
}

// Resolution is the effective skill set for one role, plus everything that was
// skipped or refused on the way. Issues are errors: a non-empty list means the
// role must not run. Disabled is reported, not fatal.
type Resolution struct {
	Skills   []Skill
	Disabled []DisabledSkill
	Issues   []Issue
}

// Valid reports whether the resolution may run.
func (r Resolution) Valid() bool {
	return len(r.Issues) == 0
}

// Resolve is the one function that turns a role's references into a skill set.
// A reference may be a bare name, a `*` pattern, or `*`; the result is
// deduplicated and sorted by name. A missing name, an unmatched non-`*`
// pattern, an invalid skill, an invalid reference, and a 33rd skill are errors.
// A disabled skill is skipped and reported. `*` may match an empty store.
func Resolve(store Report, ledger Ledger, refs []string) Resolution {
	var resolution Resolution
	seenIssue := map[string]bool{}
	addIssue := func(issue Issue) {
		key := issue.Error()
		if seenIssue[key] {
			return
		}
		seenIssue[key] = true
		resolution.Issues = append(resolution.Issues, issue)
	}

	seenSkill := map[string]bool{}
	wildcard := false
	for _, raw := range refs {
		ref, err := ParseReference(raw)
		if err != nil {
			addIssue(Issue{Reason: err.Error()})
			continue
		}
		if ref.IsWildcard() {
			wildcard = true
		}
		matched := store.matching(ref)
		if len(matched) == 0 && !ref.IsWildcard() {
			addIssue(Issue{Reason: fmt.Sprintf("reference %q matches no skill in %s", ref, store.Root)})
			continue
		}
		for _, skill := range matched {
			if seenSkill[skill.Name] {
				continue
			}
			seenSkill[skill.Name] = true
			if record, disabled := ledger.Record(skill.Name); disabled {
				resolution.Disabled = append(resolution.Disabled, DisabledSkill{
					Name:   skill.Name,
					Reason: record.Reason,
				})
				continue
			}
			if !skill.Valid() {
				for _, issue := range skill.Issues {
					addIssue(issue)
				}
				continue
			}
			resolution.Skills = append(resolution.Skills, skill)
		}
	}

	// `*` declares the whole store as the role's gate, so a store-wide finding
	// (a stray entry, the store budget) is this role's failure. A bare
	// reference does not inherit an unrelated store finding.
	if wildcard {
		for _, issue := range store.store {
			addIssue(issue)
		}
	}

	sort.Slice(resolution.Skills, func(i, j int) bool {
		return resolution.Skills[i].Name < resolution.Skills[j].Name
	})
	sort.Slice(resolution.Disabled, func(i, j int) bool {
		return resolution.Disabled[i].Name < resolution.Disabled[j].Name
	})

	if len(resolution.Skills) > MaxSkillsPerRole {
		addIssue(Issue{Reason: fmt.Sprintf(
			"role resolves to %d skills; the limit is %d",
			len(resolution.Skills), MaxSkillsPerRole,
		)})
	}

	sortIssues(resolution.Issues)
	return resolution
}
