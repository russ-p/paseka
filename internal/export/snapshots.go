package export

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/cues"
)

// NamedYAML is a committed config file snapshot for export.
type NamedYAML struct {
	Name    string
	Path    string
	Content string
	Missing bool
}

func loadBeeYAML(colonyRoot string, roles []string) ([]NamedYAML, error) {
	seen := make(map[string]struct{})
	var names []string
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" || !validBeeRole(role) {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		names = append(names, role)
	}
	sort.Strings(names)

	var out []NamedYAML
	for _, role := range names {
		rel := filepath.Join(".paseka", "bees", role+".yaml")
		path := filepath.Join(colonyRoot, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		out = append(out, NamedYAML{
			Name:    role,
			Path:    rel,
			Content: string(data),
		})
	}
	return out, nil
}

func validBeeRole(role string) bool {
	return !strings.Contains(role, "/") && !strings.Contains(role, "..") &&
		!strings.Contains(role, `\`)
}

func loadColonyYAML(colonyRoot string) (*NamedYAML, error) {
	rel := filepath.Join(".paseka", "colony.yaml")
	path := filepath.Join(colonyRoot, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &NamedYAML{
				Name:    "colony.yaml",
				Path:    rel,
				Missing: true,
			}, nil
		}
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	return &NamedYAML{
		Name:    "colony.yaml",
		Path:    rel,
		Content: string(data),
	}, nil
}

func loadCueYAML(colonyRoot string) ([]NamedYAML, error) {
	summaries, err := cues.List(colonyRoot)
	if err != nil {
		return nil, err
	}
	out := make([]NamedYAML, 0, len(summaries))
	for _, summary := range summaries {
		rel := filepath.Join(".paseka", "cues", summary.ID+".yaml")
		path := filepath.Join(colonyRoot, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", rel, err)
		}
		out = append(out, NamedYAML{
			Name:    summary.ID,
			Path:    rel,
			Content: string(data),
		})
	}
	return out, nil
}
