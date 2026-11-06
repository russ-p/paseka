package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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
