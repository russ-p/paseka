package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
	"github.com/russ-p/paseka/internal/skills"
)

func skillsServer(t *testing.T, repo string) *console.Server {
	t.Helper()
	return console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   setupConsoleHome(t, repo),
		Sessions: sessions.NewManager(),
	})
}

func getSkillsInventory(t *testing.T, srv *console.Server) console.SkillsInventoryView {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/skills status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.SkillsInventoryView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

func getSkillProposal(t *testing.T, srv *console.Server, trail, name string) console.SkillProposalDetailView {
	t.Helper()
	rec := httptest.NewRecorder()
	path := "/api/skills/proposals/" + trail + "/" + name
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d body=%s", path, rec.Code, rec.Body.String())
	}
	var view console.SkillProposalDetailView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

func writeStoreSkill(t *testing.T, repo, name, frontmatter, body string) {
	t.Helper()
	writeStoreFile(t, filepath.Join(repo, ".paseka", "skills", name, skills.ManifestName),
		"---\n"+frontmatter+"\n---\n"+body)
}

func writeStoreFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeProposal creates one proposal bundle and its discovery. An empty name
// for the bundle directory is not supported: use writeProposalDiscovery for a
// discovery whose bundle is gone.
func writeProposal(t *testing.T, repo, traceID, name, reason string, at time.Time) {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	writeStoreFile(t, filepath.Join(dir, skills.ManifestName),
		"---\nname: "+name+"\ndescription: Run the "+name+" procedure.\n---\n# "+name+"\n\n1. Do the thing.\n")
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	writeProposalDiscovery(t, repo, traceID, name, reason, at, hash)
}

func writeProposalDiscovery(t *testing.T, repo, traceID, name, reason string, at time.Time, hash string) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "archivist-1", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Run the " + name + " procedure.",
		Reason:        reason,
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
}

func skillRow(t *testing.T, view console.SkillsInventoryView, name string) console.SkillRowView {
	t.Helper()
	for _, row := range view.Skills {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("no skill %q in %+v", name, view.Skills)
	return console.SkillRowView{}
}

func proposalRow(t *testing.T, view console.SkillsInventoryView, trail, name string) console.SkillProposalView {
	t.Helper()
	for _, row := range view.Proposals {
		if row.Trail == trail && row.Name == name {
			return row
		}
	}
	t.Fatalf("no proposal %s/%s in %+v", trail, name, view.Proposals)
	return console.SkillProposalView{}
}

func TestSkillsAPIReadsTheSharedInventory(t *testing.T) {
	repo := initConsoleRepo(t)
	// The wildcard role is named against every skill it resolves to, and the
	// family pattern against its own family — never as `*` or `paseka-*`.
	writeBeeYAML(t, repo, "scout", "role: scout\nadapter: cursor\nprompt_template: scout.md\nskills:\n  - '*'\n")
	writeBeeYAML(t, repo, "docs", "role: docs\nadapter: script\ncommand: [echo]\nskills:\n  - 'paseka-*'\n")
	writeStoreSkill(t, repo, "paseka-release", "name: paseka-release\ndescription: Release procedure.\nmetadata:\n  source: proposal", "# Release\n")
	writeStoreSkill(t, repo, "paseka-ship", "name: paseka-ship\ndescription: Ship procedure.", "# Ship\n")
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "skills", "broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeStoreFile(t, filepath.Join(repo, ".paseka", "skills.index.json"),
		`{"schema_version":1,"disabled":{"paseka-ship":{"at":"2026-11-06T12:00:00Z","reason":"wrong step"}}}`)

	view := getSkillsInventory(t, skillsServer(t, repo))

	if view.Store == "" {
		t.Fatal("store root is empty")
	}
	release := skillRow(t, view, "paseka-release")
	if !strings.Contains(strings.Join(release.Roles, ","), "scout") ||
		!strings.Contains(strings.Join(release.Roles, ","), "docs") {
		t.Fatalf("paseka-release roles = %v, want the wildcard and pattern expanded", release.Roles)
	}
	if release.Provenance != skills.ProvenanceProposal {
		t.Fatalf("provenance = %q, want proposal", release.Provenance)
	}

	ship := skillRow(t, view, "paseka-ship")
	if ship.State != skills.StateDisabled || ship.Reason != "wrong step" {
		t.Fatalf("disabled row = %+v", ship)
	}

	broken := skillRow(t, view, "broken")
	if broken.State != skills.StateInvalid || !strings.Contains(broken.Reason, "SKILL.md is missing") {
		t.Fatalf("invalid row = %+v", broken)
	}

	if view.Skills == nil || view.Proposals == nil || view.Issues == nil || view.ProposalIssues == nil {
		t.Fatalf("every list must be an array, not null: %+v", view)
	}
}

