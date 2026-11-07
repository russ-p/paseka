package skills_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
)

var proposalFixtureTime = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

// writeProposalBundle writes one bundle under a trail's artifact comb and
// returns its directory.
func writeProposalBundle(t *testing.T, root, traceID, name, frontmatter, body string) string {
	t.Helper()
	comb := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", "skill-proposals")
	writeSkill(t, comb, name, frontmatter, body)
	return filepath.Join(comb, name)
}

// bundleHash is the identity a correct discovery carries.
func bundleHash(t *testing.T, dir string) string {
	t.Helper()
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatalf("procedure hash: %v", err)
	}
	return hash
}

// appendProposalEvent records one discovery in the trail's event record.
func appendProposalEvent(t *testing.T, root, traceID, name, hash string, at time.Time) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Run the " + name + " procedure. Use when the task calls for it.",
		Reason:        "Two runs repeated this by hand; see task 002.",
		Bundle:        "skill-proposals/" + name,
		ProcedureHash: hash,
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = at
	trail := runs.Trail{ColonyRoot: root, TraceID: traceID}
	if err := trail.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
}

// addProposal creates a valid bundle and its matching discovery.
func addProposal(t *testing.T, root, traceID, name string, at time.Time) string {
	t.Helper()
	dir := writeProposalBundle(t, root, traceID, name, frontmatter(name), "# "+name+"\n\n1. Do the thing.\n")
	appendProposalEvent(t, root, traceID, name, bundleHash(t, dir), at)
	return dir
}

func appendDecision(t *testing.T, root, traceID string, decision skills.ProposalDecision) {
	t.Helper()
	if err := skills.AppendProposalDecision(root, traceID, decision); err != nil {
		t.Fatalf("append decision: %v", err)
	}
}

func proposalByName(t *testing.T, inventory skills.ProposalInventory, name string) skills.Proposal {
	t.Helper()
	for _, proposal := range inventory.Proposals {
		if proposal.Name == name {
			return proposal
		}
	}
	t.Fatalf("no proposal %q in %+v", name, inventory.Proposals)
	return skills.Proposal{}
}

func proposalIssueText(inventory skills.ProposalInventory) string {
	lines := make([]string, 0, len(inventory.Issues))
	for _, issue := range inventory.Issues {
		lines = append(lines, issue.Error())
	}
	return strings.Join(lines, "\n")
}

func TestScanProposalsListsPendingWithTrailNameAndTime(t *testing.T) {
	root := t.TempDir()
	addProposal(t, root, "trace-alpha", "paseka-release", proposalFixtureTime)

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	if len(inventory.Issues) != 0 {
		t.Fatalf("unexpected issues: %s", proposalIssueText(inventory))
	}
	if len(inventory.Proposals) != 1 {
		t.Fatalf("proposals = %+v, want one", inventory.Proposals)
	}
	proposal := inventory.Proposals[0]
	if proposal.Trail != "trace-alpha" || proposal.Name != "paseka-release" {
		t.Fatalf("row = %+v", proposal)
	}
	if !proposal.ProposedAt.Equal(proposalFixtureTime) {
		t.Fatalf("proposedAt = %s, want %s", proposal.ProposedAt, proposalFixtureTime)
	}
	if proposal.State != skills.ProposalPending {
		t.Fatalf("state = %q, want pending", proposal.State)
	}
	if !proposal.Valid || len(proposal.Validation) != 0 {
		t.Fatalf("valid = %v, validation = %v", proposal.Valid, proposal.Validation)
	}
}

func TestScanProposalsReportsMissingBundle(t *testing.T) {
	root := t.TempDir()
	appendProposalEvent(t, root, "trace-alpha", "paseka-gone", "abc123", proposalFixtureTime)

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	proposal := proposalByName(t, inventory, "paseka-gone")
	if proposal.State != skills.ProposalMissing {
		t.Fatalf("state = %q, want missing", proposal.State)
	}
	if !proposal.BundleMissing {
		t.Fatal("missing bundle must be reported as such")
	}
	if proposal.Valid {
		t.Fatal("a missing bundle must not be valid")
	}
}

