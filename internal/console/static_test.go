package console

import (
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
