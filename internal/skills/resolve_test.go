package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBareNamePatternAndWildcard(t *testing.T) {
	dir := t.TempDir()
	writeResolveSkill(t, dir, "paseka-release")
	writeResolveSkill(t, dir, "paseka-ship")
	writeResolveSkill(t, dir, "docs-style")
	store := scanForTest(t, dir)

	t.Run("bare name", func(t *testing.T) {
		res := Resolve(store, Ledger{}, []string{"paseka-release"})
		if !res.Valid() {
			t.Fatalf("issues = %v", res.Issues)
		}
		if got := skillNames(res.Skills); !equalStrings(got, []string{"paseka-release"}) {
			t.Fatalf("skills = %v", got)
		}
	})

	t.Run("family pattern", func(t *testing.T) {
		res := Resolve(store, Ledger{}, []string{"paseka-*"})
		if !res.Valid() {
			t.Fatalf("issues = %v", res.Issues)
		}
		if got := skillNames(res.Skills); !equalStrings(got, []string{"paseka-release", "paseka-ship"}) {
			t.Fatalf("skills = %v", got)
		}
	})

	t.Run("wildcard is every skill, sorted", func(t *testing.T) {
		res := Resolve(store, Ledger{}, []string{"*"})
		if !res.Valid() {
			t.Fatalf("issues = %v", res.Issues)
		}
		if got := skillNames(res.Skills); !equalStrings(got, []string{"docs-style", "paseka-release", "paseka-ship"}) {
			t.Fatalf("skills = %v", got)
		}
	})

	t.Run("overlapping references are deduplicated", func(t *testing.T) {
		res := Resolve(store, Ledger{}, []string{"*", "paseka-*", "docs-style"})
		if !res.Valid() {
			t.Fatalf("issues = %v", res.Issues)
		}
		if got := skillNames(res.Skills); !equalStrings(got, []string{"docs-style", "paseka-release", "paseka-ship"}) {
			t.Fatalf("skills = %v", got)
		}
	})
}

func TestResolveWildcardOnEmptyStoreIsAllowed(t *testing.T) {
	store := scanForTest(t, t.TempDir())
	res := Resolve(store, Ledger{}, []string{"*"})
	if !res.Valid() {
		t.Fatalf("a wildcard on an empty store must be allowed: %v", res.Issues)
	}
	if len(res.Skills) != 0 {
		t.Fatalf("skills = %v", res.Skills)
	}
}

func TestResolveMissingReferenceAndUnmatchedPatternAreErrors(t *testing.T) {
	dir := t.TempDir()
	writeResolveSkill(t, dir, "paseka-release")
	store := scanForTest(t, dir)

	for _, ref := range []string{"paseka-ship", "other-*"} {
		t.Run(ref, func(t *testing.T) {
			res := Resolve(store, Ledger{}, []string{ref})
			if res.Valid() {
				t.Fatalf("reference %q must not resolve silently", ref)
			}
			if !strings.Contains(res.Issues[0].Error(), "matches no skill") {
				t.Fatalf("issues = %v", res.Issues)
			}
		})
	}
}

func TestResolveInvalidSkillIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeResolveSkill(t, dir, "good")
	writeInvalidResolveSkill(t, dir, "broken")
	store := scanForTest(t, dir)

	// A bare reference to the valid skill does not inherit the unrelated
	// invalid entry.
	res := Resolve(store, Ledger{}, []string{"good"})
	if !res.Valid() {
		t.Fatalf("an unrelated invalid skill must not block a bare reference: %v", res.Issues)
	}

	// The wildcard declares the whole store, so the invalid entry is fatal.
	res = Resolve(store, Ledger{}, []string{"*"})
	if res.Valid() {
		t.Fatal("the wildcard must fail on an invalid skill")
	}
	found := false
	for _, issue := range res.Issues {
		if issue.Skill == "broken" {
			found = true
		}
	}
	if !found {
		t.Fatalf("issues = %v, want one naming the invalid skill", res.Issues)
	}
}

