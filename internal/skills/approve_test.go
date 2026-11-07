package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
)

// approvedAt is the deterministic approval time the tests record.
var approvedAt = time.Date(2026, 11, 7, 11, 0, 0, 0, time.UTC)

func storeSkill(t *testing.T, root, name string) skills.Skill {
	t.Helper()
	report, err := skills.ScanColony(root)
	if err != nil {
		t.Fatalf("scan store: %v", err)
	}
	for _, skill := range report.Skills {
		if skill.Name == name {
			return skill
		}
	}
	t.Fatalf("store does not hold %q: %+v", name, report.Skills)
	return skills.Skill{}
}

func proposalDecisions(t *testing.T, root, traceID string) []skills.ProposalDecision {
	t.Helper()
	decisions, issues, err := skills.ReadProposalDecisions(
		skills.ProposalDecisionsPath(root, traceID), traceID,
	)
	if err != nil {
		t.Fatalf("read decisions: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("decision issues: %v", issues)
	}
	return decisions
}

// TestApproveProposalInstallsBundleAndProvenance is the happy path: the bundle's
// SKILL.md and supporting files land in the store, the installed skill passes
// the shared rule set, provenance names the trail and hash, and one installed
// decision is appended to the source trail.
func TestApproveProposalInstallsBundleAndProvenance(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	name := "paseka-release"

	dir := writeProposalBundle(t, root, traceID, name,
		frontmatter(name)+"\nmetadata:\n  owner: release-team\n  source: hand",
		"# Release\n\n1. Run scripts/run.sh.\n")
	writeFile(t, filepath.Join(dir, "scripts", "run.sh"), []byte("echo hi\n"))
	appendProposalEvent(t, root, traceID, name, bundleHash(t, dir), proposalFixtureTime)

	result, err := skills.ApproveProposals(root, []string{traceID + "/" + name}, "", false, approvedAt, false)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("approvals = %d, want 1", len(result))
	}
	if !result[0].Decision.At.Equal(approvedAt) {
		t.Fatalf("decision at = %s, want %s", result[0].Decision.At, approvedAt)
	}
	if len(result[0].Files) != 2 {
		t.Fatalf("files = %v, want SKILL.md and the script", result[0].Files)
	}

	installed := storeSkill(t, root, name)
	if !installed.Valid() {
		t.Fatalf("installed skill is invalid: %v", installed.Issues)
	}
	for key, want := range map[string]string{
		skills.MetadataSource:     "proposal",
		skills.MetadataTrail:      traceID,
		skills.MetadataRole:       "agent-archivist",
		skills.MetadataApprovedAt: approvedAt.Format(time.RFC3339),
	} {
		if got := installed.Metadata[key]; got != want {
			t.Fatalf("metadata[%s] = %q, want %q", key, got, want)
		}
	}
	if got := installed.Metadata["owner"]; got != "release-team" {
		t.Fatalf("provenance replacement dropped other metadata: %+v", installed.Metadata)
	}
	hash, err := skills.ProcedureHash(installed.Dir)
	if err != nil {
		t.Fatalf("hash installed skill: %v", err)
	}
	if installed.Metadata[skills.MetadataProcedureHash] != hash {
		t.Fatalf("installed procedure hash = %q, store hashes to %s",
			installed.Metadata[skills.MetadataProcedureHash], hash)
	}
	if hash != bundleHash(t, dir) {
		t.Fatalf("installed procedure differs from the discovery bundle")
	}
	if _, err := os.Stat(filepath.Join(installed.Dir, "scripts", "run.sh")); err != nil {
		t.Fatalf("supporting file was not installed: %v", err)
	}

	decisions := proposalDecisions(t, root, traceID)
	if len(decisions) != 1 {
		t.Fatalf("decisions = %+v, want one", decisions)
	}
	decision := decisions[0]
	if decision.Decision != skills.ProposalInstalled ||
		decision.Skill != name ||
		decision.Trail != traceID ||
		decision.Destination != skills.ProposalStoreDestination ||
		decision.ProcedureHash != hash ||
		decision.At.IsZero() {
		t.Fatalf("decision = %+v", decision)
	}
}

