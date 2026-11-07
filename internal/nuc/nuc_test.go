package nuc_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/nuc"
	"github.com/russ-p/paseka/internal/skills"
)

func TestRoundTripExportImport(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}

	importRoot := t.TempDir()
	writeMinimalColony(t, importRoot)

	res, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: importRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) == 0 {
		t.Fatalf("expected created files, got %+v", res)
	}

	for role, wantBody := range doc.Spec.Bees {
		got, err := os.ReadFile(filepath.Join(importRoot, ".paseka", "bees", role+".yaml"))
		if err != nil {
			t.Fatalf("read bee %q: %v", role, err)
		}
		if string(got) != wantBody {
			t.Fatalf("bee %q mismatch:\nwant:\n%s\ngot:\n%s", role, wantBody, got)
		}
	}
	for ref, wantBody := range doc.Spec.Prompts {
		got, err := os.ReadFile(filepath.Join(importRoot, ".paseka", "prompts", filepath.FromSlash(ref)))
		if err != nil {
			t.Fatalf("read prompt %q: %v", ref, err)
		}
		if string(got) != wantBody {
			t.Fatalf("prompt %q mismatch", ref)
		}
	}
	for id, wantBody := range doc.Spec.Cues {
		got, err := os.ReadFile(filepath.Join(importRoot, ".paseka", "cues", id+".yaml"))
		if err != nil {
			t.Fatalf("read cue %q: %v", id, err)
		}
		if string(got) != wantBody {
			t.Fatalf("cue %q mismatch:\nwant:\n%s\ngot:\n%s", id, wantBody, got)
		}
	}
}

func TestImportSkipVsForce(t *testing.T) {
	root := t.TempDir()
	beesDir := filepath.Join(root, ".paseka", "bees")
	if err := os.MkdirAll(beesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := []byte("role: scout\nadapter: cursor\n")
	if err := os.WriteFile(filepath.Join(beesDir, "scout.yaml"), existing, 0o644); err != nil {
		t.Fatal(err)
	}

	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "test"},
		Spec: nuc.Spec{
			Bees: map[string]string{
				"scout": "role: scout\nadapter: pi\n",
			},
		},
	}

	skipRes, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(skipRes.Skipped) != 1 || len(skipRes.Created) != 0 {
		t.Fatalf("skip: %+v", skipRes)
	}
	got, _ := os.ReadFile(filepath.Join(beesDir, "scout.yaml"))
	if string(got) != string(existing) {
		t.Fatal("skip should not change file")
	}

	forceRes, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(forceRes.Overwritten) != 1 {
		t.Fatalf("force: %+v", forceRes)
	}
	got, _ = os.ReadFile(filepath.Join(beesDir, "scout.yaml"))
	if string(got) != doc.Spec.Bees["scout"] {
		t.Fatal("force should overwrite file")
	}
}

func TestRejectPromptPathTraversal(t *testing.T) {
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "bad"},
		Spec: nuc.Spec{
			Bees: map[string]string{
				"scout": "role: scout\nadapter: cursor\n",
			},
			Prompts: map[string]string{
				"../escape.md": "nope",
			},
		},
	}
	if err := doc.Validate(); err == nil {
		t.Fatal("expected validation error for path traversal")
	}
}

func TestRejectInvalidKind(t *testing.T) {
	_, err := nuc.ParseDocument([]byte(`apiVersion: paseka/v1
kind: Hive
metadata:
  name: x
spec:
  bees:
    scout: |
      role: scout
`))
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

func TestExportOmitsProfilesDirectory(t *testing.T) {
	src := setupNucFixture(t)
	mustWrite(t, filepath.Join(src, ".paseka", "profiles", "pi.yaml"), "adapter: pi\n")

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "profiles") || strings.Contains(string(raw), "adapter: pi") {
		t.Fatalf("nuc export packed profiles: %s", raw)
	}
}

