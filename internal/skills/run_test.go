package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runFixture builds a minimal configuration tree for one role. It is the shape
// both a live colony root and a trail's frozen snapshot have: a `.paseka/`
// directory holding the manifest, the role, and the store.
func runFixture(t *testing.T, role, beeBody string) string {
	t.Helper()
	root := t.TempDir()
	writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: run-test\n")
	writeRunFixtureFile(t, root, filepath.Join(".paseka", "bees", role+".yaml"), beeBody)
	return root
}

func writeRunFixtureFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRunSkill(t *testing.T, root, name, description, body string) {
	t.Helper()
	writeRunFixtureFile(t, root, filepath.Join(".paseka", "skills", name, ManifestName),
		"---\nname: "+name+"\ndescription: "+description+"\n---\n"+body+"\n")
}

func resolveFixture(t *testing.T, root, role string) RunResolution {
	t.Helper()
	resolved, err := ResolveRun(RunInput{ConfigRoot: root, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestResolveRunSelection(t *testing.T) {
	t.Run("no list and no default receives nothing", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\n")
		writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
		resolved := resolveFixture(t, root, "builder")
		if len(resolved.Skills) != 0 {
			t.Fatalf("skills = %v", resolved.Names())
		}
		if section := resolved.Section(SectionContext{TraceID: "trace-t", AgentID: "agent-a"}); section != "" {
			t.Fatalf("a run with no skills must have no section: %q", section)
		}
	})

	t.Run("a declared list is delivered exactly", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-ship\n")
		writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
		writeRunSkill(t, root, "paseka-ship", "Ship procedure.", "# Ship")
		resolved := resolveFixture(t, root, "builder")
		if got := resolved.Names(); !equalStrings(got, []string{"paseka-ship"}) {
			t.Fatalf("skills = %v", got)
		}
	})

	t.Run("an empty list opts out of the colony default", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills: []\n")
		writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: run-test\ndefaults:\n  skills:\n    - paseka-release\n")
		writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
		resolved := resolveFixture(t, root, "builder")
		if len(resolved.Skills) != 0 {
			t.Fatalf("skills = %v", resolved.Names())
		}
		if !resolved.ExplicitEmpty {
			t.Fatal("an explicit empty list must be reported as the opt-out")
		}
	})

	t.Run("a role inherits the colony default", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\n")
		writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: run-test\ndefaults:\n  skills:\n    - paseka-release\n")
		writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
		resolved := resolveFixture(t, root, "builder")
		if got := resolved.Names(); !equalStrings(got, []string{"paseka-release"}) {
			t.Fatalf("skills = %v", got)
		}
		if resolved.Source != "defaults" {
			t.Fatalf("source = %q", resolved.Source)
		}
	})

	t.Run("wildcard expands to every valid enabled skill, sorted", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - '*'\n")
		writeRunSkill(t, root, "zeta", "Zeta procedure.", "# Zeta")
		writeRunSkill(t, root, "alpha", "Alpha procedure.", "# Alpha")
		writeRunSkill(t, root, "beta", "Beta procedure.", "# Beta")
		writeRunFixtureFile(t, root, ".paseka/skills.index.json",
			`{"schema_version":1,"disabled":{"beta":{"at":"2026-11-06T12:00:00Z","reason":"wrong step"}}}`)
		resolved := resolveFixture(t, root, "builder")
		if got := resolved.Names(); !equalStrings(got, []string{"alpha", "zeta"}) {
			t.Fatalf("skills = %v", got)
		}
		if len(resolved.Disabled) != 1 || resolved.Disabled[0].Name != "beta" {
			t.Fatalf("disabled = %+v", resolved.Disabled)
		}
	})
}

