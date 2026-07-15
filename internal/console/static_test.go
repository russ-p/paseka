package console

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppJSUses24HourTimeFormat(t *testing.T) {
	data, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	src := string(data)
	if !strings.Contains(src, "function formatTime(iso)") {
		t.Fatal("formatTime helper missing from app.js")
	}
	if !strings.Contains(src, "hour12: false") {
		t.Fatal("formatTime must use 24-hour clock (hour12: false)")
	}
}

func TestAgentsPanelStaticContract(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	htmlSrc := string(html)
	for _, needle := range []string{
		`id="agents-panel"`,
		`id="agents-badge"`,
		`id="agents-meta"`,
		`id="agents-detail"`,
		`aria-label="Live bees"`,
	} {
		if !strings.Contains(htmlSrc, needle) {
			t.Fatalf("index.html missing %s", needle)
		}
	}

	js, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	jsSrc := string(js)
	for _, needle := range []string{
		"function renderAgents()",
		"api('/api/agents')",
		"function navigateAgentsPanel()",
		"setTab('runs')",
		"setTab('sessions')",
	} {
		if !strings.Contains(jsSrc, needle) {
			t.Fatalf("app.js missing %q", needle)
		}
	}

	navIdx := strings.Index(jsSrc, "function navigateAgentsPanel()")
	if navIdx < 0 {
		t.Fatal("navigateAgentsPanel missing")
	}
	navFn := jsSrc[navIdx:]
	if end := strings.Index(navFn, "\nfunction "); end > 0 {
		navFn = navFn[:end]
	}
	afkIdx := strings.Index(navFn, "ag.afk > 0")
	runsIdx := strings.Index(navFn, "setTab('runs')")
	sessIdx := strings.Index(navFn, "ag.sessions > 0")
	sessTabIdx := strings.Index(navFn, "setTab('sessions')")
	if afkIdx < 0 || runsIdx < 0 || sessIdx < 0 || sessTabIdx < 0 {
		t.Fatal("navigateAgentsPanel must check afk then sessions")
	}
	if !(afkIdx < runsIdx && runsIdx < sessIdx && sessIdx < sessTabIdx) {
		t.Fatal("smart-nav order must be: afk→runs, sessions→sessions, else runs")
	}
}

func TestTracesTabStaticContract(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	htmlSrc := string(html)
	for _, needle := range []string{
		`id="tab-traces"`,
		`id="traces-layout"`,
		`id="trace-list"`,
		`id="trace-detail-body"`,
		`id="trace-tasks-list"`,
		`id="trace-runs-list"`,
		`id="trace-events-list"`,
		`id="trace-open-timeline-btn"`,
	} {
		if !strings.Contains(htmlSrc, needle) {
			t.Fatalf("index.html missing %s", needle)
		}
	}

	js, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	jsSrc := string(js)
	for _, needle := range []string{
		"async function navigateToTrace(traceId)",
		"async function loadTraces()",
		"async function selectTrace(traceId",
		"function startTracesPolling()",
		"function renderTraceDetail(detail)",
		"api('/api/traces')",
		"`/api/traces/${encodeURIComponent(traceId)}`",
		"setTab('traces')",
		"navigateToTask(state.selectedTraceId, task.taskId)",
		"navigateToRun(run.traceId || state.selectedTraceId, run.agentId)",
		"navigateToTaskTimeline(state.selectedTraceId, null)",
	} {
		if !strings.Contains(jsSrc, needle) {
			t.Fatalf("app.js missing %q", needle)
		}
	}

	// Filters must be applied before setTab so the automatic timeline load uses them.
	navIdx := strings.Index(jsSrc, "function navigateToTaskTimeline(traceId, taskId)")
	if navIdx < 0 {
		t.Fatal("navigateToTaskTimeline missing")
	}
	navFn := jsSrc[navIdx:]
	if end := strings.Index(navFn, "\nfunction "); end > 0 {
		navFn = navFn[:end]
	}
	readIdx := strings.Index(navFn, "readTimelineFiltersFromForm()")
	setTabIdx := strings.Index(navFn, "setTab('timeline')")
	if readIdx < 0 || setTabIdx < 0 || readIdx > setTabIdx {
		t.Fatal("navigateToTaskTimeline must read filters before setTab('timeline')")
	}

	// Dashboard and insight links must open the Traces tab, not the Runs fallback.
	if !strings.Contains(jsSrc, "navigateToTrace(trace.traceId)") {
		t.Fatal("dashboard traces must call navigateToTrace")
	}
	if strings.Contains(jsSrc, "setTab('runs');\n  await loadRuns();\n  const match = state.runs.find((r) => r.traceId === traceId)") {
		t.Fatal("navigateToTrace must not fall back to Runs tab")
	}
	if !strings.Contains(jsSrc, "state.selectedTraceDetail = null") {
		t.Fatal("selectTrace must clear previous detail when switching traces")
	}
	if !strings.Contains(jsSrc, "const switching = state.selectedTraceId !== traceId") {
		t.Fatal("selectTrace must detect trace switches before clearing detail")
	}

	css, err := staticFiles.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	cssSrc := string(css)
	if !strings.Contains(cssSrc, "#traces-layout") {
		t.Fatal("style.css missing #traces-layout")
	}
	mediaIdx := strings.Index(cssSrc, "@media (max-width: 1100px)")
	if mediaIdx < 0 {
		t.Fatal("style.css missing responsive media query")
	}
	mediaBlock := cssSrc[mediaIdx:]
	if end := strings.Index(mediaBlock, "}"); end > 0 {
		mediaBlock = mediaBlock[:end]
	}
	if !strings.Contains(mediaBlock, "#traces-layout") {
		t.Fatal("responsive media query must collapse #traces-layout")
	}
}