// TestApproveProposalBatchIsAtomic pins that one bad reference writes none of
// the good ones, for each way a reference can be bad.
func TestApproveProposalBatchIsAtomic(t *testing.T) {
	traceID := "trace-alpha"

	t.Run("invalid bundle", func(t *testing.T) {
		root := t.TempDir()
		addProposal(t, root, traceID, "good-skill", proposalFixtureTime)
		badDir := writeProposalBundle(t, root, traceID, "bad-skill",
			"name: some-other-name\ndescription: The wrong name.", "# body\n")
		appendProposalEvent(t, root, traceID, "bad-skill", bundleHash(t, badDir), proposalFixtureTime)

		_, err := skills.ApproveProposals(root, []string{traceID + "/good-skill", traceID + "/bad-skill"},
			"", false, approvedAt, false)
		if err == nil {
			t.Fatal("an invalid bundle must fail the batch")
		}
		if !strings.Contains(err.Error(), "cannot be approved") {
			t.Fatalf("error = %v", err)
		}
		assertStoreEmpty(t, root)
		if got := proposalDecisions(t, root, traceID); len(got) != 0 {
			t.Fatalf("batch wrote decisions: %+v", got)
		}
	})

	t.Run("hash mismatch", func(t *testing.T) {
		root := t.TempDir()
		addProposal(t, root, traceID, "good-skill", proposalFixtureTime)
		writeProposalBundle(t, root, traceID, "stale-skill", frontmatter("stale-skill"), "# body\n")
		appendProposalEvent(t, root, traceID, "stale-skill", "0000", proposalFixtureTime)

		_, err := skills.ApproveProposals(root, []string{traceID + "/good-skill", traceID + "/stale-skill"},
			"", false, approvedAt, false)
		if err == nil {
			t.Fatal("a hash mismatch must fail the batch")
		}
		assertStoreEmpty(t, root)
	})

	t.Run("refused bundle", func(t *testing.T) {
		root := t.TempDir()
		dir := addProposal(t, root, traceID, "refused-skill", proposalFixtureTime)
		appendDecision(t, root, traceID, skills.ProposalDecision{
			Skill: "refused-skill", Trail: traceID, ProcedureHash: bundleHash(t, dir),
			Destination: skills.ProposalStoreDestination, Decision: skills.ProposalRefused,
			Reason: "not a procedure", At: proposalFixtureTime.Add(time.Hour),
		})

		_, err := skills.ApproveProposals(root, []string{traceID + "/refused-skill"},
			"", false, approvedAt, false)
		if err == nil {
			t.Fatal("a refused proposal must not be approved")
		}
		if !strings.Contains(err.Error(), "already refused") {
			t.Fatalf("error = %v", err)
		}
		assertStoreEmpty(t, root)
	})

	t.Run("missing bundle", func(t *testing.T) {
		root := t.TempDir()
		appendProposalEvent(t, root, traceID, "gone-skill", "abc123", proposalFixtureTime)

		_, err := skills.ApproveProposals(root, []string{traceID + "/gone-skill"},
			"", false, approvedAt, false)
		if err == nil {
			t.Fatal("a missing bundle must not be approved")
		}
		assertStoreEmpty(t, root)
	})

	t.Run("conflicting names in one batch", func(t *testing.T) {
		root := t.TempDir()
		addProposal(t, root, "trace-alpha", "shared-name", proposalFixtureTime)
		addProposal(t, root, "trace-beta", "shared-name", proposalFixtureTime.Add(time.Hour))

		_, err := skills.ApproveProposals(root,
			[]string{"trace-alpha/shared-name", "trace-beta/shared-name"}, "", false, approvedAt, false)
		if err == nil {
			t.Fatal("two proposals for one store name must conflict")
		}
		if !strings.Contains(err.Error(), "one store entry has one name") {
			t.Fatalf("error = %v", err)
		}
		assertStoreEmpty(t, root)
	})

	t.Run("forced replacement waits for the whole batch", func(t *testing.T) {
		root := t.TempDir()
		name := "paseka-release"
		addProposal(t, root, traceID, name, proposalFixtureTime)
		writeSkill(t, skills.Dir(root), name, frontmatter(name), "# hand-written\n")
		badDir := writeProposalBundle(t, root, traceID, "bad-skill",
			"name: another-name\ndescription: The wrong name.\n", "# body\n")
		appendProposalEvent(t, root, traceID, "bad-skill", bundleHash(t, badDir), proposalFixtureTime)

		_, err := skills.ApproveProposals(root, []string{traceID + "/" + name, traceID + "/bad-skill"},
			"", true, approvedAt, false)
		if err == nil {
			t.Fatal("a bad reference must cancel a forced replacement too")
		}
		hand := storeSkill(t, root, name)
		if hand.Metadata[skills.MetadataSource] == skills.ProvenanceProposal {
			t.Fatal("the forced replacement ran before the batch validated")
		}
		if got := proposalDecisions(t, root, traceID); len(got) != 0 {
			t.Fatalf("a failed batch wrote decisions: %+v", got)
		}
	})
}