func TestSkillsAPIReportsProposalStatesAndOpensABody(t *testing.T) {
	repo := initConsoleRepo(t)
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	writeProposal(t, repo, "trace-alpha", "paseka-release", "Two runs repeated this by hand.", at)
	writeProposal(t, repo, "trace-beta", "paseka-ship", "The release step was skipped.", at.Add(time.Hour))
	// A discovery whose bundle has disappeared stays visible as missing.
	writeProposalDiscovery(t, repo, "trace-gamma", "paseka-gone", "The bundle was cleaned.", at.Add(2*time.Hour), "deadbeef")
	if err := skills.AppendProposalDecision(repo, "trace-beta", skills.ProposalDecision{
		Skill:         "paseka-ship",
		Trail:         "trace-beta",
		ProcedureHash: hashOfProposal(t, repo, "trace-beta", "paseka-ship"),
		Destination:   "trace-dest",
		Decision:      skills.ProposalTrialled,
		At:            at.Add(3 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	writeProposal(t, repo, "trace-delta", "paseka-doc", "Documentation kept being re-derived.", at.Add(4*time.Hour))
	if err := skills.AppendProposalDecision(repo, "trace-delta", skills.ProposalDecision{
		Skill:         "paseka-doc",
		Trail:         "trace-delta",
		ProcedureHash: hashOfProposal(t, repo, "trace-delta", "paseka-doc"),
		Destination:   skills.ProposalStoreDestination,
		Decision:      skills.ProposalInstalled,
		At:            at.Add(5 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	writeProposal(t, repo, "trace-epsilon", "paseka-bad", "This was one incident, not a procedure.", at.Add(6*time.Hour))
	if err := skills.AppendProposalDecision(repo, "trace-epsilon", skills.ProposalDecision{
		Skill:         "paseka-bad",
		Trail:         "trace-epsilon",
		ProcedureHash: hashOfProposal(t, repo, "trace-epsilon", "paseka-bad"),
		Destination:   skills.ProposalStoreDestination,
		Decision:      skills.ProposalRefused,
		Reason:        "one incident, not a procedure",
		At:            at.Add(7 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	srv := skillsServer(t, repo)
	view := getSkillsInventory(t, srv)

	pending := proposalRow(t, view, "trace-alpha", "paseka-release")
	if pending.State != skills.ProposalPending || pending.ProposedAt == "" {
		t.Fatalf("pending row = %+v", pending)
	}
	trialled := proposalRow(t, view, "trace-beta", "paseka-ship")
	if trialled.State != skills.ProposalTrialled || trialled.Destination != "trace-dest" || trialled.DecidedAt == "" {
		t.Fatalf("trialled row = %+v", trialled)
	}
	installed := proposalRow(t, view, "trace-delta", "paseka-doc")
	if installed.State != skills.ProposalInstalled || installed.Destination != skills.ProposalStoreDestination {
		t.Fatalf("installed row = %+v", installed)
	}
	refused := proposalRow(t, view, "trace-epsilon", "paseka-bad")
	if refused.State != skills.ProposalRefused || refused.Reason != "one incident, not a procedure" {
		t.Fatalf("refused row = %+v", refused)
	}
	missing := proposalRow(t, view, "trace-gamma", "paseka-gone")
	if missing.State != skills.ProposalMissing || !missing.BundleMissing {
		t.Fatalf("missing row = %+v", missing)
	}

	detail := getSkillProposal(t, srv, "trace-alpha", "paseka-release")
	if !strings.Contains(detail.Body, "1. Do the thing.") {
		t.Fatalf("body = %q", detail.Body)
	}
	if detail.ProposedReason != "Two runs repeated this by hand." {
		t.Fatalf("proposed reason = %q", detail.ProposedReason)
	}
	if detail.BodyError != "" {
		t.Fatalf("body error = %q", detail.BodyError)
	}

	// Opening a proposal whose bundle is gone reports the absence in place
	// rather than hiding the row or failing the read.
	gone := getSkillProposal(t, srv, "trace-gamma", "paseka-gone")
	if gone.BodyError == "" {
		t.Fatal("a missing bundle must report a body error")
	}
}

func hashOfProposal(t *testing.T, repo, traceID, name string) string {
	t.Helper()
	dir := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts", "skill-proposals", name)
	hash, err := skills.ProcedureHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func TestSkillsAPIReportsAnUnreadableTrailInPlace(t *testing.T) {
	repo := initConsoleRepo(t)
	// A trail whose event record cannot be read must be named in the proposal
	// issues, not silently dropped from the pane.
	bad := filepath.Join(repo, ".paseka", "runs", "trace-bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, runs.TrailEventsFileName), []byte("{not json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	view := getSkillsInventory(t, skillsServer(t, repo))
	if len(view.ProposalIssues) == 0 {
		t.Fatal("an unreadable trail must be reported in proposalIssues")
	}
}

func TestSkillsAPIIsReadOnly(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := skillsServer(t, repo)

	for _, path := range []string{"/api/skills", "/api/skills/proposals/trace-a/name"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST %s status = %d, want 405", path, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills/proposals/trace-a/name", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown proposal status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
