package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// setupSkillsTrialCLI adds one script bee and an ignored run directory to the
// skills fixture, then binds a destination trail so a trial has an active
// snapshot to build from.
func setupSkillsTrialCLI(t *testing.T) string {
	t.Helper()
	repo := setupSkillsCLI(t)
	bee := "role: worker\nadapter: script\ncommand: [echo]\n"
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "bees", "worker.yaml"), []byte(bee), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".paseka/runs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runStatusGit(t, repo, "add", ".paseka", ".gitignore")
	runStatusGit(t, repo, "commit", "-m", "trial fixture")
	if _, err := trailconfig.EnsureForRoot(repo, "trace-target"); err != nil {
		t.Fatal(err)
	}
	return repo
}

func trialSnapshotTree(t *testing.T, repo string) string {
	t.Helper()
	manifest, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(trailconfig.SnapshotDir(repo, "trace-target"), manifest.ActiveConfigRef, "tree")
}

func TestSkillsApproveTrialWritesOneConfigurationRevision(t *testing.T) {
	repo := setupSkillsTrialCLI(t)
	liveBee := filepath.Join(repo, ".paseka", "bees", "worker.yaml")
	beeBefore, err := os.ReadFile(liveBee)
	if err != nil {
		t.Fatal(err)
	}
	addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	baseline, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "approve", "trace-alpha/paseka-release",
		"--into", "trace-target", "--role", "worker", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve --into: %v\n%s", err, out)
	}
	for _, want := range []string{
		"trial applied trace-target: revision 2",
		"added .paseka/skills/paseka-release/SKILL.md",
		"changed .paseka/bees/worker.yaml",
		"the store and the working tree are unchanged",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	manifest, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != baseline.ConfigRevision+1 {
		t.Fatalf("revision = %d, want %d", manifest.ConfigRevision, baseline.ConfigRevision+1)
	}
	if len(manifest.Snapshots) != len(baseline.Snapshots)+1 {
		t.Fatalf("snapshots = %d, want one new", len(manifest.Snapshots))
	}
	tree := trialSnapshotTree(t, repo)
	if _, err := os.Stat(filepath.Join(tree, ".paseka", "skills", "paseka-release", skills.ManifestName)); err != nil {
		t.Fatalf("candidate store missing the trialled skill: %v", err)
	}

	// The store and the live working tree are untouched.
	if _, err := os.Stat(skills.Dir(repo)); !os.IsNotExist(err) {
		t.Fatalf("trial wrote the live store (stat err = %v)", err)
	}
	beeAfter, err := os.ReadFile(liveBee)
	if err != nil {
		t.Fatal(err)
	}
	if string(beeAfter) != string(beeBefore) {
		t.Fatal("trial changed the live bee definition")
	}

	// A run dispatched under the trail receives the trialled skill from its
	// frozen snapshot.
	resolved, err := skills.ResolveRun(skills.RunInput{ConfigRoot: tree, Frozen: true, Role: "worker"})
	if err != nil {
		t.Fatal(err)
	}
	if names := resolved.Names(); len(names) != 1 || names[0] != "paseka-release" {
		t.Fatalf("candidate role resolves %v", names)
	}

	lines := decisionLines(t, repo, "trace-alpha")
	if len(lines) != 1 {
		t.Fatalf("decisions = %d, want one trialled:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var decision skills.ProposalDecision
	if err := json.Unmarshal([]byte(lines[0]), &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Decision != skills.ProposalTrialled || decision.Destination != "trace-target" ||
		len(decision.Roles) != 1 || decision.Roles[0] != "worker" ||
		decision.ProcedureHash == "" || decision.At.IsZero() {
		t.Fatalf("decision = %+v", decision)
	}

	// The proposal stays pending for the store; a later approval installs it and
	// leaves the trial record in place.
	out, err = runCLI(t, "skills", "proposals", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals: %v\n%s", err, out)
	}
	if row := proposalRows(t, out)["paseka-release"]; len(row) == 0 || row[3] != "trialled: trace-target" {
		t.Fatalf("proposal state = %q\n%s", row, out)
	}
	if out, err = runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "-C", repo); err != nil {
		t.Fatalf("later store approve: %v\n%s", err, out)
	}
	lines = decisionLines(t, repo, "trace-alpha")
	if len(lines) != 2 {
		t.Fatalf("decisions after store approval = %d, want trial plus install:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var installed bool
	for _, line := range lines {
		var got skills.ProposalDecision
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatal(err)
		}
		if got.Decision == skills.ProposalTrialled && got.Destination == "trace-target" {
			continue
		}
		if got.Decision == skills.ProposalInstalled && got.Destination == skills.ProposalStoreDestination {
			installed = true
		}
	}
	if !installed {
		t.Fatalf("store approval did not record installed:\n%s", strings.Join(lines, "\n"))
	}
	out, err = runCLI(t, "skills", "proposals", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals after install: %v\n%s", err, out)
	}
	if row := proposalRows(t, out)["paseka-release"]; len(row) == 0 || row[3] != "installed" {
		t.Fatalf("proposal state after install = %q\n%s", row, out)
	}
}

func TestSkillsApproveTrialDryRunAndBatch(t *testing.T) {
	repo := setupSkillsTrialCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "alpha-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-alpha", "beta-skill", cliProposalTime.Add(time.Minute))
	baseline, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "approve", "trace-alpha/alpha-skill", "trace-alpha/beta-skill",
		"--into", "trace-target", "--role", "worker", "--dry-run", "-C", repo)
	if err != nil {
		t.Fatalf("dry-run trial: %v\n%s", err, out)
	}
	if !strings.Contains(out, "trial would apply trace-target: revision 2") ||
		!strings.Contains(out, "dry run: nothing was written") {
		t.Fatalf("dry-run output:\n%s", out)
	}
	afterDry, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}
	if afterDry.ConfigRevision != baseline.ConfigRevision || len(afterDry.Snapshots) != len(baseline.Snapshots) {
		t.Fatal("dry run changed the trail binding")
	}
	if lines := decisionLines(t, repo, "trace-alpha"); len(lines) != 0 {
		t.Fatalf("dry run wrote decisions: %v", lines)
	}

	out, err = runCLI(t, "skills", "approve", "trace-alpha/alpha-skill", "trace-alpha/beta-skill",
		"--into", "trace-target", "--role", "worker", "-C", repo)
	if err != nil {
		t.Fatalf("batch trial: %v\n%s", err, out)
	}
	manifest, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != 2 {
		t.Fatalf("batch revision = %d, want one new revision", manifest.ConfigRevision)
	}
	tree := trialSnapshotTree(t, repo)
	for _, name := range []string{"alpha-skill", "beta-skill"} {
		if _, err := os.Stat(filepath.Join(tree, ".paseka", "skills", name, skills.ManifestName)); err != nil {
			t.Fatalf("batch candidate missing %s: %v", name, err)
		}
	}
	if lines := decisionLines(t, repo, "trace-alpha"); len(lines) != 2 {
		t.Fatalf("batch decisions = %d, want one per proposal", len(lines))
	}
}

