package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
)

// diagnoseSkillSelection reports the per-role gate's verdict: an unresolvable
// reference or pattern, an unmatched non-`*` pattern, and the 32-skill breach
// are errors; an explicit empty list is a warning; a disabled skill is skipped
// and named with the roles that still list it. Findings are the same verdict
// `skills validate` and a dispatch would produce, because they come from the
// same scan and the same resolver.
func diagnoseSkillSelection(
	ctx colony.Context,
	bees map[string]colony.Bee,
	store skills.Report,
	ledger skills.Ledger,
	report *bus.DoctorReport,
) {
	// An invalid skill is already reported by the store scan, which doctor adds
	// as an error. Resolution repeats it against the role that matched it; that
	// repetition would say the same thing twice.
	storeIssues := map[string]bool{}
	for _, issue := range store.Issues() {
		storeIssues[issue.Error()] = true
	}

	disabledRoles := map[string][]string{}
	disabledReason := map[string]string{}
	for _, role := range sortedBeeRoleNames(bees) {
		effective, err := ctx.EffectiveSkills(role, bees[role])
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("bee %q: %v", role, err))
			continue
		}
		if !effective.Present {
			continue
		}
		if effective.ExplicitEmpty() {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"bee %q declares an explicit empty skill list (opts out of every skill)", role,
			))
		}
		resolution := skills.Resolve(store, ledger, effective.Refs)
		for _, issue := range resolution.Issues {
			if issue.Skill != "" {
				continue // the store scan already reports this skill
			}
			if storeIssues[issue.Error()] {
				continue // a store-wide finding the scan already reports
			}
			report.Errors = append(report.Errors, fmt.Sprintf("bee %q: %s", role, issue.Error()))
		}
		for _, disabled := range resolution.Disabled {
			disabledRoles[disabled.Name] = append(disabledRoles[disabled.Name], role)
			disabledReason[disabled.Name] = disabled.Reason
		}
	}

	names := make([]string, 0, len(disabledRoles))
	for name := range disabledRoles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		roles := disabledRoles[name]
		sort.Strings(roles)
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"skill %q is disabled (%s); skipped for bee(s): %s",
			name, disabledReason[name], strings.Join(roles, ", "),
		))
	}
}

func sortedBeeRoleNames(bees map[string]colony.Bee) []string {
	roles := make([]string, 0, len(bees))
	for role := range bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
