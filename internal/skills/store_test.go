package skills_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/skills"
)

// writeSkill writes one store entry: <root>/<name>/SKILL.md with the given
// frontmatter (without fences) and body.
func writeSkill(t *testing.T, root, name, frontmatter, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

const validFrontmatter = "name: paseka-release\ndescription: Run the release procedure."

// frontmatter returns a valid frontmatter block for a skill of this name.
func frontmatter(name string) string {
	return "name: " + name + "\ndescription: Run the " + name + " procedure."
}

func scanRoot(t *testing.T, root string) skills.Report {
	t.Helper()
	report, err := skills.Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return report
}

func issueText(report skills.Report) string {
	var lines []string
	for _, issue := range report.Issues() {
		lines = append(lines, issue.Error())
	}
	return strings.Join(lines, "\n")
}

// TestScanAcceptsSpecShapedSkill pins the accepted shape with and without the
// optional supporting directories, and that the scan surfaces what a later
// delivery path needs: name, description, metadata, and body.
func TestScanAcceptsSpecShapedSkill(t *testing.T) {
	cases := []struct {
		name  string
		extra []string
	}{
		{name: "manifest only"},
		{name: "with scripts", extra: []string{"scripts/run.sh"}},
		{name: "with references", extra: []string{"references/style.md"}},
		{name: "with assets", extra: []string{"assets/logo.txt"}},
		{name: "with all", extra: []string{"scripts/run.sh", "references/style.md", "assets/logo.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "paseka-release", validFrontmatter+
				"\nmetadata:\n  source: hand\n  revision: \"3\"", "# Release\n\nDo the thing.\n")
			for _, extra := range tc.extra {
				writeFile(t, filepath.Join(root, "paseka-release", extra), []byte("supporting content\n"))
			}

			report := scanRoot(t, root)
			if !report.Valid() {
				t.Fatalf("valid skill reported issues:\n%s", issueText(report))
			}
			if len(report.Skills) != 1 {
				t.Fatalf("skills = %d, want 1", len(report.Skills))
			}
			skill := report.Skills[0]
			if skill.Name != "paseka-release" || skill.DeclaredName != "paseka-release" {
				t.Fatalf("skill = %+v", skill)
			}
			if skill.Description != "Run the release procedure." {
				t.Fatalf("description = %q", skill.Description)
			}
			if skill.Metadata["source"] != "hand" || skill.Metadata["revision"] != "3" {
				t.Fatalf("metadata = %v", skill.Metadata)
			}
			if !strings.Contains(skill.Body, "# Release") || !strings.Contains(skill.Body, "Do the thing.") {
				t.Fatalf("body = %q", skill.Body)
			}
			if !filepath.IsAbs(skill.Path) || !filepath.IsAbs(skill.Dir) {
				t.Fatalf("resolved paths must be absolute: %+v", skill)
			}
			if skill.Bytes <= 0 {
				t.Fatalf("bytes = %d", skill.Bytes)
			}
		})
	}
}

// TestScanReportsNameRules covers every way a name can be wrong: absent,
// malformed, and different from the directory that holds it. Each finding must
// name the skill and the reason.
func TestScanReportsNameRules(t *testing.T) {
	longName := strings.Repeat("a", skills.MaxNameLength+1)
	cases := []struct {
		name        string
		dir         string
		frontmatter string
		wantReason  string
	}{
		{
			name:        "missing name",
			dir:         "paseka-release",
			frontmatter: "description: A procedure.",
			wantReason:  "name is required",
		},
		{
			name:        "blank name",
			dir:         "paseka-release",
			frontmatter: "name: \"  \"\ndescription: A procedure.",
			wantReason:  "name is required",
		},
		{
			name:        "uppercase name",
			dir:         "Paseka-Release",
			frontmatter: "name: Paseka-Release\ndescription: A procedure.",
			wantReason:  "must use lowercase letters",
		},
		{
			name:        "leading hyphen",
			dir:         "-paseka",
			frontmatter: "name: -paseka\ndescription: A procedure.",
			wantReason:  "must not begin or end with a hyphen",
		},
		{
			name:        "trailing hyphen",
			dir:         "paseka-",
			frontmatter: "name: paseka-\ndescription: A procedure.",
			wantReason:  "must not begin or end with a hyphen",
		},
		{
			name:        "doubled hyphen",
			dir:         "paseka--release",
			frontmatter: "name: paseka--release\ndescription: A procedure.",
			wantReason:  "must not contain a doubled hyphen",
		},
		{
			name:        "over-long name",
			dir:         longName,
			frontmatter: "name: " + longName + "\ndescription: A procedure.",
			wantReason:  "the limit is 64",
		},
		{
			name:        "directory mismatch",
			dir:         "paseka-release",
			frontmatter: "name: paseka-ship\ndescription: A procedure.",
			wantReason:  "does not match its directory",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, tc.dir, tc.frontmatter, "body\n")

			report := scanRoot(t, root)
			if report.Valid() {
				t.Fatalf("%s was accepted", tc.name)
			}
			issues := report.Issues()
			if len(issues) != 1 {
				t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
			}
			if issues[0].Skill != tc.dir {
				t.Fatalf("issue skill = %q, want %q", issues[0].Skill, tc.dir)
			}
			if !strings.Contains(issues[0].Reason, tc.wantReason) {
				t.Fatalf("reason = %q, want %q", issues[0].Reason, tc.wantReason)
			}
			if !strings.Contains(issues[0].Error(), tc.dir) {
				t.Fatalf("rendered issue does not name the skill: %q", issues[0].Error())
			}
		})
	}
}

