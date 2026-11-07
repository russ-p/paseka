package nuc

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/skills"
)

func validateBeeRole(role string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		return fmt.Errorf("nuc: bee role is required")
	}
	if strings.Contains(role, "/") || strings.Contains(role, "..") {
		return fmt.Errorf("nuc: invalid bee role %q", role)
	}
	if strings.HasSuffix(role, ".local") {
		return fmt.Errorf("nuc: local overlay roles are not allowed in a nuc")
	}
	return nil
}

func validateCueID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("nuc: cue id is required")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("nuc: invalid cue id %q", id)
	}
	return nil
}

// validateSkillName refuses a name that could not be one immediate directory
// under the store. The store's own reference syntax is the authority on the
// shape; a nuc carries concrete skills, so a pattern is refused here too.
func validateSkillName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("nuc: skill name is required")
	}
	ref, err := skills.ParseReference(name)
	if err != nil {
		return fmt.Errorf("nuc: invalid skill name: %w", err)
	}
	if ref.IsPattern() {
		return fmt.Errorf("nuc: skill name %q must not contain a wildcard", name)
	}
	return nil
}

// validateSkillFilePath refuses an absolute path or one that leaves the skill
// directory. Files are carried as slash-form paths relative to that directory.
func validateSkillFilePath(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("nuc: empty skill file path")
	}
	if filepath.IsAbs(ref) || strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`) {
		return fmt.Errorf("nuc: absolute skill file path not allowed: %q", ref)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(ref)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return fmt.Errorf("nuc: skill file path must stay under the skill directory: %q", ref)
	}
	return nil
}

func validatePromptPath(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("nuc: empty prompt path")
	}
	if filepath.IsAbs(ref) {
		return fmt.Errorf("nuc: absolute prompt path not allowed: %q", ref)
	}
	clean := filepath.ToSlash(filepath.Clean(ref))
	if clean == "." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return fmt.Errorf("nuc: prompt path must stay under prompts: %q", ref)
	}
	return nil
}
