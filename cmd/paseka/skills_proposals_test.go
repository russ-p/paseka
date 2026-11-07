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

var cliProposalTime = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

// addCLIProposal creates one valid bundle and its discovery in a repo trail.
func addCLIProposal(t *testing.T, repo, traceID, name string, at time.Time) string {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Run the " + name + " procedure.\n---\n# " + name + "\n\n1. Do the thing.\n"
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
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
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	if err := trail.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
	return dir
}

func addCLIDecision(t *testing.T, repo, traceID string, decision skills.ProposalDecision) {
	t.Helper()
	if err := skills.AppendProposalDecision(repo, traceID, decision); err != nil {
		t.Fatal(err)
	}
}

// proposalRows parses the proposals table keyed by skill name.
func proposalRows(t *testing.T, out string) map[string][]string {
	t.Helper()
	rows := map[string][]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := tableColumns(line)
		if len(cols) < 5 || cols[0] == "TRAIL" {
			continue
		}
		rows[cols[1]] = cols
	}
	return rows
}

func TestSkillsProposalsTable(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "pending-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-alpha", "refused-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-alpha", "trial-skill", cliProposalTime)
	addCLIProposal(t, repo, "trace-alpha", "installed-skill", cliProposalTime)

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

	trialDir := filepath.Join(repo, ".paseka", "runs", "trace-alpha", "artifacts", "skill-proposals", "trial-skill")
	trialHash, err := skills.ProcedureHash(trialDir)
	if err != nil {
		t.Fatal(err)
	}
	addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
		Skill: "trial-skill", Trail: "trace-alpha", ProcedureHash: trialHash,
		Destination: "trace-dest", Decision: skills.ProposalTrialled,
		At: cliProposalTime.Add(time.Hour),
	})

	installedDir := filepath.Join(repo, ".paseka", "runs", "trace-alpha", "artifacts", "skill-proposals", "installed-skill")
	installedHash, err := skills.ProcedureHash(installedDir)
	if err != nil {
		t.Fatal(err)
	}
	addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
		Skill: "installed-skill", Trail: "trace-alpha", ProcedureHash: installedHash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
		At: cliProposalTime.Add(time.Hour),
	})

	ev, err := protocol.NewEvent("trace-beta", "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          "gone-skill",
		Description:   "A bundle that was removed.",
		Reason:        "The trail was cleaned.",
		Bundle:        "skill-proposals/gone-skill",
		ProcedureHash: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	ev.CreatedAt = cliProposalTime
	if err := (runs.Trail{ColonyRoot: repo, TraceID: "trace-beta"}).AppendEvent(ev); err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "proposals", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals: %v\n%s", err, out)
	}
	for _, header := range []string{"TRAIL", "SKILL", "PROPOSED", "STATE", "BUNDLE"} {
		if !strings.Contains(out, header) {
			t.Fatalf("table header missing %q:\n%s", header, out)
		}
	}
	rows := proposalRows(t, out)
	if len(rows) != 5 {
		t.Fatalf("got %d rows, want 5:\n%s", len(rows), out)
	}

	pending := rows["pending-skill"]
	if pending[0] != "trace-alpha" || pending[3] != "pending" || pending[4] != "valid" {
		t.Fatalf("pending row = %v", pending)
	}
	if !strings.HasPrefix(pending[2], "2026-11-07T10:00:00Z") {
		t.Fatalf("proposed column = %q", pending[2])
	}
	if refused := rows["refused-skill"]; refused[3] != "refused: not a procedure" {
		t.Fatalf("refused state = %q", refused[3])
	}
	if trialled := rows["trial-skill"]; trialled[3] != "trialled: trace-dest" {
		t.Fatalf("trialled state = %q", trialled[3])
	}
	if installed := rows["installed-skill"]; installed[3] != "installed" {
		t.Fatalf("installed state = %q", installed[3])
	}
	gone := rows["gone-skill"]
	if gone[0] != "trace-beta" || gone[3] != "missing" || gone[4] != "missing" {
		t.Fatalf("missing row = %v", gone)
	}
}

