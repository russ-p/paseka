package trailconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
)

// candidateSkillReport carries the non-fatal findings of candidate skill
// validation. Only a disabled skill lands here: the disable is an explicit
// operator decision, so a role that still lists the skill is reported rather
// than turning the candidate into a refusal.
type candidateSkillReport struct {
	Warnings []string
}

// validateCandidateSkills validates the candidate's skill store and every
// role's resolved selection against the one rule set and the one resolver the
// runtime already uses.
//
// The store scan is the same call `paseka skills validate` and `doctor` make,
// and every role is resolved through skills.ResolveRun, the function dispatch
// and sessions use. A second copy of the rules would drift, so this function
// only decides what to do with a finding: the store and the roles are checked
// before anything is recorded, and a disabled skill is skipped and reported
// rather than treated as an error.
func validateCandidateSkills(root string, bees map[string]colony.Bee, profile string) (candidateSkillReport, error) {
	var report candidateSkillReport

	store, err := skills.Scan(skills.Dir(root))
	if err != nil {
		return report, fmt.Errorf("trailconfig: candidate skill store is unreadable: %w", err)
	}
	if issues := store.Issues(); len(issues) > 0 {
		return report, fmt.Errorf("trailconfig: candidate skill store is invalid: %s", formatSkillIssues(issues))
	}
	// The ledger is an input to resolution, so a corrupt one is refused even
	// when no role selects a skill. ResolveRun loads it again per role; doing it
	// here keeps the verdict independent of the role list.
	if _, err := skills.LoadLedgerFile(skills.LedgerPath(root)); err != nil {
		return report, fmt.Errorf("trailconfig: candidate %w", err)
	}
	if err := validateCapturedClientSkills(root); err != nil {
		return report, err
	}

	profile = strings.TrimSpace(profile)
	if profile == "" {
		profile = inferCandidateProfile(root)
	}

	disabledRoles := map[string][]string{}
	disabledReason := map[string]string{}
	for _, role := range sortedBeeRoleNames(bees) {
		resolved, err := skills.ResolveRun(skills.RunInput{
			ConfigRoot: root,
			Frozen:     true,
			Role:       role,
			Profile:    profile,
		})
		if err != nil {
			return report, fmt.Errorf("trailconfig: candidate %w", err)
		}
		for _, disabled := range resolved.Disabled {
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
	return report, nil
}

// formatSkillIssues renders findings exactly as the store reports them, so a
// candidate refusal names the skill and reason in the same words the shell
// prints.
func formatSkillIssues(issues []skills.Issue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "; ")
}

// validateCapturedClientSkills keeps the long-standing check on the captured
// client directories. These are not the colony store and this layer neither
// gates nor advertises them, but a trail captures their bytes for
// reproducibility, so a directory without a manifest is still a broken input.
func validateCapturedClientSkills(root string) error {
	roots := []string{
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
			skill := filepath.Join(skillsRoot, entry.Name(), skills.ManifestName)
			info, err := os.Stat(skill)
			if err != nil {
				return fmt.Errorf("trailconfig: skill %q is missing %s", entry.Name(), skills.ManifestName)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("trailconfig: skill %q has a non-regular %s", entry.Name(), skills.ManifestName)
			}
		}
	}
	return nil
}

// inferCandidateProfile recovers the selected profile from a captured tree when
// the caller cannot supply it. A candidate tree carries only the selected
// profile's files: the colony recipe and/or the home profile config. An
// ambiguous tree (several distinct names) yields no inference rather than
// guessing, because validating the wrong overlay would be worse than validating
// the committed selection alone.
func inferCandidateProfile(root string) string {
	names := map[string]struct{}{}
	colonyProfiles, err := os.ReadDir(filepath.Join(root, ".paseka", "profiles"))
	if err == nil {
		for _, entry := range colonyProfiles {
			if entry.IsDir() {
				continue
			}
			switch ext := filepath.Ext(entry.Name()); ext {
			case ".yaml", ".yml":
				name := strings.TrimSuffix(entry.Name(), ext)
				if name != "" {
					names[name] = struct{}{}
				}
			}
		}
	}
	homeProfiles, err := os.ReadDir(filepath.Join(root, "home", "profiles"))
	if err == nil {
		for _, entry := range homeProfiles {
			if entry.IsDir() && entry.Name() != "" {
				names[entry.Name()] = struct{}{}
			}
		}
	}
	if len(names) != 1 {
		return ""
	}
	for name := range names {
		return name
	}
	return ""
}

func sortedBeeRoleNames(bees map[string]colony.Bee) []string {
	roles := make([]string, 0, len(bees))
	for role := range bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
