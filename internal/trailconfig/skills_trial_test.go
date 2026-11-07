package trailconfig_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func trialBundle(t *testing.T, name string) trailconfig.SkillTrialBundle {
	t.Helper()
	dir := t.TempDir()
	content := "---\nname: " + name + "\ndescription: Run the " + name + " procedure.\n---\n# " + name + "\n\n1. Do the trial thing.\n"
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	return trailconfig.SkillTrialBundle{
		Name:          name,
		Files:         []trailconfig.SkillTrialFile{{Rel: skills.ManifestName, Data: []byte(content)}},
		ProcedureHash: hash,
	}
}

func trialCandidateRoot(t *testing.T, root, traceID string) string {
	t.Helper()
	manifest, err := trailconfig.Load(root, traceID)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(trailconfig.SnapshotDir(root, traceID), manifest.ActiveConfigRef, "tree")
}

func TestSkillTrialAddsBundleAndRoleSelectionInOneRevision(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-trial")
	if err != nil {
		t.Fatal(err)
	}

	liveBee, err := os.ReadFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	expected := manifest.ConfigRevision
	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-trial", Source: trailconfig.SourceSkillTrial,
		Actor: "operator", ExpectedRevision: &expected, RequireImmediate: true,
		SkillTrial: &trailconfig.SkillTrial{
			Bundles: []trailconfig.SkillTrialBundle{trialBundle(t, "trial-skill")},
			Roles:   []string{"builder"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Change.Outcome != trailconfig.OutcomeApplied || result.Manifest.ConfigRevision != 2 {
		t.Fatalf("trial result = outcome %q revision %d", result.Change.Outcome, result.Manifest.ConfigRevision)
	}
	if len(result.Diff) == 0 || !containsLine(result.Diff, "added .paseka/skills/trial-skill/SKILL.md") {
		t.Fatalf("diff = %v", result.Diff)
	}

	tree := trialCandidateRoot(t, root, "trace-trial")
	if _, err := os.Stat(filepath.Join(tree, ".paseka", "skills", "trial-skill", skills.ManifestName)); err != nil {
		t.Fatalf("candidate store is missing the trial bundle: %v", err)
	}
	if _, err := os.Stat(skills.Dir(root)); !os.IsNotExist(err) {
		t.Fatalf("trial wrote the live store (stat err = %v)", err)
	}
	liveAfter, err := os.ReadFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(liveAfter) != string(liveBee) {
		t.Fatal("trial changed the live bee definition")
	}

	resolved, err := skills.ResolveRun(skills.RunInput{
		ConfigRoot: tree, Frozen: true, Role: "builder", Profile: "pi",
	})
	if err != nil {
		t.Fatal(err)
	}
	if names := resolved.Names(); len(names) != 1 || names[0] != "trial-skill" {
		t.Fatalf("candidate role resolves %v, want the trialled skill", names)
	}

	candidateBee, err := os.ReadFile(filepath.Join(tree, ".paseka", "bees", "builder.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(candidateBee), "trial-skill") {
		t.Fatalf("candidate bee does not list the skill:\n%s", candidateBee)
	}
}

func TestSkillTrialRoleSelectionLayers(t *testing.T) {
	cases := []struct {
		name      string
		beeSkills string
		colony    string
		want      []string
		unchanged bool
	}{
		{
			name:   "inherited default gets resolved default plus skill",
			colony: "slug: test\ndefaults:\n  skills: [base-skill]\n",
			want:   []string{"base-skill", "trial-skill"},
		},
		{
			name:      "explicit list gets skill appended",
			beeSkills: "[base-skill]\n",
			want:      []string{"base-skill", "trial-skill"},
		},
		{
			name:      "explicit empty becomes one skill",
			beeSkills: "[]\n",
			want:      []string{"trial-skill"},
		},
		{
			name:      "wildcard is not narrowed",
			beeSkills: "[\"*\"]\n",
			want:      []string{"*"},
			unchanged: true,
		},
		{
			name:      "matching pattern is not narrowed",
			beeSkills: "[trial-*]\n",
			want:      []string{"trial-*"},
			unchanged: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manifest := tc.colony
			if manifest == "" {
				manifest = "slug: test\n"
			}
			root := writeColony(t, manifest)
			setupHome(t, "test")
			if tc.beeSkills != "" {
				bee := "role: builder\nadapter: script\nprompt_template: builder.md\ncommand: [echo]\nskills: " + tc.beeSkills
				if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"), []byte(bee), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if containsString(tc.want, "base-skill") {
				writeStoreSkillForTest(t, root, "base-skill")
			}
			ctx := testContext(root)
			baseline, err := trailconfig.Ensure(ctx, "trace-roles")
			if err != nil {
				t.Fatal(err)
			}
			expected := baseline.ConfigRevision
			result, err := trailconfig.Apply(trailconfig.ApplyOptions{
				Context: ctx, TraceID: "trace-roles", Source: trailconfig.SourceSkillTrial,
				Actor: "operator", ExpectedRevision: &expected, RequireImmediate: true,
				SkillTrial: &trailconfig.SkillTrial{
					Bundles: []trailconfig.SkillTrialBundle{trialBundle(t, "trial-skill")},
					Roles:   []string{"builder"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Change.Outcome != trailconfig.OutcomeApplied {
				t.Fatalf("outcome = %q", result.Change.Outcome)
			}
			tree := trialCandidateRoot(t, root, "trace-roles")
			candidateBee, err := os.ReadFile(filepath.Join(tree, ".paseka", "bees", "builder.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, ref := range tc.want {
				if !strings.Contains(string(candidateBee), ref) {
					t.Fatalf("candidate bee missing %q:\n%s", ref, candidateBee)
				}
			}
			if tc.unchanged && strings.Contains(string(candidateBee), "trial-skill,") {
				t.Fatalf("candidate bee was narrowed or appended to:\n%s", candidateBee)
			}
			resolved, err := skills.ResolveRun(skills.RunInput{ConfigRoot: tree, Frozen: true, Role: "builder", Profile: "pi"})
			if err != nil {
				t.Fatal(err)
			}
			if !containsString(resolved.Names(), "trial-skill") {
				t.Fatalf("candidate role does not receive the trialled skill: %v", resolved.Names())
			}
		})
	}
}

func TestSkillTrialRefusesPendingBlockerAndInvalidCandidate(t *testing.T) {
	root := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	ctx := testContext(root)
	manifest, err := trailconfig.Ensure(ctx, "trace-refuse")
	if err != nil {
		t.Fatal(err)
	}
	expected := manifest.ConfigRevision
	opts := trailconfig.ApplyOptions{
		Context: ctx, TraceID: "trace-refuse", Source: trailconfig.SourceSkillTrial,
		Actor: "operator", ExpectedRevision: &expected, RequireImmediate: true,
		SkillTrial: &trailconfig.SkillTrial{
			Bundles: []trailconfig.SkillTrialBundle{trialBundle(t, "trial-skill")},
			Roles:   []string{"builder"},
		},
	}

	// A pending change of another kind blocks the trial without recording it.
	first := opts
	first.Source = trailconfig.SourceOverrides
	first.SkillTrial = nil
	first.Overrides = []trailconfig.Override{{Role: "builder", Key: "timeout", Value: "12m"}}
	first.Blockers = []string{"task-1 is running"}
	first.RequireImmediate = false
	pending, err := trailconfig.Apply(first)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Change.Outcome != trailconfig.OutcomePending {
		t.Fatalf("setup outcome = %q", pending.Change.Outcome)
	}
	if _, err := trailconfig.Apply(opts); err == nil || !strings.Contains(err.Error(), "another configuration change is pending") {
		t.Fatalf("trial with a pending change err = %v", err)
	}
	reloaded, err := trailconfig.Load(root, "trace-refuse")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConfigRevision != manifest.ConfigRevision {
		t.Fatalf("refused trial advanced to revision %d", reloaded.ConfigRevision)
	}
	if _, err := trailconfig.Cancel(root, "trace-refuse", "", "operator", "clear pending"); err != nil {
		t.Fatal(err)
	}

	// An execution blocker refuses an immediate trial.
	writeRemainingTask(t, root, "trace-refuse", taskSnapshotRunning("task-1"))
	if _, err := trailconfig.Apply(opts); err == nil || !strings.Contains(err.Error(), "cannot activate immediately") {
		t.Fatalf("trial with a blocker err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(trailconfig.SnapshotDir(root, "trace-refuse"), "sha256-does-not-exist")); err == nil {
		t.Fatal("unexpected snapshot")
	}

	// An invalid candidate is refused before a revision or snapshot is recorded.
	root2 := writeColony(t, "slug: test\n")
	setupHome(t, "test")
	writeStoreSkillForTest(t, root2, "base-skill")
	broken := "role: broken\nadapter: script\ncommand: [echo]\nskills: [missing-skill]\n"
	if err := os.WriteFile(filepath.Join(root2, ".paseka", "bees", "broken.yaml"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx2 := testContext(root2)
	baseline, err := trailconfig.Ensure(ctx2, "trace-invalid")
	if err != nil {
		t.Fatal(err)
	}
	expectedBaseline := baseline.ConfigRevision
	if _, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context: ctx2, TraceID: "trace-invalid", Source: trailconfig.SourceSkillTrial,
		Actor: "operator", ExpectedRevision: &expectedBaseline, RequireImmediate: true,
		SkillTrial: &trailconfig.SkillTrial{
			Bundles: []trailconfig.SkillTrialBundle{trialBundle(t, "trial-skill")},
			Roles:   []string{"builder"},
		},
	}); err == nil || !strings.Contains(err.Error(), "missing-skill") {
		t.Fatalf("invalid candidate err = %v", err)
	}
	after, err := trailconfig.Load(root2, "trace-invalid")
	if err != nil {
		t.Fatal(err)
	}
	if after.ConfigRevision != baseline.ConfigRevision || len(after.Snapshots) != len(baseline.Snapshots) {
		t.Fatalf("invalid candidate changed the binding: %+v", after)
	}
}

func writeStoreSkillForTest(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(skills.Dir(root), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Run the " + name + " procedure.\n---\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsLine(lines []string, want string) bool {
	for _, line := range lines {
		if line == want {
			return true
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func taskSnapshotRunning(taskID string) taskledger.TaskSnapshot {
	return taskledger.TaskSnapshot{TaskID: taskID, Title: "implement", Bee: "builder", Status: protocol.TaskStatusRunning}
}