// TestScanProposalsAppliesDecisions pins the states a decision line can produce,
// the newest decision per destination, and a refusal's terminality.
func TestScanProposalsAppliesDecisions(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"

	installedDir := addProposal(t, root, traceID, "installed-skill", proposalFixtureTime)
	installedHash := bundleHash(t, installedDir)
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "installed-skill", Trail: traceID, ProcedureHash: installedHash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
		At: proposalFixtureTime.Add(time.Hour),
	})
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "installed-skill", Trail: traceID, ProcedureHash: installedHash,
		Destination: "trace-trial", Decision: skills.ProposalTrialled,
		Roles: []string{"builder"}, At: proposalFixtureTime.Add(2 * time.Hour),
	})

	trialledDir := addProposal(t, root, traceID, "trialled-skill", proposalFixtureTime)
	trialledHash := bundleHash(t, trialledDir)
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "trialled-skill", Trail: traceID, ProcedureHash: trialledHash,
		Destination: "trace-old", Decision: skills.ProposalTrialled,
		At: proposalFixtureTime.Add(time.Hour),
	})
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "trialled-skill", Trail: traceID, ProcedureHash: trialledHash,
		Destination: "trace-new", Decision: skills.ProposalTrialled,
		Roles: []string{"allrounder"}, At: proposalFixtureTime.Add(3 * time.Hour),
	})
	// A second trial on the same destination: the newest line is the one shown.
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "trialled-skill", Trail: traceID, ProcedureHash: trialledHash,
		Destination: "trace-new", Decision: skills.ProposalTrialled,
		Roles: []string{"shipper"}, At: proposalFixtureTime.Add(4 * time.Hour),
	})

	refusedDir := addProposal(t, root, traceID, "refused-skill", proposalFixtureTime)
	refusedHash := bundleHash(t, refusedDir)
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "refused-skill", Trail: traceID, ProcedureHash: refusedHash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalRefused,
		Reason: "not a procedure", At: proposalFixtureTime.Add(time.Hour),
	})
	// A refusal is terminal for the exact bundle: a later line must not make
	// the same bytes installed.
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "refused-skill", Trail: traceID, ProcedureHash: refusedHash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
		At: proposalFixtureTime.Add(4 * time.Hour),
	})

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	if len(inventory.Issues) != 0 {
		t.Fatalf("unexpected issues: %s", proposalIssueText(inventory))
	}

	installed := proposalByName(t, inventory, "installed-skill")
	if installed.State != skills.ProposalInstalled {
		t.Fatalf("installed state = %q", installed.State)
	}
	if len(installed.Decisions) != 2 {
		t.Fatalf("decisions = %+v, want one per destination", installed.Decisions)
	}

	trialled := proposalByName(t, inventory, "trialled-skill")
	if trialled.State != skills.ProposalTrialled || trialled.Destination != "trace-new" {
		t.Fatalf("trialled = %+v", trialled)
	}
	if !trialled.DecidedAt.Equal(proposalFixtureTime.Add(4 * time.Hour)) {
		t.Fatalf("decidedAt = %s, want the newest decision", trialled.DecidedAt)
	}
	if len(trialled.Decisions) != 2 {
		t.Fatalf("decisions = %+v, want one per destination", trialled.Decisions)
	}
	for _, decision := range trialled.Decisions {
		if decision.Destination == "trace-new" && (len(decision.Roles) != 1 || decision.Roles[0] != "shipper") {
			t.Fatalf("newest same-destination decision was not kept: %+v", decision)
		}
	}

	refused := proposalByName(t, inventory, "refused-skill")
	if refused.State != skills.ProposalRefused || refused.Reason != "not a procedure" {
		t.Fatalf("refused = %+v", refused)
	}

	// A decision keyed by other bytes does not apply to a changed bundle.
	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "refused-skill", Trail: traceID, ProcedureHash: "different-bytes",
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalRefused,
		Reason: "someone else's bundle", At: proposalFixtureTime.Add(5 * time.Hour),
	})
	inventory, err = skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	if got := proposalByName(t, inventory, "refused-skill"); got.Reason != "not a procedure" {
		t.Fatalf("a decision for other bytes changed the row: %+v", got)
	}
}