// TestScanReportsDescriptionRules covers a description that is absent, empty, or
// past the standard's bound.
func TestScanReportsDescriptionRules(t *testing.T) {
	long := strings.Repeat("d", skills.MaxDescriptionLength+1)
	cases := []struct {
		name        string
		frontmatter string
		wantReason  string
	}{
		{
			name:        "missing description",
			frontmatter: "name: paseka-release",
			wantReason:  "description is required",
		},
		{
			name:        "empty description",
			frontmatter: "name: paseka-release\ndescription: \"\"",
			wantReason:  "description is required",
		},
		{
			name:        "blank description",
			frontmatter: "name: paseka-release\ndescription: \"   \"",
			wantReason:  "description is empty",
		},
		{
			name:        "over-long description",
			frontmatter: "name: paseka-release\ndescription: " + long,
			wantReason:  "the limit is 1024",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "paseka-release", tc.frontmatter, "body\n")

			report := scanRoot(t, root)
			issues := report.Issues()
			if len(issues) != 1 {
				t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
			}
			if issues[0].Skill != "paseka-release" || !strings.Contains(issues[0].Reason, tc.wantReason) {
				t.Fatalf("issue = %+v, want reason %q", issues[0], tc.wantReason)
			}
		})
	}
}

// TestScanReportsBrokenManifest covers a directory that is not a skill yet: no
// SKILL.md, no frontmatter, and frontmatter that is not YAML.
func TestScanReportsBrokenManifest(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		wantReason string
	}{
		{name: "missing manifest", content: "", wantReason: "SKILL.md is missing"},
		{name: "no frontmatter", content: "# Just a body\n", wantReason: "must begin with a --- frontmatter block"},
		{name: "unclosed frontmatter", content: "---\nname: paseka-release\n", wantReason: "must begin with a --- frontmatter block"},
		{name: "invalid yaml", content: "---\nname: [unclosed\n---\nbody\n", wantReason: "invalid frontmatter"},
		{name: "non-string metadata", content: "---\nname: paseka-release\ndescription: ok\nmetadata:\n  count: 3\n---\nbody\n", wantReason: "invalid frontmatter"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "paseka-release")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.content != "" {
				writeFile(t, filepath.Join(dir, skills.ManifestName), []byte(tc.content))
			}

			report := scanRoot(t, root)
			issues := report.Issues()
			if len(issues) != 1 {
				t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
			}
			if issues[0].Skill != "paseka-release" || !strings.Contains(issues[0].Reason, tc.wantReason) {
				t.Fatalf("issue = %+v, want reason %q", issues[0], tc.wantReason)
			}
		})
	}
}

// TestScanEnforcesFileBudget pins the 256 KiB per-file bound at its boundary.
func TestScanEnforcesFileBudget(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paseka-release", validFrontmatter, "body\n")
	atLimit := filepath.Join(root, "paseka-release", "scripts", "at-limit.bin")
	writeFile(t, atLimit, make([]byte, skills.MaxSkillFileBytes))

	report := scanRoot(t, root)
	if !report.Valid() {
		t.Fatalf("a file exactly at the limit was rejected:\n%s", issueText(report))
	}

	writeFile(t, atLimit, make([]byte, skills.MaxSkillFileBytes+1))
	report = scanRoot(t, root)
	issues := report.Issues()
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
	}
	if issues[0].Skill != "paseka-release" || !strings.Contains(issues[0].Reason, "may not exceed 262144 bytes") {
		t.Fatalf("issue = %+v", issues[0])
	}
	if !strings.Contains(issues[0].Reason, "at-limit.bin") {
		t.Fatalf("issue does not name the file: %q", issues[0].Reason)
	}
}

