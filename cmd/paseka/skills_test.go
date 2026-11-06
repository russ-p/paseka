package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/skills"
)

// setupSkillsCLI builds a git repository with a colony manifest and the
// machine-local config every command resolution needs, so a test can drop a
// skill store into it and run the real command tree against it.
func setupSkillsCLI(t *testing.T) string {
	t.Helper()
	t.Setenv("PASEKA_PROFILE", "")
	t.Setenv("PASEKA_NATS_URL", "")
	repo := t.TempDir()
	runStatusGit(t, repo, "init")
	runStatusGit(t, repo, "config", "user.email", "test@test.com")
	runStatusGit(t, repo, "config", "user.name", "test")

	paseka := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(filepath.Join(paseka, "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paseka, "colony.yaml"), []byte("slug: skills-fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStatusGit(t, repo, "add", ".paseka")
	runStatusGit(t, repo, "commit", "-m", "skills fixture")

	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", "skills-fixture")
	if err := os.MkdirAll(filepath.Join(homeDir, "adapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "colony_root: " + repo + "\nslug: skills-fixture\n"
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func writeStoreSkill(t *testing.T, repo, name, frontmatter string) {
	t.Helper()
	dir := filepath.Join(skills.Dir(repo), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestSkillsValidateCleanStore(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")

	out, err := runCLI(t, "skills", "validate", "-C", repo)
	if err != nil {
		t.Fatalf("validate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skills: ok") {
		t.Fatalf("output = %q", out)
	}
}

func TestSkillsValidateEmptyStore(t *testing.T) {
	repo := setupSkillsCLI(t)

	out, err := runCLI(t, "skills", "validate", "-C", repo)
	if err != nil {
		t.Fatalf("validate on an empty store: %v\n%s", err, out)
	}
	// The wildcard is allowed to match an empty store; a name is not.
	if out, err = runCLI(t, "skills", "validate", "*", "-C", repo); err != nil {
		t.Fatalf("validate * on an empty store: %v\n%s", err, out)
	}
	if out, err = runCLI(t, "skills", "validate", "paseka-release", "-C", repo); err == nil {
		t.Fatalf("a name that matches nothing must fail:\n%s", out)
	}
}

func TestSkillsValidateReportsInvalidSkill(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-ship\ndescription: Run the release procedure.")

	out, err := runCLI(t, "skills", "validate", "-C", repo)
	if err == nil {
		t.Fatalf("an invalid skill must fail:\n%s", out)
	}
	for _, want := range []string{
		`skill "paseka-release"`,
		`name "paseka-ship" does not match its directory "paseka-release"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSkillsValidateBudgetBreach(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	big := filepath.Join(skills.Dir(repo), "paseka-release", "scripts", "big.bin")
	if err := os.MkdirAll(filepath.Dir(big), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(big, make([]byte, skills.MaxSkillFileBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "validate", "-C", repo)
	if err == nil {
		t.Fatalf("a file over 256 KiB must fail:\n%s", out)
	}
	if !strings.Contains(out, "may not exceed") {
		t.Fatalf("output = %q", out)
	}
}

// TestSkillsValidateReferences pins the pattern form and the references that are
// refused outright: a tier prefix or a path names something that cannot exist in
// a one-root store.
func TestSkillsValidateReferences(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-ship\ndescription: Run the ship procedure.")
	writeStoreSkill(t, repo, "other", "name: other\ndescription: Something else.")

	accepted := []string{"paseka-*", "*", "paseka-release", "other"}
	for _, ref := range accepted {
		t.Run("accept "+ref, func(t *testing.T) {
			out, err := runCLI(t, "skills", "validate", ref, "-C", repo)
			if err != nil {
				t.Fatalf("validate %q: %v\n%s", ref, err, out)
			}
		})
	}

	refused := []struct {
		ref  string
		want string
	}{
		{ref: "shared:foo", want: "tier prefixes are not supported"},
		{ref: "local:foo", want: "tier prefixes are not supported"},
		{ref: "./foo", want: "not a path"},
		{ref: "nope-*", want: "matches no skill"},
		{ref: "nope", want: "matches no skill"},
		{ref: "foo?", want: "the only wildcard is *"},
	}
	for _, tc := range refused {
		t.Run("refuse "+tc.ref, func(t *testing.T) {
			out, err := runCLI(t, "skills", "validate", tc.ref, "-C", repo)
			if err == nil {
				t.Fatalf("validate %q was accepted:\n%s", tc.ref, out)
			}
			if !strings.Contains(out+err.Error(), tc.want) {
				t.Fatalf("output missing %q:\n%s", tc.want, out)
			}
		})
	}
}

// TestSkillsGroupPrintsHelp pins that the group is a namespace, not an action:
// bare `paseka skills` prints help, exits zero, and changes nothing.
func TestSkillsGroupPrintsHelp(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	before := repoTree(t, repo)

	out, err := runCLI(t, "skills")
	if err != nil {
		t.Fatalf("bare skills group: %v\n%s", err, out)
	}
	if !strings.Contains(out, "validate") || !strings.Contains(out, "Usage:") {
		t.Fatalf("help output = %q", out)
	}
	after := repoTree(t, repo)
	if before != after {
		t.Fatalf("bare skills group changed the tree:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// TestDoctorReportsSkillFindings pins that doctor prints the same verdict as
// `skills validate` and fails on it, and that a clean store adds no finding.
func TestDoctorReportsSkillFindings(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-ship\ndescription: Run the release procedure.")

	out, err := runCLI(t, "doctor", "-C", repo)
	if err == nil {
		t.Fatalf("doctor must fail on an invalid skill:\n%s", out)
	}
	if !strings.Contains(out, `skill "paseka-release"`) {
		t.Fatalf("doctor output missing the skill finding:\n%s", out)
	}

	clean := setupSkillsCLI(t)
	writeStoreSkill(t, clean, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	out, _ = runCLI(t, "doctor", "-C", clean)
	if strings.Contains(out, `skill "paseka-release"`) {
		t.Fatalf("doctor reported a valid skill:\n%s", out)
	}
}

// repoTree lists the repository's files so a test can prove a command changed
// nothing.
func repoTree(t *testing.T, repo string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(repo, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		lines = append(lines, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

func writeCLIBee(t *testing.T, repo, role, body string) {
	t.Helper()
	path := filepath.Join(repo, ".paseka", "bees", role+".yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorReportsUnresolvedSkillReference pins that doctor's verdict is the
// resolution verdict: a role naming a skill that is not in the store is an
// error, not a silently empty set.
func TestDoctorReportsUnresolvedSkillReference(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "scout", "role: scout\nadapter: cursor\nskills:\n  - missing\n")

	out, err := runCLI(t, "doctor", "-C", repo)
	if err == nil {
		t.Fatalf("doctor must fail on an unresolvable reference:\n%s", out)
	}
	for _, want := range []string{`bee "scout"`, "matches no skill"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsUnmatchedSkillPattern(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "scout", "role: scout\nadapter: cursor\nskills:\n  - other-*\n")

	out, err := runCLI(t, "doctor", "-C", repo)
	if err == nil {
		t.Fatalf("doctor must fail on an unmatched pattern:\n%s", out)
	}
	if !strings.Contains(out, `reference "other-*" matches no skill`) {
		t.Fatalf("doctor output = %q", out)
	}
}

func TestDoctorWarnsExplicitEmptySkillList(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeCLIBee(t, repo, "scout", "role: scout\nadapter: cursor\nskills: []\n")

	out, _ := runCLI(t, "doctor", "-C", repo)
	if !strings.Contains(out, `bee "scout" declares an explicit empty skill list`) {
		t.Fatalf("doctor output = %q", out)
	}
}

func TestDoctorReportsDisabledSkillAndRoles(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "scout", "role: scout\nadapter: cursor\nskills:\n  - paseka-release\n")
	ledger := `{
  "schema_version": 1,
  "disabled": {
    "paseka-release": {"at": "2026-11-06T12:00:00Z", "reason": "broken step"}
  }
}`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "skills.index.json"), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCLI(t, "doctor", "-C", repo)
	for _, want := range []string{
		`skill "paseka-release" is disabled`,
		"broken step",
		"bee(s): scout",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out)
		}
	}
}

func TestDoctorReportsSkillBudgetBreach(t *testing.T) {
	repo := setupSkillsCLI(t)
	for i := 0; i < skills.MaxSkillsPerRole+1; i++ {
		name := "skill-" + fmt.Sprintf("%02d", i)
		writeStoreSkill(t, repo, name, "name: "+name+"\ndescription: A valid skill.")
	}
	writeCLIBee(t, repo, "scout", "role: scout\nadapter: cursor\nskills:\n  - \"*\"\n")

	out, err := runCLI(t, "doctor", "-C", repo)
	if err == nil {
		t.Fatalf("doctor must fail on the 33rd skill:\n%s", out)
	}
	if !strings.Contains(out, "limit is 32") {
		t.Fatalf("doctor output = %q", out)
	}
}

func TestSkillCommandsFailOnCorruptLedger(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "skills.index.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := runCLI(t, "skills", "validate", "-C", repo); err == nil {
		t.Fatalf("skills validate must fail on a corrupt ledger:\n%s", out)
	}
	out, err := runCLI(t, "doctor", "-C", repo)
	if err == nil {
		t.Fatalf("doctor must fail on a corrupt ledger:\n%s", out)
	}
	if !strings.Contains(out, "corrupt") {
		t.Fatalf("doctor output = %q", out)
	}
}

// writeStoreSkillBody writes one store entry with a given body, so a show test
// can prove the body is printed without its frontmatter.
func writeStoreSkillBody(t *testing.T, repo, name, frontmatter, body string) {
	t.Helper()
	dir := filepath.Join(skills.Dir(repo), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillLedger(t *testing.T, repo, disabledJSON string) {
	t.Helper()
	ledger := `{"schema_version": 1, "disabled": ` + disabledJSON + "}\n"
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "skills.index.json"), []byte(ledger), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tableColumns splits a rendered table row on the two-or-more spaces tabwriter
// leaves between columns. Descriptions are single-spaced in the fixtures, so the
// last column stays whole.
func tableColumns(line string) []string {
	return regexp.MustCompile(`\s{2,}`).Split(strings.TrimSpace(line), -1)
}

// skillTableRows returns the table's rows keyed by skill name.
func skillTableRows(t *testing.T, out string) map[string][]string {
	t.Helper()
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := tableColumns(line)
		if len(cols) < 4 || cols[0] == "NAME" {
			continue
		}
		rows[cols[0]] = cols
	}
	return rows
}

// TestSkillsListTable pins the four columns and the roles column's computed
// answer: a bare-name role, a family pattern, and `*` each appear against every
// skill they resolve to, an invalid skill carries its reason in the state
// column, and a disabled skill says so.
func TestSkillsListTable(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-shipped\ndescription: Run the ship procedure.")
	writeStoreSkill(t, repo, "legacy", "name: legacy\ndescription: The old procedure.")
	writeSkillLedger(t, repo, `{"legacy": {"at": "2026-11-06T12:00:00Z", "reason": "step 2 is wrong"}}`)

	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	writeCLIBee(t, repo, "shipper", "role: shipper\nadapter: cursor\nskills:\n  - \"paseka-*\"\n")
	writeCLIBee(t, repo, "allrounder", "role: allrounder\nadapter: cursor\nskills:\n  - \"*\"\n")
	writeCLIBee(t, repo, "optout", "role: optout\nadapter: cursor\nskills: []\n")

	out, err := runCLI(t, "skills", "list", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "ROLES") || !strings.Contains(out, "DESCRIPTION") {
		t.Fatalf("table header missing:\n%s", out)
	}

	rows := skillTableRows(t, out)
	release, ok := rows["paseka-release"]
	if !ok {
		t.Fatalf("no row for paseka-release:\n%s", out)
	}
	if release[1] != "ok" {
		t.Fatalf("state column = %q, want ok", release[1])
	}
	for _, role := range []string{"builder", "shipper", "allrounder"} {
		if !strings.Contains(release[2], role) {
			t.Fatalf("roles column %q missing %q:\n%s", release[2], role, out)
		}
	}
	if strings.Contains(release[2], "optout") {
		t.Fatalf("an explicit empty list must not name a role: %q", release[2])
	}

	ship, ok := rows["paseka-ship"]
	if !ok {
		t.Fatalf("no row for paseka-ship:\n%s", out)
	}
	if !strings.HasPrefix(ship[1], "invalid:") {
		t.Fatalf("invalid state column = %q", ship[1])
	}
	if !strings.Contains(ship[1], "does not match its directory") {
		t.Fatalf("invalid state is missing its reason: %q", ship[1])
	}
	// A pattern role is named against the skill it resolves to even when the
	// skill itself is invalid: the gate still matches it.
	if !strings.Contains(ship[2], "shipper") || !strings.Contains(ship[2], "allrounder") {
		t.Fatalf("roles column %q missing pattern roles", ship[2])
	}

	legacy, ok := rows["legacy"]
	if !ok {
		t.Fatalf("no row for legacy:\n%s", out)
	}
	if !strings.HasPrefix(legacy[1], "disabled:") || !strings.Contains(legacy[1], "step 2 is wrong") {
		t.Fatalf("disabled state column = %q", legacy[1])
	}
	if !strings.Contains(legacy[2], "allrounder") {
		t.Fatalf("wildcard role must name a disabled skill it matches: %q", legacy[2])
	}
}

// TestSkillsListJSON pins the machine contract: stable keys, arrays rather than
// nulls, and the state and reason kept separate from the table's rendering.
func TestSkillsListJSON(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-shipped\ndescription: Run the ship procedure.")
	writeStoreSkill(t, repo, "legacy", "name: legacy\ndescription: The old procedure.")
	writeSkillLedger(t, repo, `{"legacy": {"at": "2026-11-06T12:00:00Z", "reason": "step 2 is wrong"}}`)
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	writeCLIBee(t, repo, "allrounder", "role: allrounder\nadapter: cursor\nskills:\n  - \"*\"\n")

	out, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list --json: %v\n%s", err, out)
	}
	var doc struct {
		Store  string `json:"store"`
		Skills []struct {
			Name        string   `json:"name"`
			State       string   `json:"state"`
			Reason      string   `json:"reason"`
			Roles       []string `json:"roles"`
			Description string   `json:"description"`
		} `json:"skills"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc.Store != skills.Dir(repo) {
		t.Fatalf("store = %q, want %q", doc.Store, skills.Dir(repo))
	}
	if doc.Skills == nil || doc.Issues == nil {
		t.Fatalf("skills and issues must be arrays, got %#v", doc)
	}
	if len(doc.Skills) != 3 {
		t.Fatalf("got %d skills, want 3:\n%s", len(doc.Skills), out)
	}
	byName := map[string]int{}
	for i, entry := range doc.Skills {
		byName[entry.Name] = i
	}
	release := doc.Skills[byName["paseka-release"]]
	if release.State != "ok" || release.Reason != "" {
		t.Fatalf("paseka-release = %+v", release)
	}
	if strings.Join(release.Roles, ",") != "allrounder,builder" {
		t.Fatalf("paseka-release roles = %v", release.Roles)
	}
	if release.Description != "Run the release procedure." {
		t.Fatalf("paseka-release description = %q", release.Description)
	}
	ship := doc.Skills[byName["paseka-ship"]]
	if ship.State != "invalid" || !strings.Contains(ship.Reason, "does not match its directory") {
		t.Fatalf("paseka-ship = %+v", ship)
	}
	legacy := doc.Skills[byName["legacy"]]
	if legacy.State != "disabled" || legacy.Reason != "step 2 is wrong" {
		t.Fatalf("legacy = %+v", legacy)
	}
}

// TestSkillsListEmptyStore pins that an empty store is a valid inventory with
// an empty, parseable contract.
func TestSkillsListEmptyStore(t *testing.T) {
	repo := setupSkillsCLI(t)
	out, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list on an empty store: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"skills": []`) {
		t.Fatalf("empty store must render an empty array:\n%s", out)
	}
}

// TestSkillsShow pins metadata, provenance, and a body printed without its
// frontmatter, for a skill with a provenance record and for a hand-written one.
func TestSkillsShow(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkillBody(t, repo, "paseka-release",
		"name: paseka-release\ndescription: Run the release procedure.\nmetadata:\n  source: proposal\n  trail: trace-abc123",
		"# Release\n\n1. Run scripts/checks.sh.\n")
	writeStoreSkillBody(t, repo, "docs-style",
		"name: docs-style\ndescription: Write the docs.",
		"# Docs\n\nShort sentences.\n")

	out, err := runCLI(t, "skills", "show", "paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills show: %v\n%s", err, out)
	}
	for _, want := range []string{
		"name: paseka-release",
		"description: Run the release procedure.",
		"provenance: proposal",
		"metadata:",
		"source: proposal",
		"trail: trace-abc123",
		"# Release",
		"1. Run scripts/checks.sh.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("show output missing %q:\n%s", want, out)
		}
	}
	// The frontmatter is not reprinted as a block: the body starts after it.
	if strings.Contains(out, "---") {
		t.Fatalf("show reprinted the frontmatter fence:\n%s", out)
	}

	out, err = runCLI(t, "skills", "show", "docs-style", "-C", repo)
	if err != nil {
		t.Fatalf("skills show docs-style: %v\n%s", err, out)
	}
	if !strings.Contains(out, "provenance: hand") {
		t.Fatalf("a skill with no provenance record must read as hand:\n%s", out)
	}
	if !strings.Contains(out, "metadata: (none)") {
		t.Fatalf("show output missing empty metadata marker:\n%s", out)
	}
}

// TestSkillsShowPath pins the path-only mode: one line, the resolved SKILL.md,
// nothing else, so a script can consume it.
func TestSkillsShowPath(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")

	out, err := runCLI(t, "skills", "show", "paseka-release", "--path", "-C", repo)
	if err != nil {
		t.Fatalf("skills show --path: %v\n%s", err, out)
	}
	want := filepath.Join(skills.Dir(repo), "paseka-release", skills.ManifestName)
	if strings.TrimSpace(out) != want {
		t.Fatalf("path output = %q, want %q", out, want)
	}
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 1 {
		t.Fatalf("path output must be one line, got %d:\n%s", len(lines), out)
	}
}

// TestSkillsShowRefusesSet pins that show acts on one procedure: a pattern and
// `*` are refused, and an unknown name is reported by name.
func TestSkillsShowRefusesSet(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-ship\ndescription: Run the ship procedure.")

	for _, ref := range []string{"paseka-*", "*"} {
		out, err := runCLI(t, "skills", "show", ref, "-C", repo)
		if err == nil {
			t.Fatalf("show %q was accepted:\n%s", ref, out)
		}
		if !strings.Contains(err.Error(), "acts on one skill") {
			t.Fatalf("show %q error = %v", ref, err)
		}
	}

	out, err := runCLI(t, "skills", "show", "nope", "-C", repo)
	if err == nil {
		t.Fatalf("show of an unknown name was accepted:\n%s", out)
	}
	for _, want := range []string{`"nope"`, "matches no skill"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("show error missing %q: %v", want, err)
		}
	}
}

// TestSkillsShowJSON pins the show contract a script consumes.
func TestSkillsShowJSON(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkillBody(t, repo, "paseka-release",
		"name: paseka-release\ndescription: Run the release procedure.\nmetadata:\n  source: nuc",
		"# Release\n\nDo the thing.\n")
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")

	out, err := runCLI(t, "skills", "show", "paseka-release", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills show --json: %v\n%s", err, out)
	}
	var doc struct {
		Name        string            `json:"name"`
		Description string            `json:"description"`
		State       string            `json:"state"`
		Roles       []string          `json:"roles"`
		Path        string            `json:"path"`
		Provenance  string            `json:"provenance"`
		Metadata    map[string]string `json:"metadata"`
		Body        string            `json:"body"`
		Issues      []string          `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc.Name != "paseka-release" || doc.State != "ok" {
		t.Fatalf("document = %+v", doc)
	}
	if doc.Provenance != "nuc" || doc.Metadata["source"] != "nuc" {
		t.Fatalf("provenance = %q metadata = %v", doc.Provenance, doc.Metadata)
	}
	if strings.Join(doc.Roles, ",") != "builder" {
		t.Fatalf("roles = %v", doc.Roles)
	}
	if doc.Path != filepath.Join(skills.Dir(repo), "paseka-release", skills.ManifestName) {
		t.Fatalf("path = %q", doc.Path)
	}
	if doc.Body != "# Release\n\nDo the thing.\n" {
		t.Fatalf("body = %q", doc.Body)
	}
}

// TestSkillsListStoreWideIssue pins the two things that have no skill row: a
// stray entry at the store root is reported after the table (and in the JSON
// issues array), and a skill no role carries shows `-` rather than a blank.
func TestSkillsListStoreWideIssue(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "docs-style", "name: docs-style\ndescription: Write the docs.")
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	if err := os.WriteFile(filepath.Join(skills.Dir(repo), "README.md"), []byte("not a skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "list", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, out)
	}
	if !strings.Contains(out, `store: entry "README.md" is not a directory`) {
		t.Fatalf("store-wide finding missing:\n%s", out)
	}
	rows := skillTableRows(t, out)
	docs, ok := rows["docs-style"]
	if !ok {
		t.Fatalf("no row for docs-style:\n%s", out)
	}
	if docs[2] != "-" {
		t.Fatalf("an uncarried skill's roles column = %q, want -", docs[2])
	}

	jsonOut, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list --json: %v\n%s", err, jsonOut)
	}
	var doc struct {
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, jsonOut)
	}
	if len(doc.Issues) != 1 || !strings.Contains(doc.Issues[0], "README.md") {
		t.Fatalf("issues = %v", doc.Issues)
	}
}

// pasekaFiles snapshots every file under .paseka with its bytes, so a mutating
// command can be proved to have left the store and ledger byte-identical.
func pasekaFiles(t *testing.T, repo string) map[string]string {
	t.Helper()
	files := map[string]string{}
	root := filepath.Join(repo, ".paseka")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func sameFiles(before, after map[string]string) bool {
	if len(before) != len(after) {
		return false
	}
	for name, content := range before {
		if after[name] != content {
			return false
		}
	}
	return true
}

// TestSkillsDisableEnableUndoEachOther pins the two reversible flips: disable
// records when and why, names the affected roles, and leaves the skill in the
// store and in the listing; enable removes the record and restores the offer.
func TestSkillsDisableEnableUndoEachOther(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	writeCLIBee(t, repo, "allrounder", "role: allrounder\nadapter: cursor\nskills:\n  - \"*\"\n")

	out, err := runCLI(t, "skills", "disable", "paseka-release", "--reason", "step 2 is wrong", "-C", repo)
	if err != nil {
		t.Fatalf("skills disable: %v\n%s", err, out)
	}
	for _, want := range []string{
		`disabled "paseka-release"`,
		"step 2 is wrong",
		"affected roles: allrounder, builder",
		"the skill stays in the store",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("disable output missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(skills.Dir(repo), "paseka-release", skills.ManifestName)); err != nil {
		t.Fatalf("disable deleted the store entry: %v", err)
	}
	ledger, err := skills.LoadLedger(repo)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := ledger.Record("paseka-release")
	if !ok || record.Reason != "step 2 is wrong" || record.At == "" {
		t.Fatalf("ledger record = %+v, ok = %v", record, ok)
	}

	// A disabled skill is not offered, but it is still listed and the decision
	// is visible on the row.
	listOut, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, `"state": "disabled"`) || !strings.Contains(listOut, "step 2 is wrong") {
		t.Fatalf("disabled skill is missing from the listing:\n%s", listOut)
	}
	doctorOut, _ := runCLI(t, "doctor", "-C", repo)
	if !strings.Contains(doctorOut, `skill "paseka-release" is disabled`) || !strings.Contains(doctorOut, "builder") {
		t.Fatalf("doctor did not report the disabled skill and its roles:\n%s", doctorOut)
	}

	out, err = runCLI(t, "skills", "enable", "paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills enable: %v\n%s", err, out)
	}
	for _, want := range []string{`enabled "paseka-release"`, "affected roles: allrounder, builder"} {
		if !strings.Contains(out, want) {
			t.Fatalf("enable output missing %q:\n%s", want, out)
		}
	}
	ledger, err = skills.LoadLedger(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.Record("paseka-release"); ok {
		t.Fatal("enable left the disabled record behind")
	}

	listOut, err = runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, `"state": "ok"`) {
		t.Fatalf("enabled skill is not listed as ok:\n%s", listOut)
	}
}

// TestSkillsRemoveRefusesReferencedRoleAndForce pins the refusal, its blast
// radius, and the override: remove deletes the store entry and the ledger
// record, and leaves no orphan.
func TestSkillsRemoveRefusesReferencedRoleAndForce(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	if out, err := runCLI(t, "skills", "disable", "paseka-release", "--reason", "broken", "-C", repo); err != nil {
		t.Fatalf("skills disable: %v\n%s", err, out)
	}

	out, err := runCLI(t, "skills", "remove", "paseka-release", "-C", repo)
	if err == nil {
		t.Fatalf("remove was accepted while a role references the skill:\n%s", out)
	}
	for _, want := range []string{`remove "paseka-release"`, "affected roles: builder", "role(s) builder still reference it"} {
		if !strings.Contains(out+err.Error(), want) {
			t.Fatalf("refusal missing %q:\n%s", want, out+err.Error())
		}
	}
	if _, err := os.Stat(filepath.Join(skills.Dir(repo), "paseka-release")); err != nil {
		t.Fatalf("a refused removal deleted the store entry: %v", err)
	}

	out, err = runCLI(t, "skills", "remove", "paseka-release", "--force", "-C", repo)
	if err != nil {
		t.Fatalf("skills remove --force: %v\n%s", err, out)
	}
	if !strings.Contains(out, `removed "paseka-release"`) || !strings.Contains(out, "affected roles: builder") {
		t.Fatalf("forced removal output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(skills.Dir(repo), "paseka-release")); !os.IsNotExist(err) {
		t.Fatalf("remove left the store entry behind: %v", err)
	}
	ledger, err := skills.LoadLedger(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ledger.Record("paseka-release"); ok {
		t.Fatal("remove left the ledger record behind")
	}
	listOut, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, listOut)
	}
	if strings.Contains(listOut, `"name": "paseka-release"`) {
		t.Fatalf("a removed skill is still listed:\n%s", listOut)
	}
}

// TestSkillsRemoveDryRunLeavesStoreIdentical pins that the preview prints the
// plan and touches nothing: the store and ledger stay byte-identical.
func TestSkillsRemoveDryRunLeavesStoreIdentical(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeCLIBee(t, repo, "builder", "role: builder\nadapter: cursor\nskills:\n  - paseka-release\n")
	if out, err := runCLI(t, "skills", "disable", "paseka-release", "--reason", "broken", "-C", repo); err != nil {
		t.Fatalf("skills disable: %v\n%s", err, out)
	}
	before := pasekaFiles(t, repo)

	out, err := runCLI(t, "skills", "remove", "paseka-release", "--dry-run", "-C", repo)
	if err != nil {
		t.Fatalf("skills remove --dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{
		`would remove "paseka-release"`,
		"store entry:",
		"ledger record: disabled at",
		"affected roles: builder",
		"would refuse",
		"dry run: nothing was changed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	after := pasekaFiles(t, repo)
	if !sameFiles(before, after) {
		t.Fatalf("dry run changed the tree:\nbefore:\n%v\nafter:\n%v", before, after)
	}

	// A forced preview prints the plan too, still without acting.
	out, err = runCLI(t, "skills", "remove", "paseka-release", "--dry-run", "--force", "-C", repo)
	if err != nil {
		t.Fatalf("skills remove --dry-run --force: %v\n%s", err, out)
	}
	if strings.Contains(out, "would refuse") {
		t.Fatalf("a forced preview must not report a refusal:\n%s", out)
	}
	if !sameFiles(before, pasekaFiles(t, repo)) {
		t.Fatal("a forced dry run changed the tree")
	}
}

// TestSkillsMutatingVerbsRefusePatterns pins that the three mutating verbs act
// on one procedure and refuse a pattern, leaving the tree alone.
func TestSkillsMutatingVerbsRefusePatterns(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-ship\ndescription: Run the ship procedure.")
	before := pasekaFiles(t, repo)

	cases := [][]string{
		{"skills", "disable", "paseka-*", "--reason", "why", "-C", repo},
		{"skills", "disable", "*", "--reason", "why", "-C", repo},
		{"skills", "enable", "paseka-*", "-C", repo},
		{"skills", "remove", "paseka-*", "-C", repo},
	}
	for _, args := range cases {
		out, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("%v was accepted:\n%s", args, out)
		}
		if !strings.Contains(err.Error(), "acts on one skill") {
			t.Fatalf("%v error = %v", args, err)
		}
	}
	if !sameFiles(before, pasekaFiles(t, repo)) {
		t.Fatal("a refused pattern changed the tree")
	}
}

// TestSkillsMutatingVerbsFailOnCorruptLedger pins that a corrupt ledger fails
// the command instead of being replaced, and nothing is deleted.
func TestSkillsMutatingVerbsFailOnCorruptLedger(t *testing.T) {
	repo := setupSkillsCLI(t)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	ledgerPath := filepath.Join(repo, ".paseka", "skills.index.json")
	if err := os.WriteFile(ledgerPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}

	cases := [][]string{
		{"skills", "disable", "paseka-release", "--reason", "why", "-C", repo},
		{"skills", "enable", "paseka-release", "-C", repo},
		{"skills", "remove", "paseka-release", "--force", "-C", repo},
	}
	for _, args := range cases {
		out, err := runCLI(t, args...)
		if err == nil {
			t.Fatalf("%v accepted a corrupt ledger:\n%s", args, out)
		}
		if !strings.Contains(err.Error(), "corrupt") {
			t.Fatalf("%v error = %v", args, err)
		}
	}
	after, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("a corrupt ledger was replaced: %q -> %q", before, after)
	}
	if _, err := os.Stat(filepath.Join(skills.Dir(repo), "paseka-release")); err != nil {
		t.Fatalf("a refused command deleted the store entry: %v", err)
	}
}
