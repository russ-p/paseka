package trailconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"gopkg.in/yaml.v3"
)

// SkillTrialBundle is one validated proposal bundle to add to a candidate
// store. Name is the destination identity; Files are relative to the skill
// directory; ProcedureHash is the identity the written bytes must reproduce.
type SkillTrialBundle struct {
	Name          string
	Files         []SkillTrialFile
	ProcedureHash string
}

// SkillTrialFile is one file of a trialled skill.
type SkillTrialFile struct {
	Rel  string
	Data []byte
}

// SkillTrial is one trial destination: the bundles to add under the candidate
// store and the roles that must resolve them in the same candidate.
type SkillTrial struct {
	Bundles []SkillTrialBundle
	Roles   []string
}

// applySkillTrial builds the candidate's content changes for a trial: the
// bundles land under the candidate store and every named role resolves each
// skill. A role that already resolves a skill through a wildcard or a matching
// pattern is left unchanged, so its gate is not narrowed. A role that inherits
// a default list receives the resolved list plus the new skills as its own
// explicit list, which keeps the trial scoped to the roles the Beekeeper named.
//
// The function writes only inside the candidate tree. The live store, the live
// bees, and the live profiles are inputs to the active snapshot and are never
// touched.
func applySkillTrial(treeRoot, profile string, trial *SkillTrial) error {
	if trial == nil || len(trial.Bundles) == 0 {
		return fmt.Errorf("trailconfig: a trial needs at least one skill bundle")
	}
	roles := normalizeRoleList(trial.Roles)
	if len(roles) == 0 {
		return fmt.Errorf("trailconfig: a trial needs at least one role")
	}

	bees, err := colony.LoadAllBees(treeRoot)
	if err != nil {
		return fmt.Errorf("trailconfig: load candidate bees: %w", err)
	}
	for _, role := range roles {
		if _, ok := bees[role]; !ok {
			return fmt.Errorf("trailconfig: candidate has no bee %q", role)
		}
	}

	for _, bundle := range trial.Bundles {
		if err := writeTrialBundle(treeRoot, bundle); err != nil {
			return err
		}
	}

	manifest, err := colony.LoadColony(treeRoot)
	if err != nil {
		return fmt.Errorf("trailconfig: load candidate colony: %w", err)
	}
	colonyProfile, homeProfile, err := skills.FrozenProfileLayers(treeRoot, profile)
	if err != nil {
		return fmt.Errorf("trailconfig: read candidate profile: %w", err)
	}

	names := make([]string, 0, len(trial.Bundles))
	for _, bundle := range trial.Bundles {
		names = append(names, bundle.Name)
	}
	sort.Strings(names)

	for _, role := range roles {
		bee, _, err := colony.LoadBee(treeRoot, role)
		if err != nil {
			return fmt.Errorf("trailconfig: candidate bee %q is invalid: %w", role, err)
		}
		if err := ensureRoleSkills(treeRoot, profile, manifest, colonyProfile, homeProfile, role, bee, names); err != nil {
			return err
		}
	}

	// Re-read the roles so a malformed rewrite fails here, before the candidate
	// is inventoried and validated.
	if _, err := colony.LoadAllBees(treeRoot); err != nil {
		return fmt.Errorf("trailconfig: candidate role selection is invalid: %w", err)
	}
	return nil
}

// writeTrialBundle copies one validated bundle into the candidate store. An
// existing entry is refused rather than replaced: a trial adds a procedure, and
// --force is a store approval concept.
func writeTrialBundle(treeRoot string, bundle SkillTrialBundle) error {
	name := strings.TrimSpace(bundle.Name)
	ref, err := skills.ParseReference(name)
	if err != nil {
		return fmt.Errorf("trailconfig: trial skill name: %w", err)
	}
	if ref.IsPattern() {
		return fmt.Errorf("trailconfig: trial skill name %q is a pattern; a trial names one procedure", name)
	}
	if len(bundle.Files) == 0 {
		return fmt.Errorf("trailconfig: trial skill %q has no files", name)
	}

	storeRoot := skills.Dir(treeRoot)
	target := filepath.Join(storeRoot, name)
	if _, err := os.Lstat(target); err == nil {
		return fmt.Errorf("trailconfig: the candidate store already holds a skill named %q", name)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("trailconfig: inspect candidate store entry %s: %w", target, err)
	}

	for _, file := range bundle.Files {
		rel, err := cleanTrialRel(file.Rel)
		if err != nil {
			return fmt.Errorf("trailconfig: trial skill %q: %w", name, err)
		}
		path := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("trailconfig: create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, file.Data, 0o644); err != nil {
			return fmt.Errorf("trailconfig: write %s: %w", path, err)
		}
	}

	hash, err := skills.ProcedureHash(target)
	if err != nil {
		return fmt.Errorf("trailconfig: verify trial skill %q: %w", name, err)
	}
	if strings.TrimSpace(bundle.ProcedureHash) != "" && hash != strings.TrimSpace(bundle.ProcedureHash) {
		return fmt.Errorf(
			"trailconfig: trial skill %q hashes to %s, not %s; refusing to add it",
			name, hash, bundle.ProcedureHash,
		)
	}
	return nil
}

