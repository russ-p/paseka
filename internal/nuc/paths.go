package nuc

import (
	"fmt"
	"path/filepath"
	"strings"
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
