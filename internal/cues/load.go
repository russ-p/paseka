package cues

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

type rawCue struct {
	Description     string            `yaml:"description"`
	Emit            string            `yaml:"emit"`
	EnergyBudget    int               `yaml:"energy_budget"`
	Type            string            `yaml:"type"`
	Kind            string            `yaml:"kind"`
	Static          map[string]string `yaml:"static"`
	Title           string            `yaml:"title"`
	Body            string            `yaml:"body"`
	PayloadTemplate string            `yaml:"payload_template"`
	Bee             string            `yaml:"bee"`
	Intent          string            `yaml:"intent"`
	Review          string            `yaml:"review"`
	Autorun         bool              `yaml:"autorun"`
}

// Dir returns the colony cues directory path.
func Dir(colonyRoot string) string {
	return colony.PasekaPath(colonyRoot, "cues")
}

// Load reads and validates .paseka/cues/<id>.yaml.
func Load(colonyRoot, id string) (Cue, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Cue{}, fmt.Errorf("cue id is required")
	}
	if err := validateCueID(id); err != nil {
		return Cue{}, fmt.Errorf("cue %q: %w", id, err)
	}

	path := filepath.Join(Dir(colonyRoot), id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Cue{}, fmt.Errorf("cue %q: not found at %s", id, relCuePath(colonyRoot, path))
		}
		return Cue{}, fmt.Errorf("cue %q: read %s: %w", id, relCuePath(colonyRoot, path), err)
	}

	raw := rawCue{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Cue{}, fmt.Errorf("cue %q: invalid yaml: %w", id, err)
	}
	return validateCue(id, raw)
}

// List returns all valid cues in stable id order.
func List(colonyRoot string) ([]Summary, error) {
	dir := Dir(colonyRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("cue list: read %s: %w", dir, err)
	}

	var ids []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".yaml")
		if err := validateCueID(id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)

	out := make([]Summary, 0, len(ids))
	for _, id := range ids {
		cue, err := Load(colonyRoot, id)
		if err != nil {
			continue
		}
		out = append(out, Summary{ID: cue.ID, Description: cue.Description})
	}
	return out, nil
}

func validateCueID(id string) error {
	if id == "" {
		return fmt.Errorf("empty cue id")
	}
	if strings.Contains(id, "/") || strings.Contains(id, "\\") || strings.Contains(id, "..") {
		return fmt.Errorf("invalid cue id %q", id)
	}
	return nil
}

func validateCue(id string, raw rawCue) (Cue, error) {
	emit := EmitKind(strings.TrimSpace(raw.Emit))
	switch emit {
	case EmitSignal, EmitTask:
	default:
		return Cue{}, fmt.Errorf("cue %q: emit must be signal or task", id)
	}
	if raw.EnergyBudget < 0 {
		return Cue{}, fmt.Errorf("cue %q: energy_budget must be a positive integer", id)
	}

	cue := Cue{
		ID:              id,
		Description:     strings.TrimSpace(raw.Description),
		Emit:            emit,
		EnergyBudget:    raw.EnergyBudget,
		Static:          copyStringMap(raw.Static),
		TitleTemplate:   strings.TrimSpace(raw.Title),
		BodyTemplate:    strings.TrimSpace(raw.Body),
		PayloadTemplate: strings.TrimSpace(raw.PayloadTemplate),
		Bee:             strings.TrimSpace(raw.Bee),
		Intent:          strings.TrimSpace(raw.Intent),
		Review:          strings.TrimSpace(raw.Review),
		Autorun:         raw.Autorun,
	}

	switch emit {
	case EmitSignal:
		typ := strings.ToUpper(strings.TrimSpace(raw.Type))
		if typ == "" {
			return Cue{}, fmt.Errorf("cue %q: type is required for emit signal", id)
		}
		if typ != "SIGNAL" {
			return Cue{}, fmt.Errorf("cue %q: type must be SIGNAL (got %q)", id, raw.Type)
		}
		kind := strings.TrimSpace(raw.Kind)
		if kind == "" {
			return Cue{}, fmt.Errorf("cue %q: kind is required for emit signal", id)
		}
		cue.SignalType = typ
		cue.SignalKind = kind
	case EmitTask:
		if strings.TrimSpace(raw.Bee) == "" {
			return Cue{}, fmt.Errorf("cue %q: bee is required for emit task", id)
		}
		if strings.TrimSpace(raw.Intent) == "" {
			return Cue{}, fmt.Errorf("cue %q: intent is required for emit task", id)
		}
	}

	return cue, nil
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func relCuePath(colonyRoot, path string) string {
	if rel, err := filepath.Rel(colonyRoot, path); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return path
}
