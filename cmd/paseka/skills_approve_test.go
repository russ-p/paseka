package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
)

// addCLIProposalBundle writes a bundle with arbitrary files and publishes the
// discovery after the bytes exist, so the recorded hash is the bundle's real
// procedure hash. addCLIProposal covers the single-file case; this covers a
// procedure with supporting content and the invalid fixtures.
func addCLIProposalBundle(t *testing.T, repo, traceID, name string, files map[string]string, at time.Time) (string, string) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent(traceID, "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Run the " + name + " procedure.",
		Reason:        "Two runs repeated this by hand.",
		Bundle:        "skill-proposals/" + name,
		ProcedureHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	if err := (runs.Trail{ColonyRoot: repo, TraceID: traceID}).AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	return dir, hash
}

func validProposalManifest(name string) string {
	return "---\nname: " + name + "\ndescription: Run the " + name + " procedure.\n---\n# " + name + "\n\n1. Do the thing.\n"
}

// TestSkillsApproveInstallsProposalAndReportsWorkingTree is the command-level
// happy path: the bundle's files land in the store with provenance, the source
// trail records one installed decision, the proposal reads installed, and the
// command names every working-tree path and leaves the commit to the Beekeeper.
func TestSkillsApproveInstallsProposalAndReportsWorkingTree(t *testing.T) {
	repo := setupSkillsCLI(t)
	dir, hash := addCLIProposalBundle(t, repo, "trace-alpha", "paseka-release", map[string]string{
		skills.ManifestName: validProposalManifest("paseka-release"),
		"scripts/run.sh":    "echo release\n",
	}, cliProposalTime)

	out, err := runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve: %v\n%s", err, out)
	}
	for _, want := range []string{
		"installed trace-alpha/paseka-release",
		"procedure: " + hash,
		"provenance: source=proposal trail=trace-alpha role=agent-archivist",
		".paseka/skills/paseka-release/SKILL.md",
		".paseka/skills/paseka-release/scripts/run.sh",
		"untracked",
		"Committing is the Beekeeper's decision",
		"this command never commits",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	report, err := skills.ScanColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skills) != 1 {
		t.Fatalf("store has %d skills, want the installed one: %+v", len(report.Skills), report.Skills)
	}
	installed := report.Skills[0]
	if !installed.Valid() {
		t.Fatalf("installed skill is invalid: %v", installed.Issues)
	}
	if installed.Metadata[skills.MetadataSource] != skills.ProvenanceProposal ||
		installed.Metadata[skills.MetadataTrail] != "trace-alpha" ||
		installed.Metadata[skills.MetadataRole] != "agent-archivist" ||
		installed.Metadata[skills.MetadataApprovedAt] == "" ||
		installed.Metadata[skills.MetadataProcedureHash] != hash {
		t.Fatalf("provenance = %+v", installed.Metadata)
	}
	if _, err := os.Stat(filepath.Join(installed.Dir, "scripts", "run.sh")); err != nil {
		t.Fatalf("supporting file was not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts", "run.sh")); err != nil {
		t.Fatalf("the proposal bundle was moved rather than copied: %v", err)
	}

	lines := decisionLines(t, repo, "trace-alpha")
	if len(lines) != 1 {
		t.Fatalf("decision lines = %d, want one:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var decision skills.ProposalDecision
	if err := json.Unmarshal([]byte(lines[0]), &decision); err != nil {
		t.Fatalf("decision line is not JSON: %v\n%s", err, lines[0])
	}
	if decision.Skill != "paseka-release" || decision.Trail != "trace-alpha" ||
		decision.Destination != skills.ProposalStoreDestination ||
		decision.Decision != skills.ProposalInstalled ||
		decision.ProcedureHash != hash || decision.At.IsZero() {
		t.Fatalf("decision = %+v", decision)
	}

	listOut, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, `"name": "paseka-release"`) || !strings.Contains(listOut, `"provenance": "proposal"`) {
		t.Fatalf("installed skill is not listed with proposal provenance:\n%s", listOut)
	}

	out, err = runCLI(t, "skills", "proposals", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals: %v\n%s", err, out)
	}
	if row := proposalRows(t, out)["paseka-release"]; len(row) == 0 || row[3] != "installed" {
		t.Fatalf("proposal state = %v, want installed:\n%s", row, out)
	}
}

