package colony_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/colonyinit"
	"gopkg.in/yaml.v3"
)

func TestDefaultAutoInviteRulesValid(t *testing.T) {
	c := colony.Colony{AutoInvites: colony.DefaultAutoInviteRules()}
	if err := c.ValidateAutoInvites(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultGrillRuleDoneWhenValid(t *testing.T) {
	rules := colony.DefaultAutoInviteRules()
	if rules[0].Invite.DoneWhen == nil {
		t.Fatal("expected grill done_when")
	}
	if rules[0].Invite.DoneWhen.RequireFile.From != "ref" {
		t.Fatalf("require_file.from = %q", rules[0].Invite.DoneWhen.RequireFile.From)
	}
}

func TestInitScaffoldIncludesAutoInvites(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colonyinit.Init(colonyinit.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := colony.LoadColony(res.ColonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.AutoInvites) == 0 {
		t.Fatal("expected default auto_invites in scaffold")
	}
	if len(manifest.AutoInvites) < 2 {
		t.Fatalf("auto_invites = %d, want at least 2", len(manifest.AutoInvites))
	}
	if manifest.AutoInvites[0].When.Kind != "feature.classified" {
		t.Fatalf("when.kind = %q", manifest.AutoInvites[0].When.Kind)
	}
	if manifest.AutoInvites[1].When.Kind != "spec.ready" {
		t.Fatalf("second rule kind = %q", manifest.AutoInvites[1].When.Kind)
	}
	if manifest.AutoInvites[0].Invite.DoneWhen == nil {
		t.Fatal("expected grill done_when in scaffold")
	}
	if manifest.AutoInvites[0].Invite.DoneWhen.When.Kind != "spec.ready" {
		t.Fatalf("done_when kind = %q", manifest.AutoInvites[0].Invite.DoneWhen.When.Kind)
	}
}

func TestSampleAutoInviteRulesValid(t *testing.T) {
	c := colony.Colony{AutoInvites: colony.SampleAutoInviteRules()}
	if err := c.ValidateAutoInvites(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadColonyRejectsInvalidDoneWhen(t *testing.T) {
	dir := t.TempDir()
	colonyDir := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(colonyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := colony.Colony{
		Slug: "test",
		AutoInvites: []colony.AutoInviteRule{{
			When: colony.EventRule{Type: "SIGNAL", Kind: "feature.classified"},
			Invite: colony.AutoInviteInviteSpec{
				Bee:  colony.InviteStringField{Default: "drone"},
				Task: colony.InviteTaskField{Default: "ok"},
				DoneWhen: &colony.InviteDoneWhen{
					When: colony.EventRule{Type: "SIGNAL", Kind: "spec.ready"},
				},
			},
		}},
	}
	raw, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(colonyDir, "colony.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := colony.LoadColony(dir); err == nil {
		t.Fatal("expected validation error for missing done_when.require_file.from")
	}
}

func TestLoadColonyRejectsInvalidAutoInvite(t *testing.T) {
	dir := t.TempDir()
	colonyDir := filepath.Join(dir, ".paseka")
	if err := os.MkdirAll(colonyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := colony.Colony{
		Slug: "test",
		AutoInvites: []colony.AutoInviteRule{{
			When: colony.EventRule{Type: "SIGNAL", Kind: "feature.classified"},
			Invite: colony.AutoInviteInviteSpec{
				Task: colony.InviteTaskField{Default: "ok"},
			},
		}},
	}
	raw, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(colonyDir, "colony.yaml"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := colony.LoadColony(dir); err == nil {
		t.Fatal("expected validation error for missing invite.bee")
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