func cleanTrialRel(rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("bundle file has an empty path")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || filepath.IsAbs(clean) {
		return "", fmt.Errorf("bundle file %q escapes the skill directory", rel)
	}
	return clean, nil
}

// ensureRoleSkills adds every skill the role does not already resolve to the
// layer that actually supplies its current list. An inherited default becomes a
// role-specific list on the bee, so one trial cannot widen the default for
// every other role.
func ensureRoleSkills(
	treeRoot, profile string,
	manifest colony.Colony,
	colonyProfile colony.ColonyProfile,
	homeProfile colony.HomeProfileConfig,
	role string,
	bee colony.Bee,
	names []string,
) error {
	selection := colony.EffectiveSkillsFor(manifest.Defaults, colonyProfile, homeProfile, role, bee)
	missing := make([]string, 0, len(names))
	for _, name := range names {
		if selectionResolves(selection.Refs, name) {
			continue
		}
		missing = append(missing, name)
	}
	if len(missing) == 0 {
		return nil
	}
	refs := append(append([]string(nil), selection.Refs...), missing...)

	switch selection.Source {
	case "home profile":
		path, err := homeProfilePath(treeRoot, profile)
		if err != nil {
			return err
		}
		return setYAMLSequence(path, []string{"bees", role, "skills"}, refs)
	case "colony profile":
		if strings.TrimSpace(profile) == "" {
			return fmt.Errorf("trailconfig: role %q selected from a colony profile, but no profile is recorded", role)
		}
		path := colony.ColonyProfilePath(treeRoot, profile)
		return setYAMLSequence(path, []string{"bees", role, "skills"}, refs)
	case "bee", "home profile defaults", "colony profile defaults", "defaults", "none":
		// A role-specific list is the narrowest layer. Writing one here also
		// turns an inherited default into the resolved list the role actually
		// had, so the trial changes nothing for another role.
		path := filepath.Join(treeRoot, ".paseka", "bees", role+".yaml")
		return setYAMLSequence(path, []string{"skills"}, refs)
	default:
		return fmt.Errorf("trailconfig: role %q has an unknown skill selection source %q", role, selection.Source)
	}
}

func selectionResolves(refs []string, name string) bool {
	for _, raw := range refs {
		ref, err := skills.ParseReference(raw)
		if err != nil {
			continue
		}
		if ref.Matches(name) {
			return true
		}
	}
	return false
}

func homeProfilePath(treeRoot, profile string) (string, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return "", fmt.Errorf("trailconfig: a home profile role list needs a selected profile")
	}
	return filepath.Join(treeRoot, "home", "profiles", profile, "config.yaml"), nil
}

func normalizeRoleList(roles []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roles))
	for _, raw := range roles {
		role := strings.TrimSpace(raw)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}

// setYAMLSequence replaces one sequence value in a YAML document while leaving
// every other node, comment, and key intact.
func setYAMLSequence(path string, keys []string, values []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("trailconfig: read %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("trailconfig: inspect %s: %w", path, err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("trailconfig: parse %s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("trailconfig: %s is not a YAML mapping", path)
	}

	node := root
	for i, key := range keys {
		if i == len(keys)-1 {
			setYAMLValue(node, key, yamlSequence(values))
			break
		}
		child := yamlMappingValue(node, key)
		if child == nil {
			child = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			node.Content = append(node.Content, yamlScalar(key), child)
		}
		if child.Kind != yaml.MappingNode {
			return fmt.Errorf("trailconfig: %s: %s is not a mapping", path, key)
		}
		node = child
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("trailconfig: encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
		return fmt.Errorf("trailconfig: write %s: %w", path, err)
	}
	return nil
}

func yamlMappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setYAMLValue(mapping *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = value
			return
		}
	}
	mapping.Content = append(mapping.Content, yamlScalar(key), value)
}

func yamlScalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func yamlSequence(values []string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, value := range values {
		seq.Content = append(seq.Content, yamlScalar(value))
	}
	return seq
}