func TestReviewMergeDiffStaticContract(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	htmlSrc := string(html)
	for _, needle := range []string{
		`id="review-merge-diff-wrap"`,
		`id="review-merge-diff-container"`,
		`/vendor/diff2html/diff2html.min.js`,
		`/vendor/diff2html/diff2html.min.css`,
	} {
		if !strings.Contains(htmlSrc, needle) {
			t.Fatalf("index.html missing %s", needle)
		}
	}

	js, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	jsSrc := string(js)
	for _, needle := range []string{
		"async function loadReviewMergeDiff(traceId)",
		"function reviewMergeDiffStillValid(traceId, token)",
		"function clearReviewMergeDiff()",
		"state.reviewMergeDiffToken += 1",
		"function renderReviewMergeDiff(view)",
		"`/api/traces/${encodeURIComponent(traceId)}/merge-diff`",
		"Diff2Html.html(view.diff",
	} {
		if !strings.Contains(jsSrc, needle) {
			t.Fatalf("app.js missing %q", needle)
		}
	}

	css, err := staticFiles.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	if !strings.Contains(string(css), ".merge-diff-container") {
		t.Fatal("style.css missing .merge-diff-container")
	}

	for _, path := range []string{
		"static/vendor/diff2html/diff2html.min.js",
		"static/vendor/diff2html/diff2html.min.css",
	} {
		if _, err := staticFiles.ReadFile(path); err != nil {
			t.Fatalf("missing vendored asset %s: %v", path, err)
		}
	}
}

func TestMermaidVendorStaticContract(t *testing.T) {
	const path = "static/vendor/mermaid/mermaid.min.js"
	data, err := staticFiles.ReadFile(path)
	if err != nil {
		t.Fatalf("missing vendored asset %s: %v", path, err)
	}
	src := string(data)
	if !strings.Contains(src, `globalThis["mermaid"]`) {
		t.Fatal("mermaid.min.js must expose globalThis[\"mermaid\"]")
	}
	if !strings.Contains(src, "11.15.0") {
		t.Fatal("mermaid.min.js must be version 11.15.0")
	}

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/vendor/mermaid/mermaid.min.js", nil)
	spaHandler(staticFS).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /vendor/mermaid/mermaid.min.js status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `globalThis["mermaid"]`) {
		t.Fatal("HTTP response must serve embedded mermaid bundle")
	}
}

func TestTopologyTabStaticContract(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	htmlSrc := string(html)
	for _, needle := range []string{
		`id="tab-topology"`,
		`id="topology-layout"`,
		`id="topology-diagram"`,
		`id="topology-copy-btn"`,
		`id="topology-refresh-btn"`,
		`/vendor/mermaid/mermaid.min.js`,
	} {
		if !strings.Contains(htmlSrc, needle) {
			t.Fatalf("index.html missing %s", needle)
		}
	}

	js, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	jsSrc := string(js)
	for _, needle := range []string{
		"async function loadTopology()",
		"function renderTopology()",
		"async function renderTopologyMermaid(mermaidSrc)",
		"async function copyTopologyMermaid()",
		"api('/api/colony/topology')",
		"mermaid.render(id, mermaidSrc)",
		"setTab('topology')",
		"navigator.clipboard.writeText(text)",
	} {
		if !strings.Contains(jsSrc, needle) {
			t.Fatalf("app.js missing %q", needle)
		}
	}

	css, err := staticFiles.ReadFile("static/style.css")
	if err != nil {
		t.Fatalf("read style.css: %v", err)
	}
	cssSrc := string(css)
	if !strings.Contains(cssSrc, "#topology-layout") {
		t.Fatal("style.css missing #topology-layout")
	}
	if !strings.Contains(cssSrc, ".topology-diagram") {
		t.Fatal("style.css missing .topology-diagram")
	}
}
