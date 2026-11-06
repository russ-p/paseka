package colony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

// TestEffectiveSkillsTriState pins the presence-aware field: absent inherits
// the colony default, an explicit empty list opts out, and a non-empty list is
// the whole set for the role.
func TestEffectiveSkillsTriState(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeManifest(t, repo, slug, "defaults:\n  skills:\n    - paseka-release\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "scout.yaml"),
		"role: scout\nadapter: cursor\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "builder.yaml"),
		"role: builder\nadapter: cursor\nskills: []\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "custom.yaml"),
		"role: custom\nadapter: cursor\nskills:\n  - docs-style\n")

	resetProcessProfile(t)
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("absent inherits the default", func(t *testing.T) {
		bee, _, err := ctx.LoadBee("scout")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ctx.EffectiveSkills("scout", bee)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Present || got.Source != "defaults" {
			t.Fatalf("selection = %+v, want the committed default", got)
		}
		if !equalStrings(got.Refs, []string{"paseka-release"}) {
			t.Fatalf("refs = %v", got.Refs)
		}
	})

	t.Run("empty opts out", func(t *testing.T) {
		bee, _, err := ctx.LoadBee("builder")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ctx.EffectiveSkills("builder", bee)
		if err != nil {
			t.Fatal(err)
		}
		if !got.Present || !got.ExplicitEmpty() || got.Source != "bee" {
			t.Fatalf("selection = %+v, want the bee's explicit empty list", got)
		}
		if len(got.Refs) != 0 {
			t.Fatalf("refs = %v, want none", got.Refs)
		}
	})

	t.Run("non-empty replaces", func(t *testing.T) {
		bee, _, err := ctx.LoadBee("custom")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ctx.EffectiveSkills("custom", bee)
		if err != nil {
			t.Fatal(err)
		}
		if !equalStrings(got.Refs, []string{"docs-style"}) {
			t.Fatalf("refs = %v, want the bee's whole list", got.Refs)
		}
	})
}

// TestEffectiveSkillsProfilePrecedence pins home profile > colony profile >
// committed for the default list, a role's list, and the delivery mode.
func TestEffectiveSkillsProfilePrecedence(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeManifest(t, repo, slug, `defaults:
  skills:
    - committed-default
  skills_delivery: command
`)
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "scout.yaml"),
		"role: scout\nadapter: cursor\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "builder.yaml"),
		"role: builder\nadapter: cursor\nskills:\n  - committed-role\n")
	writeColonyProfile(t, repo, "lab", `skills:
  - colony-default
skills_delivery: command
bees:
  builder:
    skills:
      - colony-role
`)
	writeHomeProfileConfig(t, slug, "lab", `skills:
  - home-default
skills_delivery: path
bees:
  builder:
    skills:
      - home-role
`)

	selection := func(t *testing.T, role string) colony.EffectiveSkills {
		t.Helper()
		ctx, err := colony.ResolveContext(repo)
		if err != nil {
			t.Fatal(err)
		}
		bee, _, err := ctx.LoadBee(role)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ctx.EffectiveSkills(role, bee)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "lab", FlagSet: true})

	home := selection(t, "scout")
	if !equalStrings(home.Refs, []string{"home-default"}) || home.Source != "home profile defaults" {
		t.Fatalf("scout = %+v, want the home profile default", home)
	}
	if home.Delivery != colony.SkillsDeliveryPath || home.DeliverySource != "home profile" {
		t.Fatalf("delivery = %q from %q, want path from the home profile", home.Delivery, home.DeliverySource)
	}
	role := selection(t, "builder")
	if !equalStrings(role.Refs, []string{"home-role"}) || role.Source != "home profile" {
		t.Fatalf("builder = %+v, want the home profile role list", role)
	}

	// Drop the home profile's skill fields; the colony profile must take over.
	writeHomeProfileConfig(t, slug, "lab", "nats:\n  url: nats://lab:4222\n")
	colonyOnly := selection(t, "scout")
	if !equalStrings(colonyOnly.Refs, []string{"colony-default"}) || colonyOnly.Source != "colony profile defaults" {
		t.Fatalf("scout = %+v, want the colony profile default", colonyOnly)
	}
	if colonyOnly.Delivery != colony.SkillsDeliveryCommand || colonyOnly.DeliverySource != "colony profile" {
		t.Fatalf("delivery = %q from %q, want command from the colony profile", colonyOnly.Delivery, colonyOnly.DeliverySource)
	}
	if got := selection(t, "builder"); !equalStrings(got.Refs, []string{"colony-role"}) {
		t.Fatalf("builder = %+v, want the colony profile role list", got)
	}

	// No profile: the committed values stand.
	resetProcessProfile(t)
	committed := selection(t, "scout")
	if !equalStrings(committed.Refs, []string{"committed-default"}) || committed.Source != "defaults" {
		t.Fatalf("scout = %+v, want the committed default", committed)
	}
	if committed.Delivery != colony.SkillsDeliveryCommand || committed.DeliverySource != "defaults" {
		t.Fatalf("delivery = %q from %q, want command from defaults", committed.Delivery, committed.DeliverySource)
	}
	if got := selection(t, "builder"); !equalStrings(got.Refs, []string{"committed-role"}) || got.Source != "bee" {
		t.Fatalf("builder = %+v, want the committed role list", got)
	}
}

