package nuc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

// ExportFromColony builds a nuc document from an initialized colony.
func ExportFromColony(opts ExportOptions) (Document, error) {
	if opts.ColonyRoot == "" {
		return Document{}, fmt.Errorf("nuc: colony root is required")
	}

	allBees, err := colony.LoadAllBees(opts.ColonyRoot)
	if err != nil {
		return Document{}, err
	}
	if len(allBees) == 0 {
		return Document{}, fmt.Errorf("nuc: no bees found under %s", colony.BeesDir(opts.ColonyRoot))
	}

	roles, err := selectExportRoles(allBees, opts.Bees)
	if err != nil {
		return Document{}, err
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		manifest, err := colony.LoadColony(opts.ColonyRoot)
		if err != nil {
			return Document{}, err
		}
		name = strings.TrimSpace(manifest.Slug)
	}
	if name == "" {
		name = "colony"
	}

	doc := Document{
		APIVersion: APIVersion,
		Kind:       Kind,
		Metadata: Metadata{
			Name:        name,
			Description: strings.TrimSpace(opts.Description),
		},
		Spec: Spec{
			Bees:    make(map[string]string, len(roles)),
			Prompts: make(map[string]string),
		},
	}

	promptPaths := make(map[string]struct{})
	for _, role := range roles {
		path := colony.BeeYAMLPath(opts.ColonyRoot, role)
		data, err := os.ReadFile(path)
		if err != nil {
			return Document{}, fmt.Errorf("nuc: read bee %q: %w", role, err)
		}
		doc.Spec.Bees[role] = string(data)

		bee := allBees[role]
		if t := strings.TrimSpace(bee.PromptTemplate); t != "" {
			promptPaths[t] = struct{}{}
		}
		if t := strings.TrimSpace(bee.SystemTemplate); t != "" {
			promptPaths[t] = struct{}{}
		}
	}

	if len(promptPaths) > 0 {
		if err := exportPrompts(opts.ColonyRoot, promptPaths, doc.Spec.Prompts); err != nil {
			return Document{}, err
		}
	}

	return doc, nil
}

func selectExportRoles(all map[string]colony.Bee, filter []string) ([]string, error) {
	if len(filter) == 0 {
		roles := make([]string, 0, len(all))
		for role := range all {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		return roles, nil
	}

	roles := make([]string, 0, len(filter))
	seen := make(map[string]struct{}, len(filter))
	for _, role := range filter {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		if _, ok := all[role]; !ok {
			return nil, fmt.Errorf("nuc: unknown bee role %q", role)
		}
		roles = append(roles, role)
	}
	if len(roles) == 0 {
		return nil, fmt.Errorf("nuc: --bees filter matched no roles")
	}
	sort.Strings(roles)
	return roles, nil
}

func exportPrompts(colonyRoot string, promptPaths map[string]struct{}, out map[string]string) error {
	promptsDir := colony.PasekaPath(colonyRoot, "prompts")
	for ref := range promptPaths {
		if err := validatePromptPath(ref); err != nil {
			return err
		}
		path := filepath.Join(promptsDir, filepath.FromSlash(ref))
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("nuc: read prompt %q: %w", ref, err)
		}
		out[ref] = string(data)
	}

	partialsDir := filepath.Join(promptsDir, "_partials")
	entries, err := os.ReadDir(partialsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("nuc: list partials: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		ref := "_partials/" + entry.Name()
		if _, ok := out[ref]; ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(partialsDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("nuc: read partial %q: %w", ref, err)
		}
		out[ref] = string(data)
	}
	return nil
}

// BeeTemplatePaths returns prompt_template and system_template paths from raw bee YAML.
func BeeTemplatePaths(body string) (prompt, system string, err error) {
	var fields struct {
		PromptTemplate string `yaml:"prompt_template"`
		SystemTemplate string `yaml:"system_template"`
	}
	if err := yaml.Unmarshal([]byte(body), &fields); err != nil {
		return "", "", fmt.Errorf("nuc: parse bee yaml: %w", err)
	}
	return strings.TrimSpace(fields.PromptTemplate), strings.TrimSpace(fields.SystemTemplate), nil
}