func TestScanProposalsReportsMalformedDecisionLines(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-alpha"
	dir := addProposal(t, root, traceID, "paseka-release", proposalFixtureTime)
	hash := bundleHash(t, dir)

	appendDecision(t, root, traceID, skills.ProposalDecision{
		Skill: "paseka-release", Trail: traceID, ProcedureHash: hash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
		At: proposalFixtureTime,
	})
	path := skills.ProposalDecisionsPath(root, traceID)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json\n" +
		`{"skill":"paseka-release","trail":"trace-alpha","procedureHash":"` + hash + `","destination":"store","decision":"bogus","at":"2026-11-07T10:00:00Z"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	issues := proposalIssueText(inventory)
	if !strings.Contains(issues, "malformed decision line") {
		t.Fatalf("malformed JSON was not reported: %s", issues)
	}
	if !strings.Contains(issues, `decision "bogus"`) {
		t.Fatalf("unknown decision was not reported: %s", issues)
	}
	if got := proposalByName(t, inventory, "paseka-release"); got.State != skills.ProposalInstalled {
		t.Fatalf("a damaged line hid the valid decision: %+v", got)
	}
}

// TestScanProposalsValidatesBundles pins the three invalid-bundle reports: a
// hash mismatch, an invalid SKILL.md, and a non-regular entry.
func TestScanProposalsValidatesBundles(t *testing.T) {
	root := t.TempDir()

	_ = writeProposalBundle(t, root, "trace-alpha", "hash-mismatch", frontmatter("hash-mismatch"), "# body\n")
	appendProposalEvent(t, root, "trace-alpha", "hash-mismatch", "0000", proposalFixtureTime)

	writeProposalBundle(t, root, "trace-alpha", "bad-skill",
		"name: some-other-name\ndescription: Says it is another skill.", "# body\n")
	badDir := filepath.Join(root, ".paseka", "runs", "trace-alpha", "artifacts", "skill-proposals", "bad-skill")
	appendProposalEvent(t, root, "trace-alpha", "bad-skill", bundleHash(t, badDir), proposalFixtureTime)

	linkDir := writeProposalBundle(t, root, "trace-alpha", "linked-skill", frontmatter("linked-skill"), "# body\n")
	if err := os.Symlink(filepath.Join(linkDir, skills.ManifestName), filepath.Join(linkDir, "copy.md")); err != nil {
		t.Fatal(err)
	}
	appendProposalEvent(t, root, "trace-alpha", "linked-skill", bundleHash(t, linkDir), proposalFixtureTime)

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}

	mismatch := proposalByName(t, inventory, "hash-mismatch")
	if mismatch.Valid {
		t.Fatal("a hash mismatch must not be valid")
	}
	if !strings.Contains(strings.Join(mismatch.Validation, "; "), "procedure hash mismatch") {
		t.Fatalf("mismatch findings = %v", mismatch.Validation)
	}
	if mismatch.State != skills.ProposalPending {
		t.Fatalf("a mismatched pending discovery lost its state: %q", mismatch.State)
	}

	bad := proposalByName(t, inventory, "bad-skill")
	if bad.Valid {
		t.Fatal("an invalid skill must not be valid")
	}
	if !strings.Contains(strings.Join(bad.Validation, "; "), "does not match its directory") {
		t.Fatalf("invalid skill findings = %v", bad.Validation)
	}

	linked := proposalByName(t, inventory, "linked-skill")
	if linked.Valid {
		t.Fatal("a bundle with a non-regular file must not be valid")
	}
	if !strings.Contains(strings.Join(linked.Validation, "; "), "symbolic link") {
		t.Fatalf("non-regular findings = %v", linked.Validation)
	}
}

func TestScanProposalsReportsUnreadableTrail(t *testing.T) {
	root := t.TempDir()
	traceRoot := filepath.Join(root, ".paseka", "runs", "trace-broken")
	if err := os.MkdirAll(filepath.Join(traceRoot, runs.TrailEventsFileName), 0o755); err != nil {
		t.Fatal(err)
	}

	inventory, err := skills.ScanProposals(root)
	if err != nil {
		t.Fatalf("scan proposals: %v", err)
	}
	if !strings.Contains(proposalIssueText(inventory), "unreadable") {
		t.Fatalf("unreadable trail was not reported: %s", proposalIssueText(inventory))
	}
}

// TestProcedureHashMatchesDocumentedPipeline pins the hash against the bytes the
// Archivist's shell recipe would hash: body first, then each supporting file
// preceded by its path, in byte order.
func TestProcedureHashMatchesDocumentedPipeline(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "paseka-release", frontmatter("paseka-release"), "one\ntwo\n")
	writeFile(t, filepath.Join(dir, "paseka-release", "scripts", "run.sh"), []byte("echo hi\n"))

	serviceDir := filepath.Join(dir, "paseka-release")
	wantBytes := "one\ntwo\n" + "scripts/run.sh\n" + "echo hi\n"
	wantSum := sha256.Sum256([]byte(wantBytes))

	got, err := skills.ProcedureHash(serviceDir)
	if err != nil {
		t.Fatalf("procedure hash: %v", err)
	}
	if got != hex.EncodeToString(wantSum[:]) {
		t.Fatalf("hash = %s, want %s for %q", got, hex.EncodeToString(wantSum[:]), wantBytes)
	}
}

func TestProposalReferenceResolution(t *testing.T) {
	inventory := skills.ProposalInventory{Proposals: []skills.Proposal{
		{Trail: "trace-a", Name: "paseka-release"},
		{Trail: "trace-b", Name: "paseka-release"},
		{Trail: "trace-b", Name: "paseka-ship"},
	}}

	ref, err := skills.ParseProposalReference("paseka-release", "")
	if err != nil {
		t.Fatalf("parse bare: %v", err)
	}
	if _, err := inventory.Resolve(ref); err == nil {
		t.Fatal("an ambiguous bare name must be refused")
	} else {
		for _, want := range []string{"trace-a", "trace-b", "trace-a/paseka-release", "trace-b/paseka-release"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("ambiguity error %q must name %q", err, want)
			}
		}
	}

	ref, err = skills.ParseProposalReference("trace-b/paseka-release", "")
	if err != nil {
		t.Fatalf("parse qualified: %v", err)
	}
	matched, err := inventory.Resolve(ref)
	if err != nil || len(matched) != 1 || matched[0].Trail != "trace-b" {
		t.Fatalf("qualified resolution = %+v, %v", matched, err)
	}

	ref, err = skills.ParseProposalReference("paseka-ship", "trace-b")
	if err != nil {
		t.Fatalf("parse with --trail: %v", err)
	}
	if _, err := inventory.Resolve(ref); err != nil {
		t.Fatalf("--trail must select one trail: %v", err)
	}

	if _, err := skills.ParseProposalReference("trace-a/paseka-release", "trace-b"); err == nil {
		t.Fatal("a qualified reference that disagrees with --trail must be refused")
	}
	if _, err := skills.ParseProposalReference("paseka-*", ""); err == nil {
		t.Fatal("a pattern must be refused")
	}
}