// TestProfileSkillsEmptyOptsOut pins the tri-state in a profile: an empty list
// in a profile is an explicit opt-out, not an absent key.
func TestProfileSkillsEmptyOptsOut(t *testing.T) {
	repo, slug := setupProfileColony(t)
	writeManifest(t, repo, slug, "defaults:\n  skills:\n    - paseka-release\n")
	mustWriteProfile(t, filepath.Join(repo, ".paseka", "bees", "scout.yaml"),
		"role: scout\nadapter: cursor\n")
	writeColonyProfile(t, repo, "off", "skills: []\n")

	resetProcessProfile(t)
	colony.SetProcessProfile(colony.ProfileSelection{Name: "off", FlagSet: true})
	ctx, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	bee, _, err := ctx.LoadBee("scout")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ctx.EffectiveSkills("scout", bee)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ExplicitEmpty() || got.Source != "colony profile defaults" {
		t.Fatalf("selection = %+v, want an explicit empty colony profile default", got)
	}
}

// TestSkillsInBeeLocalOverlayFailsLoad pins that a misplaced `skills` key is a
// load error naming the file and the key, not a silent no-op.
func TestSkillsInBeeLocalOverlayFailsLoad(t *testing.T) {
	repo, _ := setupProfileColony(t)
	local := filepath.Join(repo, ".paseka", "bees", "scout.local.yaml")
	if err := os.WriteFile(local, []byte("skills:\n  - paseka-release\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := colony.LoadBee(repo, "scout")
	if err == nil {
		t.Fatal("expected a load error for a skills key in the bee local overlay")
	}
	for _, want := range []string{local, `"skills"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want it to name %q", err, want)
		}
	}

	// A prompt-only overlay still loads.
	if err := os.WriteFile(local, []byte("prompt_template: other.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := colony.LoadBee(repo, "scout"); err != nil {
		t.Fatalf("prompt-only overlay should load: %v", err)
	}
}

func TestSkillsDeliveryValidation(t *testing.T) {
	t.Run("manifest accepts path and command", func(t *testing.T) {
		for _, value := range []string{"path", "command"} {
			repo, slug := setupProfileColony(t)
			writeManifest(t, repo, slug, "defaults:\n  skills_delivery: "+value+"\n")
			if _, err := colony.ResolveContext(repo); err != nil {
				t.Fatalf("delivery %q: %v", value, err)
			}
		}
	})

	t.Run("manifest rejects anything else", func(t *testing.T) {
		repo, slug := setupProfileColony(t)
		writeManifest(t, repo, slug, "defaults:\n  skills_delivery: both\n")
		_, err := colony.ResolveContext(repo)
		if err == nil || !strings.Contains(err.Error(), "skills_delivery") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("colony profile rejects anything else", func(t *testing.T) {
		repo, _ := setupProfileColony(t)
		writeColonyProfile(t, repo, "bad", "skills_delivery: both\n")
		resetProcessProfile(t)
		colony.SetProcessProfile(colony.ProfileSelection{Name: "bad", FlagSet: true})
		_, err := colony.ResolveContext(repo)
		if err == nil || !strings.Contains(err.Error(), "skills_delivery") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("home profile rejects anything else", func(t *testing.T) {
		repo, slug := setupProfileColony(t)
		writeHomeProfileConfig(t, slug, "bad", "skills_delivery: both\n")
		resetProcessProfile(t)
		colony.SetProcessProfile(colony.ProfileSelection{Name: "bad", FlagSet: true})
		_, err := colony.ResolveContext(repo)
		if err == nil || !strings.Contains(err.Error(), "skills_delivery") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("absent means path", func(t *testing.T) {
		repo, slug := setupProfileColony(t)
		writeManifest(t, repo, slug, "")
		resetProcessProfile(t)
		ctx, err := colony.ResolveContext(repo)
		if err != nil {
			t.Fatal(err)
		}
		bee, _, err := ctx.LoadBee("scout")
		if err != nil {
			t.Fatal(err)
		}
		got, err := ctx.EffectiveSkills("scout", bee)
		if err != nil {
			t.Fatal(err)
		}
		if got.Delivery != colony.SkillsDeliveryPath {
			t.Fatalf("delivery = %q, want path", got.Delivery)
		}
	})
}

func writeManifest(t *testing.T, repo, slug, body string) {
	t.Helper()
	if !strings.HasPrefix(body, "slug:") {
		body = "slug: " + slug + "\n" + body
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSkillSelectionYAMLRoundTrip pins that the presence-aware field survives a
// config rewrite: absent stays omitted, empty stays an explicit empty list, and
// a list stays a list rather than a `present:`/`refs:` mapping.
func TestSkillSelectionYAMLRoundTrip(t *testing.T) {
	type doc struct {
		Skills colony.SkillSelection `yaml:"skills,omitempty"`
	}
	cases := []struct {
		name  string
		in    doc
		want  string
		empty bool
	}{
		{name: "absent", in: doc{}, want: "{}\n"},
		{name: "empty", in: doc{Skills: colony.SkillSelection{Present: true}}, want: "skills: []\n", empty: true},
		{name: "list", in: doc{Skills: colony.SkillSelection{Present: true, Refs: []string{"a", "b"}}}, want: "skills:\n    - a\n    - b\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := yaml.Marshal(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.want {
				t.Fatalf("marshal = %q, want %q", data, tc.want)
			}
			var back doc
			if err := yaml.Unmarshal(data, &back); err != nil {
				t.Fatal(err)
			}
			if back.Skills.Present != (tc.name != "absent") || back.Skills.Empty() != tc.empty {
				t.Fatalf("round trip = %+v", back.Skills)
			}
		})
	}
}