func TestResolveRunSection(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "paseka-release", "Run the release procedure.", "SECRET-BODY-TEXT")
	writeRunSkill(t, root, "paseka-ship", "Ship a version.", "OTHER-BODY")

	resolved := resolveFixture(t, root, "builder")
	section := resolved.Section(SectionContext{TraceID: "trace-t", AgentID: "agent-a"})

	if !strings.HasPrefix(section, SectionHeading) {
		t.Fatalf("section must open with its heading:\n%s", section)
	}
	for _, want := range []string{"paseka-release", "Run the release procedure.", resolved.Skills[0].Path} {
		if !strings.Contains(section, want) {
			t.Fatalf("section missing %q:\n%s", want, section)
		}
	}
	if !filepath.IsAbs(resolved.Skills[0].Path) {
		t.Fatalf("advertised path is not absolute: %q", resolved.Skills[0].Path)
	}
	if strings.Contains(section, "SECRET-BODY-TEXT") {
		t.Fatalf("the advertisement must not inline a body:\n%s", section)
	}
	if strings.Contains(section, "paseka-ship") {
		t.Fatalf("the advertisement names a skill the role was not offered:\n%s", section)
	}
}

func TestResolveRunAdapterSkills(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "paseka-release", "Run the release procedure.", "SECRET-BODY-TEXT")

	resolved := resolveFixture(t, root, "builder")
	refs := resolved.AdapterSkills()
	if len(refs) != 1 {
		t.Fatalf("adapter skills = %+v, want one entry", refs)
	}
	if refs[0].Name != "paseka-release" || refs[0].Description != "Run the release procedure." {
		t.Fatalf("adapter skill = %+v", refs[0])
	}
	if refs[0].Path != resolved.Skills[0].Path || !filepath.IsAbs(refs[0].Path) {
		t.Fatalf("adapter path = %q, want absolute %q", refs[0].Path, resolved.Skills[0].Path)
	}
}

func TestResolveRunFailsFast(t *testing.T) {
	cases := []struct {
		name    string
		bee     string
		prepare func(t *testing.T, root string)
		want    string
	}{
		{
			name: "missing reference",
			bee:  "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-ship\n",
			prepare: func(t *testing.T, root string) {
				writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
			},
			want: "paseka-ship",
		},
		{
			name: "unmatched pattern",
			bee:  "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - other-*\n",
			prepare: func(t *testing.T, root string) {
				writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
			},
			want: "other-*",
		},
		{
			name: "invalid skill under the wildcard",
			bee:  "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - '*'\n",
			prepare: func(t *testing.T, root string) {
				writeRunSkill(t, root, "good", "Good procedure.", "# Good")
				writeRunFixtureFile(t, root, ".paseka/skills/broken/SKILL.md",
					"---\nname: not-the-dir\ndescription: broken\n---\n# Broken\n")
			},
			want: "broken",
		},
		{
			name: "over-budget skill file",
			bee:  "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n",
			prepare: func(t *testing.T, root string) {
				writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
				writeRunFixtureFile(t, root, ".paseka/skills/paseka-release/scripts/big.bin",
					strings.Repeat("x", MaxSkillFileBytes+1))
			},
			want: "256 KiB",
		},
		{
			name: "over-budget store",
			bee:  "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n",
			prepare: func(t *testing.T, root string) {
				writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
				blob := strings.Repeat("y", MaxSkillFileBytes)
				for i := 0; i < 33; i++ {
					writeRunFixtureFile(t, root,
						filepath.Join(".paseka", "skills", "paseka-release", "assets", "blob-"+twoDigits(i)+".bin"), blob)
				}
			},
			want: "8 MiB",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := runFixture(t, "builder", tc.bee)
			tc.prepare(t, root)
			_, err := ResolveRun(RunInput{ConfigRoot: root, Role: "builder"})
			if err == nil {
				t.Fatal("an invalid set must stop the run")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not name %q", err, tc.want)
			}
		})
	}

	t.Run("33 skills breach the role budget", func(t *testing.T) {
		root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - '*'\n")
		for i := 0; i < MaxSkillsPerRole+1; i++ {
			writeRunSkill(t, root, skillName(i), "A procedure.", "# Procedure")
		}
		_, err := ResolveRun(RunInput{ConfigRoot: root, Role: "builder"})
		if err == nil || !strings.Contains(err.Error(), "limit is 32") {
			t.Fatalf("err = %v, want the role budget", err)
		}
	})
}

