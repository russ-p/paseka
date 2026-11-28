package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

func TestTraceArtifactsAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-artifacts-api"
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(comb, "research.md"), []byte("# Research\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var items []hiveview.ArtifactView
	if err := json.NewDecoder(listRec.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ArtifactKind != "research" {
		t.Fatalf("items = %+v", items)
	}
	if !items[0].Staged || items[0].Announced {
		t.Fatalf("expected staged only: %+v", items[0])
	}

	q := url.Values{"ref": {items[0].Ref}}
	contentReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts?"+q.Encode(), nil)
	contentRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(contentRec, contentReq)
	if contentRec.Code != http.StatusOK {
		t.Fatalf("content status = %d", contentRec.Code)
	}
	var view hiveview.ArtifactContentView
	if err := json.NewDecoder(contentRec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Content == "" || view.ContentHTML == "" {
		t.Fatalf("preview = %+v", view)
	}
}

// A proposal is one thing a Beekeeper decides about, so the comb listing names
// its directory once — with the discovery as the announcement — instead of
// presenting each file of the bundle as a separate handoff.
func TestTraceArtifactsFoldsSkillProposalBundle(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-artifacts-proposal"
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	bundle := filepath.Join(comb, "skill-proposals", "paseka-release")
	writeCombFile(t, filepath.Join(comb, "research.md"), "# Research\n")
	writeCombFile(t, filepath.Join(bundle, "SKILL.md"), "---\nname: paseka-release\n---\n# Release\n")
	writeCombFile(t, filepath.Join(bundle, "scripts", "checks.sh"), "#!/bin/sh\n")
	writeSkillProposed(t, repo, traceID, "paseka-release")

	items := listArtifacts(t, repo, traceID)
	if len(items) != 2 {
		t.Fatalf("items = %+v, want the note and one bundle row", items)
	}
	if items[0].ArtifactKind != "research" {
		t.Fatalf("first row = %+v", items[0])
	}
	row := items[1]
	if row.ArtifactKind != hiveview.ProposalBundleKind {
		t.Fatalf("kind = %q", row.ArtifactKind)
	}
	if row.Title != "paseka-release" || row.Skill != "paseka-release" {
		t.Fatalf("row = %+v", row)
	}
	wantRef := ".paseka/runs/" + traceID + "/artifacts/skill-proposals/paseka-release"
	if row.Ref != wantRef {
		t.Fatalf("ref = %q, want %q", row.Ref, wantRef)
	}
	if row.Files != 2 {
		t.Fatalf("files = %d, want 2", row.Files)
	}
	if row.Bytes <= 0 {
		t.Fatalf("bytes = %d", row.Bytes)
	}
	if !row.Announced || row.Staged {
		t.Fatalf("a folded bundle is announced by its discovery: %+v", row)
	}
	if row.Producer != "archivist-1" {
		t.Fatalf("producer = %q", row.Producer)
	}
}

// A bundle no discovery names is not a proposal: it is the leftover of a run
// that failed its contract, and its files stay visible as the files they are.
func TestTraceArtifactsKeepsAnUnannouncedBundleApart(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-artifacts-orphan"
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	writeCombFile(t, filepath.Join(comb, "research.md"), "# Research\n")
	writeCombFile(t, filepath.Join(comb, "skill-proposals", "paseka-gone", "SKILL.md"), "---\nname: paseka-gone\n---\n")

	items := listArtifacts(t, repo, traceID)
	if len(items) != 2 {
		t.Fatalf("items = %+v, want both files separately", items)
	}
	for _, item := range items {
		if item.ArtifactKind == hiveview.ProposalBundleKind {
			t.Fatalf("folded a bundle nothing announced: %+v", item)
		}
		if !item.Staged {
			t.Fatalf("a file no event announces reads staged: %+v", item)
		}
	}
}

// The console's Mermaid hydration reads goldmark's fenced output, so the class
// on the code element is a contract: if the renderer stops emitting
// `language-mermaid`, diagrams silently fall back to source with no Go change.
func TestTraceArtifactMarkdownKeepsMermaidFence(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-artifacts-mermaid"
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	writeCombFile(t, filepath.Join(comb, "diagram.md"), "# Diagram\n\n```mermaid\ngraph TD\n  A --> B\n```\n")

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	items := listArtifacts(t, repo, traceID)
	if len(items) != 1 {
		t.Fatalf("items = %+v, want the diagram note", items)
	}
	q := url.Values{"ref": {items[0].Ref}}
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts?"+q.Encode(), nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("content status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view hiveview.ArtifactContentView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.ContentHTML, `<code class="language-mermaid">`) {
		t.Fatalf("contentHtml lost the Mermaid fence:\n%s", view.ContentHTML)
	}
	if !strings.Contains(view.ContentHTML, "graph TD") {
		t.Fatalf("contentHtml lost the fence source:\n%s", view.ContentHTML)
	}
}

func listArtifacts(t *testing.T, repo, traceID string) []hiveview.ArtifactView {
	t.Helper()
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", rec.Code, rec.Body.String())
	}
	var items []hiveview.ArtifactView
	if err := json.NewDecoder(rec.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	return items
}

func writeCombFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillProposed(t *testing.T, repo, traceID, name string) {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, "archivist-1", 0, protocol.EventInsight, protocol.SkillProposedPayload{
		Kind:          protocol.InsightSkillProposed,
		Name:          name,
		Description:   "Run the " + name + " procedure.",
		Reason:        "Two runs re-ran the checklist by hand.",
		Bundle:        "skill-proposals/" + name,
		ProcedureHash: "9f2c",
	})
	if err != nil {
		t.Fatal(err)
	}
	trail := runs.Trail{ColonyRoot: repo, TraceID: traceID}
	if err := trail.AppendEvent(ev); err != nil {
		t.Fatal(err)
	}
}