func TestSkillsApproveTrialSequentialAndRefusals(t *testing.T) {
	repo := setupSkillsTrialCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "alpha-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-alpha", "beta-skill", cliProposalTime.Add(time.Minute))

	if out, err := runCLI(t, "skills", "approve", "trace-alpha/alpha-skill",
		"--into", "trace-target", "-C", repo); err == nil || !strings.Contains(err.Error(), "--role is required") {
		t.Fatalf("trial without --role = %v\n%s", err, out)
	}

	if out, err := runCLI(t, "skills", "approve", "trace-alpha/alpha-skill",
		"--into", "trace-target", "--role", "worker", "-C", repo); err != nil {
		t.Fatalf("first trial: %v\n%s", err, out)
	}
	if out, err := runCLI(t, "skills", "approve", "trace-alpha/beta-skill",
		"--into", "trace-target", "--role", "worker", "-C", repo); err != nil {
		t.Fatalf("second trial after the first applied: %v\n%s", err, out)
	}
	manifest, err := trailconfig.Load(repo, "trace-target")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigRevision != 3 {
		t.Fatalf("sequential trials revision = %d, want 3", manifest.ConfigRevision)
	}
	tree := trialSnapshotTree(t, repo)
	for _, name := range []string{"alpha-skill", "beta-skill"} {
		if _, err := os.Stat(filepath.Join(tree, ".paseka", "skills", name, skills.ManifestName)); err != nil {
			t.Fatalf("final candidate missing %s: %v", name, err)
		}
	}

	// A refused proposal cannot be trialled.
	addCLIProposal(t, repo, "trace-alpha", "refused-skill", cliProposalTime.Add(2*time.Minute))
	refusedDir := filepath.Join(repo, ".paseka", "runs", "trace-alpha", "artifacts", "skill-proposals", "refused-skill")
	refusedHash, err := skills.ProcedureHash(refusedDir)
	if err != nil {
		t.Fatal(err)
	}
	addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
		Skill: "refused-skill", Trail: "trace-alpha", ProcedureHash: refusedHash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalRefused,
		Reason: "not a procedure", At: cliProposalTime.Add(time.Hour),
	})
	if out, err := runCLI(t, "skills", "approve", "trace-alpha/refused-skill",
		"--into", "trace-target", "--role", "worker", "-C", repo); err == nil || !strings.Contains(err.Error(), "already refused") {
		t.Fatalf("refused proposal trial = %v\n%s", err, out)
	}
}
