package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Sector is a named workspace scope inside a colony (module/subfolder).
type Sector struct {
	Path string `yaml:"path"`
}

// ResolveSector returns the configured sector by name.
func (c Colony) ResolveSector(name string) (Sector, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Sector{}, nil
	}
	if c.Sectors == nil {
		return Sector{}, fmt.Errorf("colony: unknown sector %q (no sectors configured)", name)
	}
	sector, ok := c.Sectors[name]
	if !ok {
		return Sector{}, fmt.Errorf("colony: unknown sector %q", name)
	}
	return sector, nil
}

// SectorRelPath returns the relative path for a sector name, or empty for root.
func (c Colony) SectorRelPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", nil
	}
	sector, err := c.ResolveSector(name)
	if err != nil {
		return "", err
	}
	rel, err := normalizeSectorPath(sector.Path)
	if err != nil {
		return "", fmt.Errorf("colony: sector %q: %w", name, err)
	}
	return rel, nil
}

// SectorAbsPath returns the absolute path for a sector under colonyRoot.
func (c Colony) SectorAbsPath(colonyRoot, name string) (string, error) {
	colonyRoot, err := filepath.Abs(colonyRoot)
	if err != nil {
		return "", err
	}
	rel, err := c.SectorRelPath(name)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return colonyRoot, nil
	}
	abs := filepath.Join(colonyRoot, rel)
	if err := ensureWithinColony(colonyRoot, abs); err != nil {
		return "", fmt.Errorf("colony: sector %q: %w", name, err)
	}
	return abs, nil
}

// JoinSectorPath appends an optional sector relative path to a base workspace.
func JoinSectorPath(base, rel string) (string, error) {
	rel, err := normalizeSectorPath(rel)
	if err != nil {
		return "", err
	}
	if rel == "" {
		return base, nil
	}
	abs, err := filepath.Abs(filepath.Join(base, rel))
	if err != nil {
		return "", err
	}
	if err := ensureWithinColony(base, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// EnsureSectorDirExists verifies that the sector directory exists on disk.
func EnsureSectorDirExists(absPath string) error {
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("sector path %q does not exist", absPath)
		}
		return fmt.Errorf("sector path %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("sector path %q is not a directory", absPath)
	}
	return nil
}

// EffectiveSector returns task sector when set, otherwise bee default sector.
func EffectiveSector(taskSector, beeSector string) string {
	if s := strings.TrimSpace(taskSector); s != "" {
		return s
	}
	return strings.TrimSpace(beeSector)
}

func normalizeSectorPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "." {
		return "", nil
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes colony root")
	}
	return clean, nil
}

func ensureWithinColony(colonyRoot, target string) error {
	colonyRoot, err := filepath.Abs(colonyRoot)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(colonyRoot, target)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes colony root")
	}
	return nil
}
