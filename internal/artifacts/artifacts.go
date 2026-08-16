package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/paseka/paseka/internal/colony"
)

const (
	// BaselineFileName is persisted under the producing run directory.
	BaselineFileName = "artifacts-baseline.json"
	// MaxInlineExportBytes caps comb file bodies inlined in trace export.
	MaxInlineExportBytes = 512 * 1024
)

// Item describes one comb file for announcements, Console, or export.
type Item struct {
	Ref          string `json:"ref"`
	ArtifactKind string `json:"artifactKind"`
	Title        string `json:"title,omitempty"`
	Updated      int64  `json:"updated,omitempty"` // unix mtime seconds
	Producer     string `json:"producer,omitempty"`
	Announced    bool   `json:"announced,omitempty"`
	Content      string `json:"content,omitempty"`
	Omitted      string `json:"omitted,omitempty"` // reason when content skipped (export)
}

// Root returns the absolute trail comb directory.
func Root(colonyRoot, traceID string) string {
	return colony.PasekaPath(colonyRoot, "runs", traceID, "artifacts")
}

// DirForPrompt returns the absolute comb path for prompt injection.
func DirForPrompt(colonyRoot, traceID string) string {
	dir, err := EnsureDir(colonyRoot, traceID)
	if err != nil {
		return Root(colonyRoot, traceID)
	}
	return dir
}

// EnsureDir creates the trail comb directory when missing.
func EnsureDir(colonyRoot, traceID string) (string, error) {
	root := Root(colonyRoot, traceID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("artifacts: mkdir %s: %w", root, err)
	}
	return root, nil
}

// ShouldSkipName reports whether a file basename should be excluded from scan.
func ShouldSkipName(name string) bool {
	if name == "" {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return strings.HasSuffix(name, "~") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".tmp")
}