func TestExportBeesFilter(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "filtered",
		Bees:       []string{"scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Bees) != 1 {
		t.Fatalf("bees: %+v", doc.Spec.Bees)
	}
	if _, ok := doc.Spec.Bees["scout"]; !ok {
		t.Fatal("expected scout only")
	}
	if _, ok := doc.Spec.Prompts["scout.md"]; !ok {
		t.Fatal("expected scout prompt in export")
	}
}

func TestExportCuesFilter(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "filtered",
		Cues:       []string{"feature"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Cues) != 1 {
		t.Fatalf("cues: %+v", doc.Spec.Cues)
	}
	if _, ok := doc.Spec.Cues["feature"]; !ok {
		t.Fatal("expected feature only")
	}
	if _, ok := doc.Spec.Cues["hotfix"]; ok {
		t.Fatal("hotfix should be filtered out")
	}
}

func TestExportIncludesAllCuesByDefault(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "all-cues",
		Bees:       []string{"scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Bees) != 1 {
		t.Fatalf("bees: %+v", doc.Spec.Bees)
	}
	if len(doc.Spec.Cues) != 2 {
		t.Fatalf("expected both cues, got %+v", doc.Spec.Cues)
	}
}

func setupNucFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeMinimalColony(t, root)
	mustWrite(t, filepath.Join(root, ".paseka", "bees", "scout.yaml"), "role: scout\nadapter: cursor\nprompt_template: scout.md\n")
	mustWrite(t, filepath.Join(root, ".paseka", "bees", "builder.yaml"), "role: builder\nadapter: cursor\nprompt_template: builder.md\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "scout.md"), "scout prompt\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "builder.md"), "builder prompt\n")
	mustWrite(t, filepath.Join(root, ".paseka", "prompts", "_partials", "emit-howto.md"), "emit howto\n")
	mustMkdir(t, filepath.Join(root, ".paseka", "cues"))
	mustWrite(t, filepath.Join(root, ".paseka", "cues", "feature.yaml"), `description: Feature intake
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)
	mustWrite(t, filepath.Join(root, ".paseka", "cues", "hotfix.yaml"), `description: Hotfix task
emit: task
bee: builder
intent: bugfix
review: none
autorun: true
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
`)
	return root
}

func writeMinimalColony(t *testing.T, root string) {
	t.Helper()
	mustMkdir(t, filepath.Join(root, ".paseka", "bees"))
	mustMkdir(t, filepath.Join(root, ".paseka", "prompts", "_partials"))
	mustWrite(t, filepath.Join(root, ".paseka", "colony.yaml"), "slug: test-colony\n")
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExportCarriesResolvedSkills(t *testing.T) {
	src := setupNucFixture(t)
	mustWrite(t, filepath.Join(src, ".paseka", "bees", "builder.yaml"),
		"role: builder\nadapter: cursor\nprompt_template: builder.md\nskills:\n  - release-proc\n")
	writeStoreSkill(t, src, "release-proc", `---
name: release-proc
description: Run the release procedure.
metadata:
  owner: release-team
---
# Release

Run scripts/run.sh.
`, map[string]string{"scripts/run.sh": "echo hi\n"})

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "skills",
		Bees:       []string{"builder"},
	})
	if err != nil {
		t.Fatal(err)
	}
	files, ok := doc.Spec.Skills["release-proc"]
	if !ok {
		t.Fatalf("skills section missing: %+v", doc.Spec.Skills)
	}
	if !strings.Contains(files["SKILL.md"], "name: release-proc") {
		t.Fatalf("SKILL.md not carried: %q", files["SKILL.md"])
	}
	if files["scripts/run.sh"] != "echo hi\n" {
		t.Fatalf("supporting file not carried: %+v", files)
	}
}

func TestExportRoleWithoutSkillsHasNoSkillsSection(t *testing.T) {
	src := setupNucFixture(t)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "no-skills",
		Bees:       []string{"scout"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Spec.Skills) != 0 {
		t.Fatalf("a role with no skills added a skills section: %+v", doc.Spec.Skills)
	}
}

func TestExportSkillsFilterAddsUnresolvedSkill(t *testing.T) {
	src := setupNucFixture(t)
	writeStoreSkill(t, src, "docs-style", `---
name: docs-style
description: House documentation style.
---
# Docs
`, nil)

	doc, err := nuc.ExportFromColony(nuc.ExportOptions{
		ColonyRoot: src,
		Name:       "explicit",
		Bees:       []string{"scout"},
		Skills:     []string{"docs-style"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Spec.Skills["docs-style"]; !ok {
		t.Fatalf("explicit skill not exported: %+v", doc.Spec.Skills)
	}
}

func TestImportInstallsSkillsWithNucProvenance(t *testing.T) {
	root := t.TempDir()
	writeMinimalColony(t, root)
	manifest := `---
name: release-proc
description: Run the release procedure.
metadata:
  owner: release-team
  source: proposal
  trail: trace-old
  role: archivist
  approved_at: "2026-01-01T00:00:00Z"
  procedure_hash: stale
---
# Release

Run scripts/run.sh.
`
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "skills"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
			Skills: map[string]map[string]string{
				"release-proc": {
					"SKILL.md":        manifest,
					"scripts/run.sh":  "echo hi\n",
					"references/a.md": "reference\n",
				},
			},
		},
	}

	res, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 4 {
		t.Fatalf("created = %v, want bee plus three skill files", res.Created)
	}

	report, err := skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if issues := report.Issues(); len(issues) > 0 {
		t.Fatalf("installed store is invalid: %v", issues)
	}
	var installed skills.Skill
	for _, skill := range report.Skills {
		if skill.Name == "release-proc" {
			installed = skill
		}
	}
	if installed.Name == "" {
		t.Fatal("release-proc was not installed")
	}
	for key, want := range map[string]string{
		skills.MetadataSource: "nuc",
		"owner":               "release-team",
	} {
		if got := installed.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %q, want %q", key, got, want)
		}
	}
	for _, key := range []string{skills.MetadataTrail, skills.MetadataRole, skills.MetadataApprovedAt} {
		if _, ok := installed.Metadata[key]; ok {
			t.Fatalf("proposal provenance %q survived: %+v", key, installed.Metadata)
		}
	}
	hash, err := skills.ProcedureHash(installed.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Metadata[skills.MetadataProcedureHash] != hash {
		t.Fatalf("procedure hash = %q, store hashes to %s", installed.Metadata[skills.MetadataProcedureHash], hash)
	}
	for _, rel := range []string{"scripts/run.sh", "references/a.md"} {
		if _, err := os.Stat(filepath.Join(installed.Dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("supporting file %q missing: %v", rel, err)
		}
	}
}

func TestImportRefusesInvalidSkillBeforeWritingAnything(t *testing.T) {
	root := t.TempDir()
	writeMinimalColony(t, root)
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "bad-skill"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
			Skills: map[string]map[string]string{
				"release-proc": {
					"SKILL.md": "---\nname: release-proc\n---\n# No description\n",
				},
			},
		},
	}
	if _, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root}); err == nil {
		t.Fatal("expected an invalid skill to refuse the pack")
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "bees", "scout.yaml")); !os.IsNotExist(err) {
		t.Fatalf("import wrote a bee before validating the skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "skills")); !os.IsNotExist(err) {
		t.Fatalf("import wrote a skill store: %v", err)
	}
}

func TestRejectSkillNameAndPathEscapes(t *testing.T) {
	base := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "bad"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
		},
	}
	escapeName := base
	escapeName.Spec.Skills = map[string]map[string]string{
		"../escape": {"SKILL.md": "---\nname: escape\ndescription: x\n---\nbody\n"},
	}
	if err := escapeName.Validate(); err == nil {
		t.Fatal("expected a skill name escape to be refused")
	}

	escapePath := base
	escapePath.Spec.Skills = map[string]map[string]string{
		"release-proc": {
			"SKILL.md":      "---\nname: release-proc\ndescription: x\n---\nbody\n",
			"../escape.txt": "nope",
		},
	}
	if err := escapePath.Validate(); err == nil {
		t.Fatal("expected a skill file path escape to be refused")
	}
}

func TestImportSkillSkipAndForce(t *testing.T) {
	root := t.TempDir()
	writeMinimalColony(t, root)
	writeStoreSkill(t, root, "release-proc", "---\nname: release-proc\ndescription: old\n---\n# Old\n", map[string]string{
		"stale.txt": "old\n",
	})
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "skills"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
			Skills: map[string]map[string]string{
				"release-proc": {
					"SKILL.md": "---\nname: release-proc\ndescription: new\n---\n# New\n",
				},
			},
		},
	}

	skip, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(skip.Skipped) != 1 || !strings.HasSuffix(skip.Skipped[0], "release-proc") {
		t.Fatalf("skip = %+v", skip)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".paseka", "skills", "release-proc", "SKILL.md"))
	if !strings.Contains(string(got), "# Old") {
		t.Fatal("skip should not change the skill")
	}

	forced, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(forced.Overwritten) != 2 {
		t.Fatalf("force = %+v, want the bee and the skill", forced)
	}
	got, _ = os.ReadFile(filepath.Join(root, ".paseka", "skills", "release-proc", "SKILL.md"))
	if !strings.Contains(string(got), "# New") {
		t.Fatal("force should replace the skill")
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "skills", "release-proc", "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("force should replace the skill directory as a unit")
	}
}

func TestImportSkillDryRunListsActionsAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	writeMinimalColony(t, root)
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "skills"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
			Skills: map[string]map[string]string{
				"release-proc": {
					"SKILL.md":       "---\nname: release-proc\ndescription: x\n---\nbody\n",
					"scripts/run.sh": "echo hi\n",
				},
			},
		},
	}
	res, err := nuc.Import(doc, nuc.ImportOptions{ColonyRoot: root, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Created) != 3 {
		t.Fatalf("dry-run created = %v, want bee plus two skill files", res.Created)
	}
	for _, p := range res.Created {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("dry-run wrote %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".paseka", "skills")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created the store: %v", err)
	}
}

func writeStoreSkill(t *testing.T, root, name, manifest string, supporting map[string]string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "skills", name)
	mustWrite(t, filepath.Join(dir, "SKILL.md"), manifest)
	for rel, body := range supporting {
		mustWrite(t, filepath.Join(dir, filepath.FromSlash(rel)), body)
	}
}

func TestSkillsSurviveYAMLAndImportRoundTrip(t *testing.T) {
	doc := nuc.Document{
		APIVersion: nuc.APIVersion,
		Kind:       nuc.Kind,
		Metadata:   nuc.Metadata{Name: "yaml"},
		Spec: nuc.Spec{
			Bees: map[string]string{"scout": "role: scout\nadapter: cursor\n"},
			Skills: map[string]map[string]string{
				"release-proc": {
					"SKILL.md":       "---\nname: release-proc\ndescription: Run it.\n---\nbody\n",
					"scripts/run.sh": "echo hi\n",
				},
			},
		},
	}
	data, err := nuc.MarshalDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := nuc.ParseDocument(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Spec.Skills["release-proc"]["scripts/run.sh"] != "echo hi\n" {
		t.Fatalf("skills did not survive YAML: %+v", parsed.Spec.Skills)
	}

	root := t.TempDir()
	writeMinimalColony(t, root)
	if _, err := nuc.Import(parsed, nuc.ImportOptions{ColonyRoot: root}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".paseka", "skills", "release-proc", "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "echo hi\n" {
		t.Fatalf("supporting file mismatch: %q", got)
	}
}
