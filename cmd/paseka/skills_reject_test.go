package main

import (
	"crypto/sha256"
	"encoding/hex"
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

// decisionLines reads one trail's decision file as raw lines.
func decisionLines(t *testing.T, repo, traceID string) []string {
	t.Helper()
	data, err := os.ReadFile(skills.ProposalDecisionsPath(repo, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// appendProposalDiscovery records a new discovery for an already-written
// bundle, which is how a re-proposal with changed bytes is represented.
func appendProposalDiscovery(t *testing.T, repo, traceID, name string, at time.Time) (string, string) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := protocol.NewEvent(traceID, "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Run the " + name + " procedure.",
		Reason:        "The trail repeated this by hand.",
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

// writeProposalBundleContent writes one bundle with arbitrary SKILL.md bytes.
func writeProposalBundleContent(t *testing.T, repo, traceID, name, content string) string {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, skills.ManifestName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// treeDigest hashes every path and byte under root, so a dry run's promise that
// nothing changed can be checked without trusting one file at a time.
func treeDigest(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = h.Write([]byte(rel))
		_, _ = h.Write([]byte{0})
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestSkillsRejectRecordsRefusalAndIsIdempotent(t *testing.T) {
	repo := setupSkillsCLI(t)
	dir := addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}

	out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-release",
		"--reason", "not a procedure", "-C", repo)
	if err != nil {
		t.Fatalf("skills reject: %v\n%s", err, out)
	}
	if !strings.Contains(out, "refused trace-alpha/paseka-release") {
		t.Fatalf("output = %q", out)
	}

	lines := decisionLines(t, repo, "trace-alpha")
	if len(lines) != 1 {
		t.Fatalf("decision lines = %d, want 1:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	var decision skills.ProposalDecision
	if err := json.Unmarshal([]byte(lines[0]), &decision); err != nil {
		t.Fatalf("decision line is not JSON: %v\n%s", err, lines[0])
	}
	if decision.Skill != "paseka-release" || decision.Trail != "trace-alpha" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.ProcedureHash != hash {
		t.Fatalf("procedureHash = %q, want %q", decision.ProcedureHash, hash)
	}
	if decision.Decision != skills.ProposalRefused {
		t.Fatalf("decision = %q, want refused", decision.Decision)
	}
	if decision.Destination != skills.ProposalStoreDestination {
		t.Fatalf("destination = %q", decision.Destination)
	}
	if decision.Reason != "not a procedure" {
		t.Fatalf("reason = %q", decision.Reason)
	}
	if decision.At.IsZero() {
		t.Fatal("at is zero")
	}

	out, err = runCLI(t, "skills", "proposals", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("skills proposals: %v\n%s", err, out)
	}
	rows := proposalRows(t, out)
	row := rows["paseka-release"]
	if len(row) == 0 {
		t.Fatalf("no row for the rejected proposal:\n%s", out)
	}
	if row[3] != "refused: not a procedure" {
		t.Fatalf("state = %q, want the reason", row[3])
	}

	// Rejecting the same bytes again reports the existing decision and writes
	// nothing.
	out, err = runCLI(t, "skills", "reject", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("rejecting twice must be reported without an error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already refused") || !strings.Contains(out, "no new decision recorded") {
		t.Fatalf("second rejection output = %q", out)
	}
	if got := len(decisionLines(t, repo, "trace-alpha")); got != 1 {
		t.Fatalf("second rejection added a decision line: %d", got)
	}

	// Changed bytes are a new proposal, so a refusal of the old bytes no longer
	// applies and the new bundle can be decided.
	if err := os.MkdirAll(filepath.Join(dir, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "references", "notes.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, newHash := appendProposalDiscovery(t, repo, "trace-alpha", "paseka-release", cliProposalTime.Add(time.Hour))
	if newHash == hash {
		t.Fatal("fixture did not change the procedure hash")
	}

	out, err = runCLI(t, "skills", "proposals", "trace-alpha/paseka-release", "-C", repo)
	if err != nil {
		t.Fatalf("proposals after re-proposal: %v\n%s", err, out)
	}
	if row = proposalRows(t, out)["paseka-release"]; row[3] != "pending" {
		t.Fatalf("a changed bundle must be pending, got %q", row[3])
	}

	if out, err = runCLI(t, "skills", "reject", "trace-alpha/paseka-release", "-C", repo); err != nil {
		t.Fatalf("reject changed bundle: %v\n%s", err, out)
	}
	lines = decisionLines(t, repo, "trace-alpha")
	if len(lines) != 2 {
		t.Fatalf("decision lines = %d, want 2", len(lines))
	}
	var second skills.ProposalDecision
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("second decision line: %v", err)
	}
	if second.ProcedureHash != newHash {
		t.Fatalf("second decision hash = %q, want %q", second.ProcedureHash, newHash)
	}
}

func TestSkillsRejectRefusesNonCandidates(t *testing.T) {
	t.Run("pattern", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
		out, err := runCLI(t, "skills", "reject", "paseka-*", "-C", repo)
		if err == nil {
			t.Fatalf("a pattern must be refused:\n%s", out)
		}
		if !strings.Contains(err.Error(), "not by pattern") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("ambiguous bare name", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
		addCLIProposal(t, repo, "trace-beta", "paseka-release", cliProposalTime.Add(time.Hour))
		out, err := runCLI(t, "skills", "reject", "paseka-release", "-C", repo)
		if err == nil {
			t.Fatalf("an ambiguous bare name must be refused:\n%s", out)
		}
		for _, want := range []string{"trace-alpha", "trace-beta", "trace-alpha/paseka-release", "trace-beta/paseka-release"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q must name %q", err, want)
			}
		}
		if lines := decisionLines(t, repo, "trace-alpha"); len(lines) != 0 {
			t.Fatalf("ambiguity wrote a decision: %v", lines)
		}
	})

	t.Run("missing bundle", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		ev, err := protocol.NewEvent("trace-alpha", "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
			Kind:          protocol.InsightSkillProposed,
			Name:          "paseka-gone",
			Description:   "A bundle that was removed.",
			Reason:        "The trail was cleaned.",
			Bundle:        "skill-proposals/paseka-gone",
			ProcedureHash: "abc123",
		})
		if err != nil {
			t.Fatal(err)
		}
		ev.CreatedAt = cliProposalTime
		if err := (runs.Trail{ColonyRoot: repo, TraceID: "trace-alpha"}).AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
		out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-gone", "-C", repo)
		if err == nil {
			t.Fatalf("a missing bundle must be refused:\n%s", out)
		}
		if !strings.Contains(err.Error(), "no bundle") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("invalid bundle", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		dir := writeProposalBundleContent(t, repo, "trace-alpha", "paseka-release",
			"---\nname: paseka-ship\ndescription: The wrong name.\n---\n# release\n")
		hash, err := skills.ProcedureHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		ev, err := protocol.NewEvent("trace-alpha", "agent-archivist", 0, protocol.EventInsight, protocol.SkillProposedPayload{
			Kind:          protocol.InsightSkillProposed,
			Name:          "paseka-release",
			Description:   "The wrong name.",
			Reason:        "Something repeated.",
			Bundle:        "skill-proposals/paseka-release",
			ProcedureHash: hash,
		})
		if err != nil {
			t.Fatal(err)
		}
		ev.CreatedAt = cliProposalTime
		if err := (runs.Trail{ColonyRoot: repo, TraceID: "trace-alpha"}).AppendEvent(ev); err != nil {
			t.Fatal(err)
		}
		out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-release", "-C", repo)
		if err == nil {
			t.Fatalf("an invalid bundle must be refused:\n%s", out)
		}
		if !strings.Contains(err.Error(), "cannot be refused") || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("already installed decision", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		dir := addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
		hash, err := skills.ProcedureHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		addCLIDecision(t, repo, "trace-alpha", skills.ProposalDecision{
			Skill: "paseka-release", Trail: "trace-alpha", ProcedureHash: hash,
			Destination: skills.ProposalStoreDestination, Decision: skills.ProposalInstalled,
			At: cliProposalTime.Add(time.Hour),
		})
		out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-release", "-C", repo)
		if err == nil {
			t.Fatalf("an installed proposal must be refused:\n%s", out)
		}
		if !strings.Contains(err.Error(), "already installed") {
			t.Fatalf("error = %v", err)
		}
		if got := len(decisionLines(t, repo, "trace-alpha")); got != 1 {
			t.Fatalf("decision lines = %d, want only the install", got)
		}
	})

	t.Run("store already holds the name", func(t *testing.T) {
		repo := setupSkillsCLI(t)
		addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
		writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Run the release procedure.")
		out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-release", "-C", repo)
		if err == nil {
			t.Fatalf("a proposal the store already holds must be refused:\n%s", out)
		}
		if !strings.Contains(err.Error(), "store already holds") {
			t.Fatalf("error = %v", err)
		}
		if got := len(decisionLines(t, repo, "trace-alpha")); got != 0 {
			t.Fatalf("decision lines = %d, want none", got)
		}
	})
}

func TestSkillsRejectDryRunPrintsLineAndChangesNothing(t *testing.T) {
	repo := setupSkillsCLI(t)
	dir := addCLIProposal(t, repo, "trace-alpha", "paseka-release", cliProposalTime)
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeStoreSkill(t, repo, "docs-style", "name: docs-style\ndescription: Write documentation.")
	before := treeDigest(t, repo)

	out, err := runCLI(t, "skills", "reject", "trace-alpha/paseka-release",
		"--reason", "not a procedure", "--dry-run", "-C", repo)
	if err != nil {
		t.Fatalf("skills reject --dry-run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "dry run: would append to") {
		t.Fatalf("output = %q", out)
	}
	var line string
	for _, candidate := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(candidate), "{") {
			line = strings.TrimSpace(candidate)
			break
		}
	}
	if line == "" {
		t.Fatalf("dry run printed no decision line:\n%s", out)
	}
	var decision skills.ProposalDecision
	if err := json.Unmarshal([]byte(line), &decision); err != nil {
		t.Fatalf("dry-run line is not JSON: %v\n%s", err, line)
	}
	if decision.Skill != "paseka-release" || decision.Trail != "trace-alpha" {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.ProcedureHash != hash || decision.Decision != skills.ProposalRefused {
		t.Fatalf("decision = %+v", decision)
	}
	if decision.Reason != "not a procedure" || decision.At.IsZero() {
		t.Fatalf("decision reason/at = %q/%s", decision.Reason, decision.At)
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