// CanonicalRef returns a repo-relative ref for a file under the trail comb.
func CanonicalRef(colonyRoot, traceID, combRel string) (string, error) {
	abs, err := ResolveUnderComb(colonyRoot, traceID, combRel)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(colonyRoot, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// ResolveUnderComb resolves combRel (repo-relative comb path or short name) to an absolute path.
func ResolveUnderComb(colonyRoot, traceID, combRel string) (string, error) {
	combRel = strings.TrimSpace(combRel)
	if combRel == "" {
		return "", fmt.Errorf("artifacts: ref is required")
	}
	combRoot, err := filepath.Abs(Root(colonyRoot, traceID))
	if err != nil {
		return "", err
	}
	var abs string
	if filepath.IsAbs(combRel) {
		abs, err = filepath.Abs(combRel)
		if err != nil {
			return "", err
		}
	} else {
		norm := filepath.ToSlash(strings.TrimPrefix(combRel, "/"))
		prefix := ".paseka/runs/" + traceID + "/artifacts/"
		if strings.HasPrefix(norm, prefix) {
			rest := strings.TrimPrefix(norm, prefix)
			abs = filepath.Join(combRoot, filepath.FromSlash(rest))
		} else if strings.HasPrefix(norm, ".paseka/") {
			abs = filepath.Join(colonyRoot, filepath.FromSlash(norm))
		} else {
			abs = filepath.Join(combRoot, filepath.FromSlash(norm))
		}
		abs, err = filepath.Abs(abs)
		if err != nil {
			return "", err
		}
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("artifacts: file not found for ref %q", combRel)
		}
		return "", fmt.Errorf("artifacts: resolve ref %q: %w", combRel, err)
	}
	if pathEscapes(combRoot, resolved) {
		return "", fmt.Errorf("artifacts: ref %q escapes comb", combRel)
	}
	return resolved, nil
}

func pathEscapes(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// RefExists reports whether ref resolves to an existing file under the trail comb.
func RefExists(colonyRoot, traceID, ref string) bool {
	_, err := ResolveUnderComb(colonyRoot, traceID, ref)
	return err == nil
}

// HashMap maps repo-relative comb refs to SHA-256 hex digests.
type HashMap map[string]string

// Scan walks the trail comb and returns ref→hash for included regular files.
func Scan(colonyRoot, traceID string) (HashMap, error) {
	root := Root(colonyRoot, traceID)
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return HashMap{}, nil
		}
		return nil, fmt.Errorf("artifacts: stat comb: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("artifacts: comb path is not a directory")
	}
	out := HashMap{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && ShouldSkipName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if ShouldSkipName(d.Name()) || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		canonical, err := CanonicalRef(colonyRoot, traceID, rel)
		if err != nil {
			return err
		}
		hash, err := hashFile(path)
		if err != nil {
			return err
		}
		out[canonical] = hash
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ArtifactKindFromRef derives artifactKind from basename stem.
func ArtifactKindFromRef(ref string) string {
	base := filepath.Base(filepath.FromSlash(ref))
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// TitleFromMarkdown reads optional title from the first non-empty line of markdown bytes.
func TitleFromMarkdown(data []byte) string {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			line = strings.TrimLeft(line, "#")
			line = strings.TrimSpace(line)
		}
		if line != "" {
			return line
		}
	}
	return ""
}

// ItemFromFile builds an Item with heuristics from a comb file path.
func ItemFromFile(colonyRoot, traceID, combRel string) (Item, error) {
	abs, err := ResolveUnderComb(colonyRoot, traceID, combRel)
	if err != nil {
		return Item{}, err
	}
	canonical, err := CanonicalRef(colonyRoot, traceID, combRel)
	if err != nil {
		return Item{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Item{}, err
	}
	item := Item{
		Ref:          canonical,
		ArtifactKind: ArtifactKindFromRef(canonical),
		Updated:      info.ModTime().UTC().Unix(),
	}
	if strings.HasSuffix(strings.ToLower(abs), ".md") {
		data, err := os.ReadFile(abs)
		if err != nil {
			return Item{}, err
		}
		if title := TitleFromMarkdown(data); title != "" {
			item.Title = title
		}
	}
	return item, nil
}

// ListItems scans the comb and returns sorted items (no content).
func ListItems(colonyRoot, traceID string) ([]Item, error) {
	hashes, err := Scan(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(hashes))
	for ref := range hashes {
		combRel, err := combRelFromCanonical(colonyRoot, traceID, ref)
		if err != nil {
			return nil, err
		}
		item, err := ItemFromFile(colonyRoot, traceID, combRel)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sortItems(items)
	return items, nil
}

func combRelFromCanonical(colonyRoot, traceID, canonical string) (string, error) {
	prefix := ".paseka/runs/" + traceID + "/artifacts/"
	norm := filepath.ToSlash(canonical)
	if strings.HasPrefix(norm, prefix) {
		return strings.TrimPrefix(norm, prefix), nil
	}
	abs, err := ResolveUnderComb(colonyRoot, traceID, canonical)
	if err != nil {
		return "", err
	}
	root := Root(colonyRoot, traceID)
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func sortItems(items []Item) {
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].Ref < items[i].Ref {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
}

// ReadContent reads comb file bytes for preview/export.
func ReadContent(colonyRoot, traceID, ref string) ([]byte, error) {
	abs, err := ResolveUnderComb(colonyRoot, traceID, ref)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(abs)
}

// IsTextContent reports whether data is valid UTF-8 without NUL bytes.
func IsTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return !strings.Contains(string(data), "\x00") && utf8.Valid(data)
}

// Delta classifies scan results against a baseline.
type Delta struct {
	Added   []Item
	Changed []Item
}

// ComputeDelta returns added and changed items vs baseline.
func ComputeDelta(colonyRoot, traceID string, baseline HashMap) (Delta, error) {
	current, err := Scan(colonyRoot, traceID)
	if err != nil {
		return Delta{}, err
	}
	var out Delta
	for ref, hash := range current {
		prev, ok := baseline[ref]
		combRel, err := combRelFromCanonical(colonyRoot, traceID, ref)
		if err != nil {
			return Delta{}, err
		}
		item, err := ItemFromFile(colonyRoot, traceID, combRel)
		if err != nil {
			return Delta{}, err
		}
		switch {
		case !ok:
			out.Added = append(out.Added, item)
		case prev != hash:
			out.Changed = append(out.Changed, item)
		}
	}
	sortItems(out.Added)
	sortItems(out.Changed)
	return out, nil
}