// freezeTree copies the live configuration into a frozen snapshot-shaped tree,
// which is what a trail binds a run to.
func freezeTree(t *testing.T, live string) string {
	t.Helper()
	frozen := t.TempDir()
	if err := os.CopyFS(frozen, os.DirFS(live)); err != nil {
		t.Fatal(err)
	}
	return frozen
}

func TestResolveRunIsPinnedToTheFrozenTree(t *testing.T) {
	live := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, live, "paseka-release", "Release procedure.", "# Release")

	frozen := freezeTree(t, live)
	before, err := ResolveRun(RunInput{ConfigRoot: frozen, Frozen: true, Role: "builder"})
	if err != nil {
		t.Fatal(err)
	}

	// A store edit after trail start must not reach the running trail.
	writeRunSkill(t, live, "paseka-release", "Changed procedure.", "# Changed")
	writeRunSkill(t, live, "paseka-extra", "Extra procedure.", "# Extra")

	after, err := ResolveRun(RunInput{ConfigRoot: frozen, Frozen: true, Role: "builder"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := after.Names(), before.Names(); !equalStrings(got, want) {
		t.Fatalf("frozen set changed: %v, want %v", got, want)
	}
	if after.Skills[0].Description != "Release procedure." {
		t.Fatalf("frozen description = %q", after.Skills[0].Description)
	}
	if !strings.HasPrefix(after.Skills[0].Path, frozen) {
		t.Fatalf("advertised path %q is not inside the frozen tree %q", after.Skills[0].Path, frozen)
	}

	// A frozen selection also ignores a later role-list change.
	writeRunFixtureFile(t, live, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-extra\n")
	still, err := ResolveRun(RunInput{ConfigRoot: frozen, Frozen: true, Role: "builder"})
	if err != nil {
		t.Fatal(err)
	}
	if got := still.Names(); !equalStrings(got, []string{"paseka-release"}) {
		t.Fatalf("frozen set followed a live role-list change: %v", got)
	}
}

func TestResolveRunFrozenProfileIsReadFromTheSnapshot(t *testing.T) {
	live := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\n")
	writeRunSkill(t, live, "paseka-release", "Release procedure.", "# Release")
	writeRunSkill(t, live, "docs-style", "Docs style.", "# Docs")

	// The frozen tree carries the profile that was selected at trail start.
	writeRunFixtureFile(t, live, ".paseka/profiles/lab.yaml", "skills:\n  - paseka-release\n")
	writeRunFixtureFile(t, live, "home/profiles/lab/config.yaml", "bees:\n  builder:\n    skills:\n      - docs-style\n")

	frozen := freezeTree(t, live)
	resolved, err := ResolveRun(RunInput{
		ConfigRoot: frozen, Frozen: true, Role: "builder", Profile: "lab",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := resolved.Names(); !equalStrings(got, []string{"docs-style"}) {
		t.Fatalf("skills = %v, want the home profile's capture", got)
	}
	if resolved.Source != "home profile" {
		t.Fatalf("source = %q", resolved.Source)
	}
}

func TestResolveRunCorruptFrozenInputBlocks(t *testing.T) {
	live := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, live, "paseka-release", "Release procedure.", "# Release")

	frozen := freezeTree(t, live)
	writeRunFixtureFile(t, frozen, ".paseka/skills.index.json", "{not json")

	_, err := ResolveRun(RunInput{ConfigRoot: frozen, Frozen: true, Role: "builder"})
	if err == nil {
		t.Fatal("a corrupt frozen ledger must block instead of falling back to the live store")
	}
	if !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("err = %v", err)
	}

	// The live store is still readable; the frozen failure must not be hidden by
	// resolving it instead.
	if _, err := ResolveRun(RunInput{ConfigRoot: live, Role: "builder"}); err != nil {
		t.Fatalf("live resolution = %v", err)
	}
}

func TestResolveRunMissingFrozenRoleBlocks(t *testing.T) {
	live := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\n")
	writeRunSkill(t, live, "paseka-release", "Release procedure.", "# Release")
	frozen := freezeTree(t, live)
	if err := os.Remove(filepath.Join(frozen, ".paseka", "bees", "builder.yaml")); err != nil {
		t.Fatal(err)
	}

	if _, err := ResolveRun(RunInput{ConfigRoot: frozen, Frozen: true, Role: "builder"}); err == nil {
		t.Fatal("a missing frozen role must block")
	}
}

func TestSectionCommandModeAdvertisesTheLoadCommandAndNoPath(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: run-test\ndefaults:\n  skills_delivery: command\n")
	writeRunSkill(t, root, "paseka-release", "Run the release procedure.", "# Release\n\nSECRET-BODY-TEXT")

	resolved := resolveFixture(t, root, "builder")
	if resolved.Delivery != "command" {
		t.Fatalf("delivery = %q, want command", resolved.Delivery)
	}
	ref := SectionContext{TraceID: "trace-t", AgentID: "agent-a"}
	section := resolved.Section(ref)

	want := LoadCommand("paseka-release", ref)
	if !strings.Contains(section, want) {
		t.Fatalf("section does not carry the load command %q:\n%s", want, section)
	}
	if !strings.Contains(section, "paseka-release") || !strings.Contains(section, "Run the release procedure.") {
		t.Fatalf("section is missing the skill metadata:\n%s", section)
	}
	if strings.Contains(section, resolved.Skills[0].Path) || strings.Contains(section, "path:") {
		t.Fatalf("command mode must not advertise the path:\n%s", section)
	}
	if strings.Contains(section, "SECRET-BODY-TEXT") {
		t.Fatalf("the advertisement must not inline a body:\n%s", section)
	}
}

func TestSectionPathModeIsUnchanged(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "paseka-release", "Run the release procedure.", "# Release")

	resolved := resolveFixture(t, root, "builder")
	section := resolved.Section(SectionContext{TraceID: "trace-t", AgentID: "agent-a"})
	if resolved.Delivery != "path" && resolved.Delivery != "" {
		t.Fatalf("delivery = %q, want path", resolved.Delivery)
	}
	if !strings.Contains(section, "  path: "+resolved.Skills[0].Path) {
		t.Fatalf("path mode must advertise the frozen path:\n%s", section)
	}
	if strings.Contains(section, "paseka skills load") {
		t.Fatalf("path mode must not advertise the load command:\n%s", section)
	}
}

func TestSectionCommandModeFollowsAFrozenProfileOverride(t *testing.T) {
	live := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, live, "paseka-release", "Release procedure.", "# Release")
	writeRunFixtureFile(t, live, "home/profiles/lab/config.yaml", "skills_delivery: command\n")

	frozen := freezeTree(t, live)
	resolved, err := ResolveRun(RunInput{
		ConfigRoot: frozen, Frozen: true, Role: "builder", Profile: "lab",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Delivery != "command" || resolved.DeliverySource != "home profile" {
		t.Fatalf("delivery = %q from %q, want the frozen profile's command mode", resolved.Delivery, resolved.DeliverySource)
	}
	if strings.Contains(resolved.Section(SectionContext{TraceID: "t", AgentID: "a"}), "path:") {
		t.Fatalf("a frozen profile override must reach the advertisement:\n%s", resolved.Section(SectionContext{TraceID: "t", AgentID: "a"}))
	}
}
