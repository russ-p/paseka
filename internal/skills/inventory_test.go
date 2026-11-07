package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

// inventoryFixture builds a colony whose roles cover the three selection
// forms: a wildcard, a family pattern, and a bare name.
func inventoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: inventory-test\n")
	writeRunFixtureFile(t, root, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - '*'\n")
	writeRunFixtureFile(t, root, ".paseka/bees/docs.yaml",
		"role: docs\nadapter: script\ncommand: [echo]\nskills:\n  - 'paseka-*'\n")
	writeRunFixtureFile(t, root, ".paseka/bees/release.yaml",
		"role: release\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "zeta", "Zeta procedure.", "# Zeta")
	writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
	writeRunSkill(t, root, "paseka-ship", "Ship procedure.", "# Ship")
	return root
}

func entryNamed(t *testing.T, inventory Inventory, name string) InventoryEntry {
	t.Helper()
	for _, entry := range inventory.Skills {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("no inventory entry %q in %+v", name, inventory.Skills)
	return InventoryEntry{}
}

// TestBuildInventoryExpandsWildcardAndPatternRoles pins the listing's central
// promise: a role is named against every skill its selection resolves to, so a
// wildcard or a pattern never appears as `*` in the roles column.
func TestBuildInventoryExpandsWildcardAndPatternRoles(t *testing.T) {
	root := inventoryFixture(t)
	writeRunFixtureFile(t, root, ".paseka/skills.index.json",
		`{"schema_version":1,"disabled":{"paseka-ship":{"at":"2026-11-06T12:00:00Z","reason":"wrong step"}}}`)

	inventory, err := BuildInventory(colony.Context{ColonyRoot: root})
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}

	zeta := entryNamed(t, inventory, "zeta")
	if !equalStrings(zeta.Roles, []string{"builder"}) {
		t.Fatalf("zeta roles = %v, want the wildcard role expanded", zeta.Roles)
	}
	release := entryNamed(t, inventory, "paseka-release")
	if !equalStrings(release.Roles, []string{"builder", "docs", "release"}) {
		t.Fatalf("paseka-release roles = %v", release.Roles)
	}
	ship := entryNamed(t, inventory, "paseka-ship")
	if !equalStrings(ship.Roles, []string{"builder", "docs"}) {
		t.Fatalf("paseka-ship roles = %v", ship.Roles)
	}
	for _, entry := range inventory.Skills {
		for _, role := range entry.Roles {
			if strings.Contains(role, "*") {
				t.Fatalf("roles column contains a wildcard or pattern: %+v", entry)
			}
		}
	}

	if ship.State != StateDisabled || ship.Reason != "wrong step" {
		t.Fatalf("disabled row = %+v", ship)
	}
	if zeta.State != StateOK || zeta.Description != "Zeta procedure." {
		t.Fatalf("ok row = %+v", zeta)
	}
	if zeta.Provenance != ProvenanceHand {
		t.Fatalf("provenance = %q, want %q", zeta.Provenance, ProvenanceHand)
	}
}

func TestBuildInventoryReportsInvalidSkillWithItsReason(t *testing.T) {
	root := inventoryFixture(t)
	// A directory under the store is always a skill; one without a SKILL.md is
	// an invalid row that must stay visible with the reason.
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "skills", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}

	inventory, err := BuildInventory(colony.Context{ColonyRoot: root})
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}

	broken := entryNamed(t, inventory, "broken")
	if broken.State != StateInvalid {
		t.Fatalf("state = %q, want invalid", broken.State)
	}
	if !strings.Contains(broken.Reason, "SKILL.md is missing") {
		t.Fatalf("reason = %q, want the missing manifest", broken.Reason)
	}
}

func TestBuildInventoryReadsProvenanceFromSkillMetadata(t *testing.T) {
	root := inventoryFixture(t)
	writeRunFixtureFile(t, root, ".paseka/skills/paseka-release/SKILL.md",
		"---\nname: paseka-release\ndescription: Release procedure.\nmetadata:\n  source: proposal\n---\n# Release\n")

	inventory, err := BuildInventory(colony.Context{ColonyRoot: root})
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}
	if got := entryNamed(t, inventory, "paseka-release").Provenance; got != ProvenanceProposal {
		t.Fatalf("provenance = %q, want %q", got, ProvenanceProposal)
	}
}

func TestBuildInventoryOfAMissingStoreIsEmptyRatherThanBroken(t *testing.T) {
	root := t.TempDir()
	writeRunFixtureFile(t, root, ".paseka/colony.yaml", "slug: inventory-test\n")

	inventory, err := BuildInventory(colony.Context{ColonyRoot: root})
	if err != nil {
		t.Fatalf("build inventory: %v", err)
	}
	if len(inventory.Skills) != 0 || len(inventory.Issues) != 0 {
		t.Fatalf("inventory = %+v, want an empty store", inventory)
	}
}