func TestResolveDisabledSkillIsSkippedAndReported(t *testing.T) {
	dir := t.TempDir()
	writeResolveSkill(t, dir, "paseka-release")
	writeResolveSkill(t, dir, "docs-style")
	store := scanForTest(t, dir)
	ledger := Ledger{
		SchemaVersion: LedgerSchemaVersion,
		Disabled: map[string]DisabledRecord{
			"paseka-release": {At: "2026-11-06T12:00:00Z", Reason: "broken step"},
		},
	}

	res := Resolve(store, ledger, []string{"*"})
	if !res.Valid() {
		t.Fatalf("a disabled skill is not a configuration error: %v", res.Issues)
	}
	if got := skillNames(res.Skills); !equalStrings(got, []string{"docs-style"}) {
		t.Fatalf("skills = %v", got)
	}
	if len(res.Disabled) != 1 || res.Disabled[0].Name != "paseka-release" || res.Disabled[0].Reason != "broken step" {
		t.Fatalf("disabled = %+v", res.Disabled)
	}

	// A role that names a disabled skill is not an error either: the disable is
	// the operator's recorded decision.
	res = Resolve(store, ledger, []string{"paseka-release"})
	if !res.Valid() {
		t.Fatalf("a bare reference to a disabled skill must not fail: %v", res.Issues)
	}
	if len(res.Skills) != 0 || len(res.Disabled) != 1 {
		t.Fatalf("resolution = %+v", res)
	}
}

func TestResolveRoleBudget(t *testing.T) {
	t.Run("32 is allowed", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < MaxSkillsPerRole; i++ {
			writeResolveSkill(t, dir, skillName(i))
		}
		res := Resolve(scanForTest(t, dir), Ledger{}, []string{"*"})
		if !res.Valid() {
			t.Fatalf("issues = %v", res.Issues)
		}
		if len(res.Skills) != MaxSkillsPerRole {
			t.Fatalf("skills = %d", len(res.Skills))
		}
	})

	t.Run("33 is an error", func(t *testing.T) {
		dir := t.TempDir()
		for i := 0; i < MaxSkillsPerRole+1; i++ {
			writeResolveSkill(t, dir, skillName(i))
		}
		res := Resolve(scanForTest(t, dir), Ledger{}, []string{"*"})
		if res.Valid() {
			t.Fatal("33 skills must breach the role budget")
		}
		if !strings.Contains(res.Issues[0].Error(), "limit is 32") {
			t.Fatalf("issues = %v", res.Issues)
		}
	})
}

func TestResolveInvalidReference(t *testing.T) {
	dir := t.TempDir()
	writeResolveSkill(t, dir, "good")
	store := scanForTest(t, dir)

	for _, ref := range []string{"shared:good", "./good", "go?d"} {
		t.Run(ref, func(t *testing.T) {
			res := Resolve(store, Ledger{}, []string{ref})
			if res.Valid() {
				t.Fatalf("reference %q must be refused", ref)
			}
		})
	}
}

func writeResolveSkill(t *testing.T, dir, name string) {
	t.Helper()
	writeResolveSkillFile(t, dir, name, "name: "+name+"\ndescription: A valid skill.")
}

func writeInvalidResolveSkill(t *testing.T, dir, name string) {
	t.Helper()
	writeResolveSkillFile(t, dir, name, "name: not-the-dir\ndescription: A broken skill.")
}

func writeResolveSkillFile(t *testing.T, dir, name, frontmatter string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\n" + frontmatter + "\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(path, ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func scanForTest(t *testing.T, dir string) Report {
	t.Helper()
	report, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func skillNames(skills []Skill) []string {
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		out = append(out, skill.Name)
	}
	return out
}

func skillName(i int) string {
	return "skill-" + twoDigits(i)
}

func twoDigits(i int) string {
	if i < 10 {
		return "0" + string(rune('0'+i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
