package trailconfig_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func writeCandidateSkill(t *testing.T, root, name, frontmatter string) {
	t.Helper()
	writeFile(t, root, filepath.ToSlash(filepath.Join(".paseka", skills.DirName, name, skills.ManifestName)),
		"---\n"+frontmatter+"\n---\n# "+name+"\n")
}

func applyCandidate(t *testing.T, root, traceID string) (trailconfig.ApplyResult, error) {
	t.Helper()
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	// Every caller of this helper expects a candidate that actually differs from
	// the active snapshot, so the prompt moves after the binding is recorded.
	writeFile(t, root, ".paseka/prompts/builder.md", "candidate after binding\n")
	expected := manifest.ConfigRevision
	return trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: traceID, Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: &expected,
	})
}

// TestApplyRefusesInvalidCandidateSkillWithShellVerdict pins the candidate gate
// to the store scan the shell and doctor use: the refusal carries the same
// name-and-reason sentence.
func TestApplyRefusesInvalidCandidateSkillWithShellVerdict(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeCandidateSkill(t, root, "paseka-release", "name: paseka-ship\ndescription: Run the release procedure.")

	store, err := skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	issues := store.Issues()
	if len(issues) == 0 {
		t.Fatal("fixture store must be invalid")
	}
	manifest, err := trailconfig.Ensure(testContext(root), "trace-invalid-store")
	if err != nil {
		t.Fatal(err)
	}

	_, applyErr := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: testContext(root), TraceID: "trace-invalid-store", Source: trailconfig.SourceColony,
		Actor: "operator", ExpectedRevision: func() *int { r := manifest.ConfigRevision; return &r }(),
	})
	if applyErr == nil {
		t.Fatal("a candidate with an invalid skill must be refused")
	}
	for _, issue := range issues {
		if !strings.Contains(applyErr.Error(), issue.Error()) {
			t.Fatalf("candidate refusal %q does not carry the shell verdict %q", applyErr, issue.Error())
		}
	}
	assertBindingUnchanged(t, root, "trace-invalid-store", manifest)
}

// TestApplyRefusesUnresolvableCandidateSkillSelection covers every reference
// shape the shared resolver rejects: a missing name, a path, a tier prefix, and
// a family pattern that matches nothing.
func TestApplyRefusesUnresolvableCandidateSkillSelection(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want string
	}{
		{name: "missing", ref: "missing", want: `reference "missing" matches no skill`},
		{name: "path", ref: "./shared", want: "not a path"},
		{name: "tier", ref: "shared:foo", want: "tier prefixes are not supported"},
		{name: "unmatched pattern", ref: "paseka-*", want: `reference "paseka-*" matches no skill`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := writeColony(t, "slug: test\n")
			setupHome(t, "test")
			writeFile(t, root, ".paseka/bees/builder.yaml",
				"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - "+tc.ref+"\n")
			manifest, err := trailconfig.Ensure(testContext(root), "trace-ref")
			if err != nil {
				t.Fatal(err)
			}
			expected := manifest.ConfigRevision
			_, err = trailconfig.Apply(trailconfig.ApplyOptions{
				Context: testContext(root), TraceID: "trace-ref", Source: trailconfig.SourceColony,
				Actor: "operator", ExpectedRevision: &expected,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			assertBindingUnchanged(t, root, "trace-ref", manifest)
		})
	}
}

// TestApplyAcceptsWildcardOnEmptyCandidateStore pins the one pattern that may
// match nothing.
func TestApplyAcceptsWildcardOnEmptyCandidateStore(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeFile(t, root, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - \"*\"\n")
	writeFile(t, root, ".paseka/prompts/builder.md", "updated after binding\n")

	result, err := applyCandidate(t, root, "trace-wildcard")
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", result.Change.Outcome)
	}
}

