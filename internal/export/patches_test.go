package export

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
)

// writeProposalPatch installs one archive record for a trail, the way the runtime would.
func writeProposalPatch(t *testing.T, repo, traceID, agentID, body string, at time.Time) {
	t.Helper()
	store := patches.NewStore(repo, traceID)
	if _, err := store.Write(patches.Record{
		AgentID:     agentID,
		Bee:         "builder",
		Mode:        patches.ModeAFK,
		Kind:        string(protocol.MutationCodeProposalIsolated),
		Workspace:   "isolated",
		BaseSha:     "9f2c1ab",
		Summary:     "add the export flag",
		CapturedAt:  at,
		FileCount:   patches.FileCountFromDiff(body),
		Publication: patches.PublicationPublished,
	}, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func TestExportPatchesSliceIsAbsentWithoutTheIncludeToken(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-absent"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	writeProposalPatch(t, repo, traceID, "agent-1", "diff --git a/x b/x\n+added\n", time.Now().UTC())

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "## Proposal patches") {
		t.Fatalf("the patches slice must be opt-in:\n%s", body)
	}
}

// The slice is filesystem-only, deterministic, and self-describing from the sidecars. It
// carries the metadata that explains a record without the event stream.
func TestExportPatchesSliceIsSelfDescribing(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-present"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	writeProposalPatch(t, repo, traceID, "agent-1", "diff --git a/x b/x\n+added\n", time.Date(2026, 1, 9, 10, 15, 0, 0, time.UTC))

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"## Proposal patches",
		"agent-1",
		"builder",
		"code.proposal.isolated",
		"9f2c1ab",
		"add the export flag",
		"published",
		"diff --git a/x b/x",
		"+added",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
}

// A trail that predates the archive reads as an empty slice rather than an error.
func TestExportPatchesSliceOnATrailWithoutPatches(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-empty"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "No proposal patches archived for this trail.") {
		t.Fatalf("empty slice should say so:\n%s", body)
	}
}

// An oversized body keeps its metadata, ref, and size, and says why the body was withheld.
// It never disappears from the report.
func TestExportPatchesSliceListsOversizedBodiesWithAReason(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-oversized"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	big := "diff --git a/big b/big\n" + strings.Repeat("+x\n", maxInlinePatchBytes)
	writeProposalPatch(t, repo, traceID, "agent-big", big, time.Now().UTC())

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "agent-big") {
		t.Fatalf("an omitted record must keep its metadata:\n%s", text)
	}
	if !strings.Contains(text, "too large for inline export") {
		t.Fatalf("an omitted record must say why:\n%s", text)
	}
}

// One malformed record does not make valid records disappear; it is reported beside them.
func TestExportPatchesSliceReportsIncompleteRecordsWithoutHidingOthers(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-diagnostics"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	writeProposalPatch(t, repo, traceID, "agent-good", "diff --git a/x b/x\n+ok\n", time.Now().UTC())
	if err := os.WriteFile(patches.NewStore(repo, traceID).MetaPath("agent-broken"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "agent-good") || !strings.Contains(text, "+ok") {
		t.Fatalf("a valid record disappeared:\n%s", text)
	}
	if !strings.Contains(text, "agent-broken") || !strings.Contains(text, "Incomplete patch records") {
		t.Fatalf("the diagnostic is missing:\n%s", text)
	}
}

func TestRenderHTMLPatchesSection(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-html"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	writeProposalPatch(t, repo, traceID, "agent-1", "diff --git a/x b/x\n+<script>alert(1)</script>\n", time.Now().UTC())

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatHTML,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "Proposal patches") || !strings.Contains(text, "agent-1") {
		t.Fatalf("HTML report missing the patches section:\n%s", text)
	}
	// Provenance is independent of a task id: a HITL record without a task still names its
	// baseline in the report.
	if !strings.Contains(text, "9f2c1ab") {
		t.Fatalf("HTML report dropped the base SHA of a record without a task id:\n%s", text)
	}
	// A patch body is repository content, not markup: the report must escape it.
	if strings.Contains(text, "<script>alert(1)</script>") {
		t.Fatalf("patch content was interpreted as markup:\n%s", text)
	}
	if !strings.Contains(text, "&lt;script&gt;") {
		t.Fatalf("patch content was not escaped:\n%s", text)
	}
}

// The aggregate budget bounds raw inlined body bytes across one report, so a long history
// of large patches cannot make the report unbounded. Recent records are preferred for body
// inclusion; the ones that fall outside the budget stay listed with a reason.
func TestExportPatchesAggregateBudgetBoundsInlinedBytes(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-patches-budget"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	// Each body fits the per-patch cap; enough of them together exceed the aggregate.
	perBody := 500 * 1024
	body := "diff --git a/x b/x\n" + strings.Repeat("+y\n", perBody/3)
	needed := maxAggregateInlinePatchBytes/perBody + 1
	base := time.Date(2026, 1, 9, 10, 0, 0, 0, time.UTC)
	for i := range needed {
		writeProposalPatch(t, repo, traceID, fmt.Sprintf("agent-%02d", i), body, base.Add(time.Duration(i)*time.Minute))
	}

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "patches"),
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(file)
	if !strings.Contains(text, "aggregate inline patch budget") {
		t.Fatalf("expected an aggregate-budget omission:\n%s", text)
	}
	// The oldest record is the one dropped first, and it is still listed by metadata.
	if !strings.Contains(text, "agent-00") {
		t.Fatalf("an omitted record must still be listed:\n%s", text)
	}
	if len(text) > 2*maxAggregateInlinePatchBytes {
		t.Fatalf("report grew past the aggregate body budget: %d bytes", len(text))
	}
}