func assertStoreEmpty(t *testing.T, root string) {
	t.Helper()
	report, err := skills.ScanColony(root)
	if err != nil {
		t.Fatalf("scan store: %v", err)
	}
	if len(report.Skills) != 0 {
		t.Fatalf("store was written: %+v", report.Skills)
	}
}

// TestApproveProposalRefusesSymlinkAndEscape drives the copy path's portability
// rules, which are stricter than a store scan: a bundle is content, not links,
// and its reference may not leave the artifact comb.
func TestApproveProposalRefusesSymlinkAndEscape(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"

	dir := writeProposalBundle(t, root, traceID, "linked-skill", frontmatter("linked-skill"), "# body\n")
	if err := os.Symlink(filepath.Join(dir, skills.ManifestName), filepath.Join(dir, "copy.md")); err != nil {
		t.Fatal(err)
	}
	appendProposalEvent(t, root, traceID, "linked-skill", bundleHash(t, dir), proposalFixtureTime)

	_, err := skills.ApproveProposals(root, []string{traceID + "/linked-skill"}, "", false, approvedAt, false)
	if err == nil {
		t.Fatal("a bundle with a symbolic link must not be installed")
	}
	if !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("error = %v", err)
	}
	assertStoreEmpty(t, root)

	escaped := t.TempDir()
	ev, err := protocol.NewEvent(traceID, "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          "escape-skill",
		Description:   "A bundle outside the comb.",
		Reason:        "The record was hand-edited.",
		Bundle:        "../outside",
		ProcedureHash: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = proposalFixtureTime
	if err := (runs.Trail{ColonyRoot: escaped, TraceID: traceID}).AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	if _, err := skills.ApproveProposals(escaped, []string{traceID + "/escape-skill"}, "", false, approvedAt, false); err == nil {
		t.Fatal("a bundle reference that escapes the comb must not be installed")
	}
	assertStoreEmpty(t, escaped)
}

// TestApproveProposalRefusesExistingAndKeepsDisable pins the force rule: an
// existing entry needs --force, the whole batch validates first, and a disabled
// skill stays disabled with its record intact.
func TestApproveProposalRefusesExistingAndKeepsDisable(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	name := "paseka-release"

	dir := addProposal(t, root, traceID, name, proposalFixtureTime)
	hash := bundleHash(t, dir)
	writeSkill(t, skills.Dir(root), name, frontmatter(name), "# hand-written\n")

	if _, err := skills.ApproveProposals(root, []string{traceID + "/" + name}, "", false, approvedAt, false); err == nil {
		t.Fatal("an existing skill must be refused without --force")
	}
	hand := storeSkill(t, root, name)
	if hand.Metadata[skills.MetadataSource] == "proposal" {
		t.Fatal("a refused replacement changed the existing skill")
	}

	ledger := skills.Ledger{SchemaVersion: skills.LedgerSchemaVersion, Disabled: map[string]skills.DisabledRecord{
		name: {At: proposalFixtureTime.Format(time.RFC3339), Reason: "step two is wrong"},
	}}
	if err := skills.SaveLedger(root, ledger); err != nil {
		t.Fatal(err)
	}

	result, err := skills.ApproveProposals(root, []string{traceID + "/" + name}, "", true, approvedAt, false)
	if err != nil {
		t.Fatalf("forced approve: %v", err)
	}
	if len(result) != 1 || !result[0].Disabled {
		t.Fatalf("forced replacement did not report the disable: %+v", result)
	}
	if result[0].DisableReason != "step two is wrong" {
		t.Fatalf("disable reason = %q", result[0].DisableReason)
	}

	installed := storeSkill(t, root, name)
	if installed.Metadata[skills.MetadataSource] != "proposal" {
		t.Fatalf("forced replacement did not install provenance: %+v", installed.Metadata)
	}
	if installed.Metadata[skills.MetadataProcedureHash] != hash {
		t.Fatalf("installed hash = %q, want %q", installed.Metadata[skills.MetadataProcedureHash], hash)
	}

	after, err := skills.LoadLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := after.Record(name)
	if !ok || record.Reason != "step two is wrong" {
		t.Fatalf("forced replacement cleared the disable record: %+v", after)
	}
}

