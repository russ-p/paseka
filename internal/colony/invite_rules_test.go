package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

func TestSampleAutoInviteRulesValid(t *testing.T) {
	c := colony.Colony{AutoInvites: colony.SampleAutoInviteRules()}
	if err := c.ValidateAutoInvites(); err != nil {
		t.Fatal(err)
	}
}

func TestSampleRuleDoneWhenValid(t *testing.T) {
	rules := colony.SampleAutoInviteRules()
	if rules[0].Invite.DoneWhen == nil {
		t.Fatal("expected sample done_when")
	}
	if rules[0].Invite.DoneWhen.RequireFile.From != "ref" {
		t.Fatalf("require_file.from = %q", rules[0].Invite.DoneWhen.RequireFile.From)
	}
}

func TestInitScaffoldOmitsAutoInvites(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := colony.LoadColony(res.ColonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.AutoInvites) != 0 {
		t.Fatalf("auto_invites = %d, want empty scaffold", len(manifest.AutoInvites))
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
			When: colony.EventRule{Type: "SIGNAL", Kind: "review.needed"},
			Invite: colony.AutoInviteInviteSpec{
				Bee:  colony.InviteStringField{Default: "drone"},
				Task: colony.InviteTaskField{Default: "ok"},
				DoneWhen: &colony.InviteDoneWhen{
					When: colony.EventRule{Type: "SIGNAL", Kind: "doc.ready"},
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
			When: colony.EventRule{Type: "SIGNAL", Kind: "review.needed"},
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