// TestApplyReportsDisabledCandidateSkillWithoutRefusing pins that an operator's
// recorded disable is not fatal for a candidate, and that the roles still
// carrying the skill are named.
func TestApplyReportsDisabledCandidateSkillWithoutRefusing(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeCandidateSkill(t, root, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeFile(t, root, ".paseka/skills.index.json",
		`{"schema_version":1,"disabled":{"paseka-release":{"at":"2026-11-06T12:00:00Z","reason":"broken step"}}}`+"\n")
	writeFile(t, root, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeFile(t, root, ".paseka/prompts/builder.md", "updated after binding\n")

	result, err := applyCandidate(t, root, "trace-disabled")
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", result.Change.Outcome)
	}
	want := `skill "paseka-release" is disabled (broken step); skipped for bee(s): builder`
	if len(result.SkillWarnings) != 1 || result.SkillWarnings[0] != want {
		t.Fatalf("warnings = %v, want %q", result.SkillWarnings, want)
	}
}

// TestApplyRefusesCandidateSkillBudgetBreaches covers the role, file, and store
// bounds through the same resolver and scan that enforce them at dispatch.
func TestApplyRefusesCandidateSkillBudgetBreaches(t *testing.T) {
	t.Run("role budget", func(t *testing.T) {
		root := writeColony(t, "slug: test\n")
		setupHome(t, "test")
		for i := 0; i < skills.MaxSkillsPerRole+1; i++ {
			name := fmt.Sprintf("skill-%02d", i)
			writeCandidateSkill(t, root, name, "name: "+name+"\ndescription: A valid procedure.")
		}
		writeFile(t, root, ".paseka/bees/builder.yaml",
			"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - \"*\"\n")
		manifest, err := trailconfig.Ensure(testContext(root), "trace-role-budget")
		if err != nil {
			t.Fatal(err)
		}
		expected := manifest.ConfigRevision
		_, err = trailconfig.Apply(trailconfig.ApplyOptions{
			Context: testContext(root), TraceID: "trace-role-budget", Source: trailconfig.SourceColony,
			Actor: "operator", ExpectedRevision: &expected,
		})
		if err == nil || !strings.Contains(err.Error(), "limit is 32") {
			t.Fatalf("err = %v, want a role-budget refusal", err)
		}
		assertBindingUnchanged(t, root, "trace-role-budget", manifest)
	})

	t.Run("file budget", func(t *testing.T) {
		root := writeColony(t, "slug: test\n")
		setupHome(t, "test")
		writeCandidateSkill(t, root, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
		writeFile(t, root, ".paseka/skills/paseka-release/scripts/big.bin",
			strings.Repeat("x", skills.MaxSkillFileBytes+1))
		manifest, err := trailconfig.Ensure(testContext(root), "trace-file-budget")
		if err != nil {
			t.Fatal(err)
		}
		expected := manifest.ConfigRevision
		_, err = trailconfig.Apply(trailconfig.ApplyOptions{
			Context: testContext(root), TraceID: "trace-file-budget", Source: trailconfig.SourceColony,
			Actor: "operator", ExpectedRevision: &expected,
		})
		if err == nil || !strings.Contains(err.Error(), "may not exceed") {
			t.Fatalf("err = %v, want a file-budget refusal", err)
		}
		assertBindingUnchanged(t, root, "trace-file-budget", manifest)
	})

	t.Run("store budget", func(t *testing.T) {
		root := writeColony(t, "slug: test\n")
		setupHome(t, "test")
		// Each file stays under the 256 KiB file bound; together they cross the
		// 8 MiB store bound.
		for i := 0; i < skills.MaxSkillsPerRole+1; i++ {
			name := fmt.Sprintf("skill-%02d", i)
			writeCandidateSkill(t, root, name, "name: "+name+"\ndescription: A valid procedure.")
			writeFile(t, root, filepath.ToSlash(filepath.Join(".paseka", skills.DirName, name, "data.bin")),
				strings.Repeat("x", skills.MaxSkillFileBytes))
		}
		manifest, err := trailconfig.Ensure(testContext(root), "trace-store-budget")
		if err != nil {
			t.Fatal(err)
		}
		expected := manifest.ConfigRevision
		_, err = trailconfig.Apply(trailconfig.ApplyOptions{
			Context: testContext(root), TraceID: "trace-store-budget", Source: trailconfig.SourceColony,
			Actor: "operator", ExpectedRevision: &expected,
		})
		if err == nil || !strings.Contains(err.Error(), "store total is") {
			t.Fatalf("err = %v, want a store-budget refusal", err)
		}
		assertBindingUnchanged(t, root, "trace-store-budget", manifest)
	})
}

// TestApplyActivatesValidCandidateWithSkills proves the gate admits a correct
// store and selection instead of refusing everything.
func TestApplyActivatesValidCandidateWithSkills(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeCandidateSkill(t, root, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeFile(t, root, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - paseka-*\n")
	writeFile(t, root, ".paseka/prompts/builder.md", "updated after binding\n")

	result, err := applyCandidate(t, root, "trace-valid-skills")
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied {
		t.Fatalf("outcome = %q, want applied", result.Change.Outcome)
	}
	if result.Manifest.ConfigRevision != 2 {
		t.Fatalf("revision = %d, want 2", result.Manifest.ConfigRevision)
	}
	// The activated snapshot is the one dispatch will later resolve against.
	active := filepath.Join(root, ".paseka", "runs", "trace-valid-skills", "config", "snapshots",
		result.Manifest.ActiveConfigRef, "tree", ".paseka", "skills", "paseka-release", "SKILL.md")
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active snapshot is missing the skill: %v", err)
	}
}

// TestValidateProcedureMatchesDoctorSelectionForSameInput compares the
// candidate verdict with the resolver doctor uses for the same role selection.
func TestValidateProcedureMatchesDoctorSelectionForSameInput(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeCandidateSkill(t, root, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
	writeFile(t, root, ".paseka/bees/builder.yaml",
		"role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills:\n  - missing\n")

	store, err := skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := skills.LoadLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	doctor := skills.Resolve(store, ledger, []string{"missing"})
	if doctor.Valid() {
		t.Fatal("doctor's resolver must reject the fixture selection")
	}

	policy, err := colonyLoad(root)
	if err != nil {
		t.Fatal(err)
	}
	err = trailconfig.ValidateProcedure(root, policy, nil)
	if err == nil {
		t.Fatal("candidate validation must reject the same selection")
	}
	if !strings.Contains(err.Error(), doctor.Issues[0].Error()) {
		t.Fatalf("candidate verdict %q does not carry doctor's %q", err, doctor.Issues[0].Error())
	}
}

// colonyLoad keeps the comparison test readable.
func colonyLoad(root string) (colony.Colony, error) {
	return colony.LoadColony(root)
}