// TestSkillsApproveDryRunPrintsPlanAndChangesNothing pins that the preview
// validates the same batch and leaves the store, the ledger, the decision file,
// and the decision lock byte-identical.
func TestSkillsApproveDryRunPrintsPlanAndChangesNothing(t *testing.T) {
	repo := setupSkillsCLI(t)
	_, hash := addCLIProposalBundle(t, repo, "trace-alpha", "paseka-release", map[string]string{
		skills.ManifestName: validProposalManifest("paseka-release"),
	}, cliProposalTime)
	writeStoreSkill(t, repo, "docs-style", "name: docs-style\ndescription: Write documentation.")
	before := treeDigest(t, repo)

	out, err := runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "--dry-run", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve --dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{
		"would install trace-alpha/paseka-release",
		"procedure: " + hash,
		".paseka/skills/paseka-release/SKILL.md",
		`"decision":"installed"`,
		"dry run: nothing was written",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
	if after := treeDigest(t, repo); after != before {
		t.Fatal("--dry-run changed the repository")
	}
	if _, err := os.Stat(skills.ProposalDecisionsPath(repo, "trace-alpha")); !os.IsNotExist(err) {
		t.Fatalf("--dry-run created the decision file: %v", err)
	}
	if _, err := os.Stat(skills.ProposalDecisionsLockPath(repo, "trace-alpha")); !os.IsNotExist(err) {
		t.Fatalf("--dry-run created the decision lock: %v", err)
	}
}

// TestSkillsApproveRefusesExistingAndForceKeepsDisable pins the overwrite rule:
// an existing entry needs --force, and a disable record survives the replacement
// so the new bytes remain inert.
func TestSkillsApproveRefusesExistingAndForceKeepsDisable(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run by hand.")

	out, err := runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "-C", repo)
	if err == nil {
		t.Fatalf("an existing skill must be refused without --force:\n%s", out)
	}
	if !strings.Contains(err.Error(), "already holds") {
		t.Fatalf("error = %v", err)
	}
	report, err := skills.ScanColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skills) != 1 || report.Skills[0].Metadata[skills.MetadataSource] == skills.ProvenanceProposal {
		t.Fatalf("a refused replacement changed the store: %+v", report.Skills)
	}

	writeSkillLedger(t, repo, `{"paseka-release": {"at": "2026-11-06T12:00:00Z", "reason": "step two is wrong"}}`)
	out, err = runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "--force", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve --force: %v\n%s", err, out)
	}
	for _, want := range []string{
		"replaced trace-alpha/paseka-release",
		"state: disabled (step two is wrong)",
		"remains disabled",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("forced replacement output missing %q:\n%s", want, out)
		}
	}
	report, err = skills.ScanColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skills) != 1 || report.Skills[0].Metadata[skills.MetadataSource] != skills.ProvenanceProposal {
		t.Fatalf("forced replacement did not install provenance: %+v", report.Skills)
	}

	ledger, err := skills.LoadLedger(repo)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := ledger.Record("paseka-release")
	if !ok || record.Reason != "step two is wrong" {
		t.Fatalf("forced replacement cleared the disable record: %+v", ledger)
	}
	listOut, err := runCLI(t, "skills", "list", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills list: %v\n%s", err, listOut)
	}
	if !strings.Contains(listOut, `"state": "disabled"`) || !strings.Contains(listOut, "step two is wrong") {
		t.Fatalf("replaced skill is not listed as disabled:\n%s", listOut)
	}
}

// TestSkillsApproveBatchWritesNothingWhenOneReferenceIsBad pins the atomicity
// the command promises: one invalid reference cancels every good one.
func TestSkillsApproveBatchWritesNothingWhenOneReferenceIsBad(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "good-skill", cliProposalTime)
	addCLIProposalBundle(t, repo, "trace-alpha", "bad-skill", map[string]string{
		skills.ManifestName: "---\nname: a-different-name\ndescription: The wrong name.\n---\n# body\n",
	}, cliProposalTime)

	out, err := runCLI(t, "skills", "approve",
		"trace-alpha/good-skill", "trace-alpha/bad-skill", "-C", repo)
	if err == nil {
		t.Fatalf("one invalid reference must fail the batch:\n%s", out)
	}
	if !strings.Contains(err.Error(), "cannot be approved") {
		t.Fatalf("error = %v", err)
	}

	report, err := skills.ScanColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skills) != 0 {
		t.Fatalf("the batch wrote the good skill: %+v", report.Skills)
	}
	if lines := decisionLines(t, repo, "trace-alpha"); len(lines) != 0 {
		t.Fatalf("the batch wrote decisions: %v", lines)
	}
}

