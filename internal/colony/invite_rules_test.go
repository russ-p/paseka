package colony_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

func TestDefaultAutoInviteRulesValid(t *testing.T) {
	c := colony.Colony{AutoInvites: colony.DefaultAutoInviteRules()}
	if err := c.ValidateAutoInvites(); err != nil {
		t.Fatal(err)
	}
}

func TestInitScaffoldIncludesAutoInvites(t *testing.T) {
	repo := initTestRepo(t)
	res, err := colony.Init(colony.InitOptions{StartDir: repo})
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
