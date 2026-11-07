package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

	// The plan is a read; only a decision is a write.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/skills/proposals/trace-a/name/plan", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST plan status = %d, want 405", rec.Code)
	}
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills/proposals/trace-a/name/approve", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET approve status = %d, want 405", rec.Code)
	}

	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills/proposals/trace-a/name", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown proposal status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
	// There is no route that accepts or returns a skill body.
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/skills/proposals/trace-a/name/body", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("body route status = %d, want 404", rec.Code)
	}
}

func skillDecisionRequest(t *testing.T, srv *console.Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeSkillDecision(t *testing.T, rec *httptest.ResponseRecorder) console.SkillDecisionResultView {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("decision status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result console.SkillDecisionResultView
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func gitHead(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// TestSkillsAPIPlansAndApprovesAProposal drives the whole deliberate act: the
// plan writes nothing, cancelling would change nothing, and the approval
// installs through the same validated path the shell uses.
func TestSkillsAPIPlansAndApprovesAProposal(t *testing.T) {
	repo := initConsoleRepo(t)
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	writeProposal(t, repo, "trace-alpha", "paseka-release", "Two runs repeated this by hand.", at)
	srv := skillsServer(t, repo)
	head := gitHead(t, repo)

	rec := skillDecisionRequest(t, srv, http.MethodGet,
		"/api/skills/proposals/trace-alpha/paseka-release/plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("plan status = %d body=%s", rec.Code, rec.Body.String())
	}
	var plan console.SkillApprovalPlanView
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	if plan.Trail != "trace-alpha" || plan.Name != "paseka-release" {
		t.Fatalf("plan identity = %+v", plan)
	}
	if plan.Destination != skills.ProposalStoreDestination {
		t.Fatalf("destination = %q, want store", plan.Destination)
	}
	if plan.Target != ".paseka/skills/paseka-release" {
		t.Fatalf("target = %q", plan.Target)
	}
	if len(plan.Files) != 1 || plan.Files[0] != ".paseka/skills/paseka-release/SKILL.md" {
		t.Fatalf("files = %v", plan.Files)
	}
	if plan.ProcedureHash == "" || plan.Replaced || plan.Disabled {
		t.Fatalf("plan details = %+v", plan)
	}
	if !strings.Contains(plan.WorkingTree, "untracked") || !strings.Contains(plan.WorkingTree, "never commits") {
		t.Fatalf("working tree consequence = %q", plan.WorkingTree)
	}

	// Reading the plan is not a decision: the store and the decision file are
	// still absent.
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "skills", "paseka-release")); !os.IsNotExist(err) {
		t.Fatalf("plan wrote the store entry: %v", err)
	}
	if _, err := os.Stat(skills.ProposalDecisionsPath(repo, "trace-alpha")); !os.IsNotExist(err) {
		t.Fatalf("plan wrote a decision: %v", err)
	}
	if now := gitHead(t, repo); now != head {
		t.Fatalf("plan moved HEAD from %s to %s", head, now)
	}

	// A body in the request must be ignored: the console path installs the
	// bundle's bytes and accepts no procedure body.
	rec = skillDecisionRequest(t, srv, http.MethodPost,
		"/api/skills/proposals/trace-alpha/paseka-release/approve", `{"body":"# not the procedure"}`)
	result := decodeSkillDecision(t, rec)
	if result.Proposal.State != skills.ProposalInstalled {
		t.Fatalf("approved row = %+v", result.Proposal)
	}
	if !strings.Contains(result.Message, "Nothing was committed") {
		t.Fatalf("receipt = %q", result.Message)
	}
	if now := gitHead(t, repo); now != head {
		t.Fatalf("approval moved HEAD from %s to %s", head, now)
	}

	installed := skillRow(t, result.Inventory, "paseka-release")
	if installed.Provenance != skills.ProvenanceProposal {
		t.Fatalf("installed provenance = %q", installed.Provenance)
	}
	body, err := os.ReadFile(filepath.Join(repo, ".paseka", "skills", "paseka-release", skills.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "1. Do the thing.") {
		t.Fatalf("installed body = %q", body)
	}
	if strings.Contains(string(body), "not the procedure") {
		t.Fatalf("request body reached the store: %q", body)
	}

	decisions, issues, err := skills.ReadProposalDecisions(
		skills.ProposalDecisionsPath(repo, "trace-alpha"), "trace-alpha",
	)
	if err != nil || len(issues) != 0 {
		t.Fatalf("read decisions: %v %v", err, issues)
	}
	if len(decisions) != 1 || decisions[0].Decision != skills.ProposalInstalled {
		t.Fatalf("decisions = %+v", decisions)
	}
}

// TestSkillsAPIRejectsWithAndWithoutAReason records one terminal decision per
// bundle, with the optional reason, and refuses a later approval.
func TestSkillsAPIRejectsWithAndWithoutAReason(t *testing.T) {
	repo := initConsoleRepo(t)
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	writeProposal(t, repo, "trace-alpha", "paseka-release", "One incident.", at)
	writeProposal(t, repo, "trace-beta", "paseka-ship", "Not a procedure.", at)
	srv := skillsServer(t, repo)

	rec := skillDecisionRequest(t, srv, http.MethodPost,
		"/api/skills/proposals/trace-alpha/paseka-release/reject", `{"reason":"one incident, not a procedure"}`)
	withReason := decodeSkillDecision(t, rec)
	if withReason.Proposal.State != skills.ProposalRefused ||
		withReason.Proposal.Reason != "one incident, not a procedure" {
		t.Fatalf("refused row = %+v", withReason.Proposal)
	}
	if !strings.Contains(withReason.Message, "terminal") {
		t.Fatalf("receipt = %q", withReason.Message)
	}

	rec = skillDecisionRequest(t, srv, http.MethodPost,
		"/api/skills/proposals/trace-beta/paseka-ship/reject", "")
	withoutReason := decodeSkillDecision(t, rec)
	if withoutReason.Proposal.State != skills.ProposalRefused || withoutReason.Proposal.Reason != "" {
		t.Fatalf("refused row = %+v", withoutReason.Proposal)
	}

	// Rejecting the same bundle again reports the existing decision without
	// appending a second line.
	rec = skillDecisionRequest(t, srv, http.MethodPost,
		"/api/skills/proposals/trace-alpha/paseka-release/reject", `{"reason":"a different reason"}`)
	repeat := decodeSkillDecision(t, rec)
	if repeat.Proposal.State != skills.ProposalRefused ||
		repeat.Proposal.Reason != "one incident, not a procedure" {
		t.Fatalf("repeated refusal row = %+v", repeat.Proposal)
	}
	if !strings.Contains(repeat.Message, "already refused") {
		t.Fatalf("repeat receipt = %q", repeat.Message)
	}
	for _, tc := range []struct{ trail, name string }{
		{"trace-alpha", "paseka-release"},
		{"trace-beta", "paseka-ship"},
	} {
		decisions, issues, err := skills.ReadProposalDecisions(
			skills.ProposalDecisionsPath(repo, tc.trail), tc.trail,
		)
		if err != nil || len(issues) != 0 {
			t.Fatalf("read decisions for %s: %v %v", tc.trail, err, issues)
		}
		if len(decisions) != 1 || decisions[0].Decision != skills.ProposalRefused {
			t.Fatalf("%s decisions = %+v", tc.trail, decisions)
		}
	}

	// A refusal is terminal: approving the same bytes is refused and writes no
	// store entry.
	rec = skillDecisionRequest(t, srv, http.MethodPost,
		"/api/skills/proposals/trace-alpha/paseka-release/approve", "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("approve after refusal status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already refused") {
		t.Fatalf("approve after refusal body = %q", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "skills", "paseka-release")); !os.IsNotExist(err) {
		t.Fatalf("refused approval wrote the store: %v", err)
	}
}

// TestSkillsAPIFailedActionReportsTheReasonAndWritesNothing checks the server's
// reason reaches the operator and the failure leaves every file alone.
func TestSkillsAPIFailedActionReportsTheReasonAndWritesNothing(t *testing.T) {
	repo := initConsoleRepo(t)
	at := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	// A discovery whose bundle is gone: there is nothing to install or refuse.
	writeProposalDiscovery(t, repo, "trace-gamma", "paseka-gone", "The bundle was cleaned.", at, "deadbeef")
	srv := skillsServer(t, repo)

	for _, path := range []string{
		"/api/skills/proposals/trace-gamma/paseka-gone/plan",
	} {
		rec := skillDecisionRequest(t, srv, http.MethodGet, path, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("GET %s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "no bundle") {
			t.Fatalf("GET %s body = %q", path, rec.Body.String())
		}
	}
	for _, action := range []string{"approve", "reject"} {
		rec := skillDecisionRequest(t, srv, http.MethodPost,
			"/api/skills/proposals/trace-gamma/paseka-gone/"+action, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d body=%s", action, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "no bundle") {
			t.Fatalf("%s body = %q", action, rec.Body.String())
		}
	}
	if _, err := os.Stat(filepath.Join(repo, ".paseka", "skills")); !os.IsNotExist(err) {
		t.Fatalf("a failed action wrote the store: %v", err)
	}
	if _, err := os.Stat(skills.ProposalDecisionsPath(repo, "trace-gamma")); !os.IsNotExist(err) {
		t.Fatalf("a failed action wrote a decision: %v", err)
	}
}