func TestSkillsProposalsJSON(t *testing.T) {
	repo := setupSkillsCLI(t)
	dir := addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
		Skill: "paseka-release", Trail: "trace-alpha", ProcedureHash: hash,
		Destination: "trace-dest", Decision: skills.ProposalTrialled,
		Roles: []string{"builder"}, At: cliProposalTime.Add(time.Hour),
	})
	addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
		Skill: "paseka-release", Trail: "trace-alpha", ProcedureHash: hash,
		Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
		At: cliProposalTime.Add(2 * time.Hour),
	})

	out, err := runCLI(t, "skills", "proposals", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals --json: %v\n%s", err, out)
	}
	var doc struct {
		Proposals []struct {
			Trail          string   `json:"trail"`
			Name           string   `json:"name"`
			Description    string   `json:"description"`
			ProposedAt     string   `json:"proposedAt"`
			State          string   `json:"state"`
			Reason         string   `json:"reason"`
			Destination    string   `json:"destination"`
			DecidedAt      string   `json:"decidedAt"`
			Bundle         string   `json:"bundle"`
			BundleMissing  bool     `json:"bundleMissing"`
			ProcedureHash  string   `json:"procedureHash"`
			RecomputedHash string   `json:"recomputedHash"`
			Valid          bool     `json:"valid"`
			Validation     []string `json:"validation"`
			Decisions      []struct {
				Decision    string   `json:"decision"`
				Destination string   `json:"destination"`
				At          string   `json:"at"`
				Roles       []string `json:"roles"`
			} `json:"decisions"`
		} `json:"proposals"`
		Issues []string `json:"issues"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if doc.Proposals == nil || doc.Issues == nil {
		t.Fatalf("proposals and issues must be arrays: %#v", doc)
	}
	if len(doc.Proposals) != 1 {
		t.Fatalf("proposals = %+v", doc.Proposals)
	}
	row := doc.Proposals[0]
	if row.Trail != "trace-alpha" || row.Name != "paseka-release" {
		t.Fatalf("row = %+v", row)
	}
	if row.State != skills.ProposalInstalled || !row.Valid {
		t.Fatalf("state/valid = %q/%v", row.State, row.Valid)
	}
	if row.ProcedureHash != hash || row.RecomputedHash != hash {
		t.Fatalf("hashes = %q / %q, want %q", row.ProcedureHash, row.RecomputedHash, hash)
	}
	if row.DecidedAt == "" {
		t.Fatal("decidedAt is empty")
	}
	if len(row.Decisions) != 2 {
		t.Fatalf("decisions = %+v, want the newest per destination", row.Decisions)
	}
	if row.Validation == nil {
		t.Fatal("validation must be an array")
	}
}

func TestSkillsProposalsResolvesReferences(t *testing.T) {
	repo := setupSkillsCLI(t)
	addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	addCLIProposal(t, repo, "trace-beta", "paseka-release", cliProposalTime.Add(time.Hour))
	addCLIProposal(t, repo, "trace-beta", "paseka-ship", cliProposalTime.Add(2*time.Hour))

	out, err := runCLI(t, "skills", "proposals", "paseka-release", "-C", repo)
	if err == nil {
		t.Fatalf("an ambiguous bare name must be refused:\n%s", out)
	}
	for _, want := range []string{"trace-alpha", "trace-beta", "trace-alpha/paseka-release", "trace-beta/paseka-release"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguity error %q must name %q", err, want)
		}
	}

	out, err = runCLI(t, "skills", "proposals", "trace-beta/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("qualified reference: %v\n%s", err, out)
	}
	rows := proposalRows(t, out)
	if len(rows) != 1 || rows["paseka-release"][0] != "trace-beta" {
		t.Fatalf("qualified selection = %v", rows)
	}

	out, err = runCLI(t, "skills", "proposals", "paseka-release", "--trail", "trace-alpha", "-C", repo)
	if err != nil {
		t.Fatalf("--trail selection: %v\n%s", err, out)
	}
	rows = proposalRows(t, out)
	if len(rows) != 1 || rows["paseka-release"][0] != "trace-alpha" {
		t.Fatalf("--trail selection = %v", rows)
	}

	out, err = runCLI(t, "skills", "proposals", "--trail", "trace-beta", "-C", repo)
	if err != nil {
		t.Fatalf("--trail filter: %v\n%s", err, out)
	}
	rows = proposalRows(t, out)
	if len(rows) != 2 || rows["paseka-ship"] == nil {
		t.Fatalf("--trail filter = %v", rows)
	}

	if out, err = runCLI(t, "skills", "proposals", "no-such-skill", "-C", repo); err == nil {
		t.Fatalf("an unknown proposal must fail:\n%s", out)
	}
	if out, err = runCLI(t, "skills", "proposals", "paseka-*", "-C", repo); err == nil {
		t.Fatalf("a pattern must be refused:\n%s", out)
	}
}

func TestSkillsProposalsEmpty(t *testing.T) {
	repo := setupSkillsCLI(t)
	out, err := runCLI(t, "skills", "proposals", "--json", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals on an empty colony: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"proposals": []`) || !strings.Contains(out, `"issues": []`) {
		t.Fatalf("empty inventory must render empty arrays:\n%s", out)
	}
}