// TestSkillsApproveResolvesBareAndAmbiguousReferences pins the reference rules:
// a bare name needs --trail when two trails proposed it, and with --trail it
// installs only that trail's proposal.
func TestSkillsApproveResolvesBareAndAmbiguousReferences(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	addCLIProposal(t, repo, "trace-beta", "paseka-release", cliProposalTime.Add(time.Hour))

	out, err := runCLI(t, "skills", "approve", "paseka-release", "-C", repo)
	if err == nil {
		t.Fatalf("an ambiguous bare name must be refused:\n%s", out)
	}
	for _, want := range []string{"trace-alpha", "trace-beta", "trace-alpha/paseka-release", "trace-beta/paseka-release"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguity error %q must name %q", err, want)
		}
	}
	if report, _ := skills.ScanColony(repo); len(report.Skills) != 0 {
		t.Fatalf("an ambiguous reference installed something: %+v", report.Skills)
	}

	out, err = runCLI(t, "skills", "approve", "paseka-release", "--trail", "trace-alpha", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve --trail: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed trace-alpha/paseka-release") {
		t.Fatalf("output = %q", out)
	}
	if lines := decisionLines(t, repo, "trace-alpha"); len(lines) != 1 {
		t.Fatalf("source trail decisions = %d, want one", len(lines))
	}
	if lines := decisionLines(t, repo, "trace-beta"); len(lines) != 0 {
		t.Fatalf("the other trail's decisions = %v, want none", lines)
	}
}

// TestSkillsApproveBatchRecordsOneDecisionPerSourceTrail pins the multi-trail
// batch: both skills install, and each installed decision is written to the
// trail that proposed that skill.
func TestSkillsApproveBatchRecordsOneDecisionPerSourceTrail(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "alpha-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-beta", "beta-skill", cliProposalTime.Add(time.Hour))

	out, err := runCLI(t, "skills", "approve",
		"trace-alpha/alpha-skill", "trace-beta/beta-skill", "-C", repo)
	if err != nil {
		t.Fatalf("skills approve batch: %v\n%s", err, out)
	}
	report, err := skills.ScanColony(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Skills) != 2 {
		t.Fatalf("store has %d skills, want both:\n%s", len(report.Skills), out)
	}

	for trail, skill := range map[string]string{"trace-alpha": "alpha-skill", "trace-beta": "beta-skill"} {
		lines := decisionLines(t, repo, trail)
		if len(lines) != 1 {
			t.Fatalf("%s decisions = %d, want one:\n%s", trail, len(lines), strings.Join(lines, "\n"))
		}
		var decision skills.ProposalDecision
		if err := json.Unmarshal([]byte(lines[0]), &decision); err != nil {
			t.Fatalf("%s decision line: %v", trail, err)
		}
		if decision.Trail != trail || decision.Skill != skill || decision.Decision != skills.ProposalInstalled {
			t.Fatalf("%s decision = %+v", trail, decision)
		}
	}
}

// TestDoctorReportsInstalledProcedureHashMismatch pins the installed procedure's
// identity: a hand edit after approval is a doctor error, while a hand-authored
// skill without a recorded hash is not accused of one.
func TestDoctorReportsInstalledProcedureHashMismatch(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	if out, err := runCLI(t, "skills", "approve", "trace-alpha/paseka-release", "-C", repo); err != nil {
		t.Fatalf("skills approve: %v\n%s", err, out)
	}
	writeStoreSkill(t, repo, "hand-written", "name: hand-written\ndescription: Written by hand.")

	manifest := filepath.Join(skills.Dir(repo), "paseka-release", skills.ManifestName)
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifest, append(data, []byte("\nAn extra step the Beekeeper never approved.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _ := runCLI(t, "doctor", "-C", repo)
	if !strings.Contains(out, `skill "paseka-release": procedure hash mismatch`) {
		t.Fatalf("doctor did not report the edited install:\n%s", out)
	}
	if strings.Contains(out, `skill "hand-written": procedure hash mismatch`) {
		t.Fatalf("doctor accused a hand-authored skill without a recorded hash:\n%s", out)
	}
}