// TestScanEnforcesStoreBudget pins the 8 MiB store bound at its boundary and
// proves the ledger beside the store is not counted.
func TestScanEnforcesStoreBudget(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paseka-release", validFrontmatter, "body\n")
	manifest := int64(len("---\n" + validFrontmatter + "\n---\nbody\n"))

	// Fill the store to exactly its bound, in files each at the per-file limit.
	remaining := int64(skills.MaxStoreBytes) - manifest
	for i := 0; remaining > 0; i++ {
		size := min(remaining, int64(skills.MaxSkillFileBytes))
		writeFile(t, filepath.Join(root, "paseka-release", "assets", fmt.Sprintf("chunk-%02d", i)), make([]byte, size))
		remaining -= size
	}

	report := scanRoot(t, root)
	if !report.Valid() {
		t.Fatalf("a store exactly at the limit was rejected:\n%s", issueText(report))
	}
	if report.Bytes != skills.MaxStoreBytes {
		t.Fatalf("store bytes = %d, want %d", report.Bytes, skills.MaxStoreBytes)
	}

	// The ledger is not the store's content, even at the store root.
	writeFile(t, filepath.Join(root, skills.LedgerName), make([]byte, 4096))
	if report = scanRoot(t, root); !report.Valid() {
		t.Fatalf("the ledger was counted against the store:\n%s", issueText(report))
	}

	// One more byte inside the store breaches it.
	writeFile(t, filepath.Join(root, "paseka-release", "assets", "one-more"), []byte("x"))

	report = scanRoot(t, root)
	issues := report.Issues()
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
	}
	if issues[0].Skill != "" || !strings.Contains(issues[0].Reason, "store total is") {
		t.Fatalf("issue = %+v, want a store-wide budget finding", issues[0])
	}
}

// TestScanRejectsNonSkillEntries pins that an immediate directory is what a
// skill is, so a stray file at the store root is reported rather than ignored.
func TestScanRejectsNonSkillEntries(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paseka-release", validFrontmatter, "body\n")
	writeFile(t, filepath.Join(root, "notes.txt"), []byte("not a skill\n"))

	report := scanRoot(t, root)
	issues := report.Issues()
	if len(issues) != 1 {
		t.Fatalf("issues = %d, want 1:\n%s", len(issues), issueText(report))
	}
	if issues[0].Skill != "" || !strings.Contains(issues[0].Reason, "is not a directory") {
		t.Fatalf("issue = %+v", issues[0])
	}
}

// TestScanMissingStoreIsEmpty pins that a colony with no skills is valid: the
// absence of the store is not a broken store.
func TestScanMissingStoreIsEmpty(t *testing.T) {
	report := scanRoot(t, filepath.Join(t.TempDir(), "skills"))
	if !report.Valid() || len(report.Skills) != 0 || report.Bytes != 0 {
		t.Fatalf("missing store = %+v", report)
	}
	ref, err := skills.ParseReference("*")
	if err != nil {
		t.Fatal(err)
	}
	if issues := report.Find(ref); len(issues) != 0 {
		t.Fatalf("* on an empty store must be allowed: %v", issues)
	}
}

// TestScanFollowsLinkedSkillDirectory pins that a linked skill is read the way
// the trail capture reads it, rather than rejected as a non-directory.
func TestScanFollowsLinkedSkillDirectory(t *testing.T) {
	root := t.TempDir()
	target := t.TempDir()
	writeSkill(t, target, "paseka-release", validFrontmatter, "body\n")
	writeFile(t, filepath.Join(target, "paseka-release", "scripts", "run.sh"), []byte("#!/bin/sh\n"))
	if err := os.Symlink(filepath.Join(target, "paseka-release"), filepath.Join(root, "paseka-release")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	report := scanRoot(t, root)
	if !report.Valid() {
		t.Fatalf("linked skill reported issues:\n%s", issueText(report))
	}
	if len(report.Skills) != 1 || report.Bytes < int64(len("#!/bin/sh\n")) {
		t.Fatalf("report = %+v", report)
	}
	if report.Skills[0].Description != "Run the release procedure." {
		t.Fatalf("skill = %+v", report.Skills[0])
	}
}

// TestScanColonyUsesRepositoryRoot pins that the store path is the repository
// one and that a ledger beside it is not reported as a store entry.
func TestScanColonyUsesRepositoryRoot(t *testing.T) {
	colonyRoot := t.TempDir()
	writeSkill(t, filepath.Join(colonyRoot, ".paseka", "skills"), "paseka-release", validFrontmatter, "body\n")
	writeFile(t, skills.LedgerPath(colonyRoot), []byte("{}\n"))

	report, err := skills.ScanColony(colonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Valid() {
		t.Fatalf("issues:\n%s", issueText(report))
	}
	if got, want := report.Root, filepath.Join(colonyRoot, ".paseka", "skills"); got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
	if got, want := skills.Dir(colonyRoot), filepath.Join(colonyRoot, ".paseka", "skills"); got != want {
		t.Fatalf("Dir = %q, want %q", got, want)
	}
}