// TestApproveProposalDryRunWritesNothing pins that a dry run validates the
// whole batch and then leaves the store, ledger, and decisions untouched.
func TestApproveProposalDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	addProposal(t, root, traceID, "paseka-release", proposalFixtureTime)

	result, err := skills.ApproveProposals(root, []string{traceID + "/paseka-release"}, "", false, approvedAt, true)
	if err != nil {
		t.Fatalf("dry-run approve: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("approvals = %d, want the plan", len(result))
	}
	if len(result[0].Files) != 1 {
		t.Fatalf("dry-run plan files = %v", result[0].Files)
	}
	assertStoreEmpty(t, root)
	if _, err := os.Stat(skills.ProposalDecisionsPath(root, traceID)); !os.IsNotExist(err) {
		t.Fatalf("dry run created the decision file: %v", err)
	}
	if _, err := os.Stat(skills.LedgerPath(root)); !os.IsNotExist(err) {
		t.Fatalf("dry run created the ledger: %v", err)
	}

	// A bad reference still fails a dry run, so the preview is the real verdict.
	if _, err := skills.ApproveProposals(root, []string{traceID + "/missing-skill"}, "", false, approvedAt, true); err == nil {
		t.Fatal("a dry run must refuse a missing proposal")
	}
}

// TestProvenanceIssuesDetectsHandEdit pins doctor's mismatch signal and that a
// skill without a recorded hash is not checked.
func TestProvenanceIssuesDetectsHandEdit(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	name := "paseka-release"
	addProposal(t, root, traceID, name, proposalFixtureTime)
	if _, err := skills.ApproveProposals(root, []string{traceID + "/" + name}, "", false, approvedAt, false); err != nil {
		t.Fatalf("approve: %v", err)
	}
	writeSkill(t, skills.Dir(root), "hand-written", frontmatter("hand-written"), "# hand\n")

	report, err := skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if issues := report.ProvenanceIssues(); len(issues) != 0 {
		t.Fatalf("fresh install reported a mismatch: %v", issues)
	}

	installed := storeSkill(t, root, name)
	path := filepath.Join(installed.Dir, skills.ManifestName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(string(data)+"\nAn extra step the Beekeeper never approved.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	issues := report.ProvenanceIssues()
	if len(issues) != 1 {
		t.Fatalf("issues = %v, want one hash mismatch", issues)
	}
	if issues[0].Skill != name || !strings.Contains(issues[0].Reason, "procedure hash mismatch") {
		t.Fatalf("issue = %+v", issues[0])
	}

	// A supporting-file edit is the same fact.
	installed = storeSkill(t, root, name)
	writeFile(t, filepath.Join(installed.Dir, "scripts", "run.sh"), []byte("echo changed\n"))
	report, err = skills.ScanColony(root)
	if err != nil {
		t.Fatal(err)
	}
	if issues := report.ProvenanceIssues(); len(issues) != 1 {
		t.Fatalf("supporting-file edit issues = %v, want one", issues)
	}
}
