package colony

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

// ComputeSlug derives a stable project slug from remote URL or directory name.
func ComputeSlug(repoRoot, originURL string) string {
	if s := slugFromRemote(originURL); s != "" {
		return s
	}
	base := filepath.Base(repoRoot)
	return sanitizeSlug(base)
}

// ResolveSlug returns slug from colony.yaml if set, otherwise computes a new one.
func ResolveSlug(repoRoot string, manifest Colony, originURL string) string {
	if s := strings.TrimSpace(manifest.Slug); s != "" {
		return s
	}
	return ComputeSlug(repoRoot, originURL)
}

// UniqueHomeSlug avoids collisions when two repos map to the same slug.
func UniqueHomeSlug(baseSlug, repoRoot, homeBase string) (string, error) {
	candidate := baseSlug
	for i := 0; i < 2; i++ {
		dir := filepath.Join(homeBase, candidate)
		cfgPath := filepath.Join(dir, "config.yaml")
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			return candidate, nil
		}
		existingRoot, err := readColonyRootFromHome(cfgPath)
		if err != nil {
			return "", err
		}
		absRepo, err := filepath.Abs(repoRoot)
		if err != nil {
			return "", err
		}
		if existingRoot == absRepo {
			return candidate, nil
		}
		if i == 0 {
			candidate = baseSlug + "-" + shortPathHash(absRepo)
		}
	}
	return candidate, nil
}

func slugFromRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, ":") && !strings.Contains(raw, "://") {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 {
			raw = "https://" + parts[0] + "/" + parts[1]
		}
	}
	raw = strings.TrimSuffix(raw, ".git")
	u, err := url.Parse(raw)
	if err != nil {
		return sanitizeSlug(filepath.Base(raw))
	}
	path := strings.Trim(u.Path, "/")
	if path == "" {
		return ""
	}
	segments := strings.Split(path, "/")
	if len(segments) >= 2 {
		return sanitizeSlug(segments[len(segments)-2] + "-" + segments[len(segments)-1])
	}
	return sanitizeSlug(segments[len(segments)-1])
}

func sanitizeSlug(s string) string {
	s = strings.ToLower(s)
	s = slugSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "colony"
	}
	return s
}

func shortPathHash(absPath string) string {
	sum := sha256.Sum256([]byte(absPath))
	return hex.EncodeToString(sum[:])[:8]
}

func readColonyRootFromHome(cfgPath string) (string, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", err
	}
	var cfg HomeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("colony: parse home config: %w", err)
	}
	if cfg.ColonyRoot == "" {
		return "", fmt.Errorf("colony: home config missing colony_root")
	}
	return filepath.Abs(cfg.ColonyRoot)
}
