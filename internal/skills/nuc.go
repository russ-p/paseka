package skills

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ProvenanceNuc marks a skill installed from a portable Nuc pack.
const ProvenanceNuc = "nuc"

// NucSkill is one skill carried by a portable pack, validated and rewritten
// with nuc provenance. Files is keyed by slash-form path relative to the skill
// directory and includes the `SKILL.md`; ProcedureHash identifies the carried
// procedure and is the value `doctor` checks later.
type NucSkill struct {
	Name          string
	Files         map[string][]byte
	ProcedureHash string
}

// PrepareNucSkill validates one carried skill against the same rule set a store
// scan uses and returns the bytes an import should write. It never touches the
// destination store.
//
// The returned `SKILL.md` carries `source: nuc` and the procedure hash. A
// carried proposal's provenance — trail, role, and approval time — is removed,
// because those facts describe another colony, while every other metadata key
// is preserved.
func PrepareNucSkill(name string, files map[string][]byte) (NucSkill, error) {
	name = strings.TrimSpace(name)
	ref, err := ParseReference(name)
	if err != nil {
		return NucSkill{}, fmt.Errorf("skills: carried skill name: %w", err)
	}
	if ref.IsPattern() {
		return NucSkill{}, fmt.Errorf("skills: carried skill name %q must not contain a wildcard", name)
	}

	planned := make([]installFile, 0, len(files))
	seen := make(map[string]string, len(files))
	for rel, data := range files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") ||
			strings.Contains(clean, "/../") || filepath.IsAbs(rel) {
			return NucSkill{}, fmt.Errorf("skills: skill %q file %q escapes the skill directory", name, rel)
		}
		if prior, ok := seen[clean]; ok {
			return NucSkill{}, fmt.Errorf(
				"skills: skill %q carries %q twice as %q", name, clean, prior,
			)
		}
		seen[clean] = rel
		planned = append(planned, installFile{Rel: clean, Data: data})
	}
	sort.SliceStable(planned, func(i, j int) bool { return planned[i].Rel < planned[j].Rel })

	manifestIndex := -1
	for i, file := range planned {
		if file.Rel == ManifestName {
			manifestIndex = i
			break
		}
	}
	if manifestIndex < 0 {
		return NucSkill{}, fmt.Errorf("skills: skill %q carries no %s", name, ManifestName)
	}

	doc, body, reason := parseDocument(planned[manifestIndex].Data)
	if reason != "" {
		return NucSkill{}, fmt.Errorf("skills: skill %q cannot be imported: %s", name, reason)
	}
	if issues := validateDocument(name, doc); len(issues) > 0 {
		reasons := make([]string, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, issue.Reason)
		}
		return NucSkill{}, fmt.Errorf(
			"skills: skill %q cannot be imported: %s", name, strings.Join(reasons, "; "),
		)
	}

	hash, err := hashInstallContent(body, planned)
	if err != nil {
		return NucSkill{}, fmt.Errorf("skills: skill %q: %w", name, err)
	}
	updated, err := writeProvenanceEdits(
		planned[manifestIndex].Data,
		map[string]string{
			MetadataSource:        ProvenanceNuc,
			MetadataProcedureHash: hash,
		},
		[]string{MetadataTrail, MetadataRole, MetadataApprovedAt},
	)
	if err != nil {
		return NucSkill{}, fmt.Errorf("skills: skill %q: %w", name, err)
	}
	planned[manifestIndex].Data = updated

	out := make(map[string][]byte, len(planned))
	for _, file := range planned {
		out[file.Rel] = file.Data
	}
	return NucSkill{Name: name, Files: out, ProcedureHash: hash}, nil
}
