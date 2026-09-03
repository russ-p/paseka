package export

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func setupExportColony(t *testing.T) (colony.Context, string) {
	t.Helper()
	repo := t.TempDir()
	slug := "export-test"
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	homeDir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "config.yaml"), []byte("colony_root: "+repo+"\nslug: "+slug+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, "state.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return colony.Context{ColonyRoot: repo, Slug: slug}, repo
}

func writeCompletedRun(t *testing.T, repo, traceID string, started time.Time, usage *protocol.Usage) {
	t.Helper()
	writeCompletedRunCfg(t, repo, traceID, started, completedRunCfg{Usage: usage})
}

type completedRunCfg struct {
	AgentID           string
	Adapter           string
	ProviderSessionID string
	Usage             *protocol.Usage
}

func writeCompletedRunCfg(t *testing.T, repo, traceID string, started time.Time, cfg completedRunCfg) {
	t.Helper()
	agentID := cfg.AgentID
	if agentID == "" {
		agentID = "agent-1"
	}
	adapter := cfg.Adapter
	if adapter == "" {
		adapter = "cursor"
	}
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "scout",
		Adapter:         adapter,
		Workspace:       repo,
		ColonyRoot:      repo,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	finished := started.Add(90 * time.Second)
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion:   protocol.Version,
		TraceID:           traceID,
		AgentID:           agentID,
		Status:            protocol.StatusCompleted,
		Summary:           "done",
		Usage:             cfg.Usage,
		ProviderSessionID: cfg.ProviderSessionID,
		FinishedAt:        finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Seq:             1,
		Type:            protocol.EventInsight,
		CreatedAt:       started.Add(2 * time.Second),
		Payload:         []byte(`{"kind":"narrative","text":"hello export"}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOutputFilename(t *testing.T) {
	got := OutputFilename("my-project", "trace-abc123", FormatHTML)
	want := "paseka-export-my-project-trace-abc123.html"
	if got != want {
		t.Fatalf("OutputFilename() = %q, want %q", got, want)
	}
	got = OutputFilename("my-project", "trace-abc123", FormatMarkdown)
	want = "paseka-export-my-project-trace-abc123.md"
	if got != want {
		t.Fatalf("OutputFilename(md) = %q, want %q", got, want)
	}
}

func TestOutputFilenameSanitizesUnsafeChars(t *testing.T) {
	got := OutputFilename("org/repo", "trace:bad", FormatHTML)
	if strings.Contains(got, "/") || strings.Contains(got, ":") {
		t.Fatalf("OutputFilename() = %q, expected sanitized", got)
	}
}

func TestRenderHTMLContainsTraceData(t *testing.T) {
	data := TraceExportData{
		Slug:       "demo-hive",
		ColonyRoot: "/tmp/colony",
		ExportedAt: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
		Trace: hiveview.TraceDetailView{
			TraceSummaryView: hiveview.TraceSummaryView{
				TraceID:        "trace-test-1",
				LastActivityAt: time.Date(2026, 7, 10, 11, 0, 0, 0, time.UTC),
				RunCount:       1,
				TaskCount:      1,
			},
			Tasks: []hiveview.TaskSummaryView{{
				TaskID: "task-1",
				Title:  "Survey codebase",
				Status: "ready",
				Bee:    "scout",
			}},
		},
		Runs: []hiveview.RunView{{
			TraceID:   "trace-test-1",
			AgentID:   "agent-1",
			Bee:       "scout",
			State:     "completed",
			StartedAt: time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC),
		}},
		Events: []hiveview.EventFeedItem{{
			Type:        protocol.EventSignal,
			PayloadKind: "task.ready",
			CreatedAt:   time.Date(2026, 7, 10, 10, 5, 0, 0, time.UTC),
			TraceID:     "trace-test-1",
			AgentID:     "agent-1",
			Bee:         "scout",
			Summary:     "Task ready: Survey codebase",
		}},
	}

	html, err := RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	body := string(html)
	for _, want := range []string{
		"Paseka export 🐝",
		"trace-test-1",
		"demo-hive",
		"SIGNAL",
		"Survey codebase",
		"fonts.googleapis.com",
		"JetBrains Mono",
		"Show raw JSON",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
}

func TestExportTraceWritesFile(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-export"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	outDir := t.TempDir()
	path, err := ExportTrace(ctx, exportOptions(traceID, outDir))
	if err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	if !strings.HasSuffix(path, "paseka-export-"+ctx.Slug+"-"+traceID+".html") {
		t.Fatalf("unexpected path %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat output: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), traceID) {
		t.Fatalf("export file missing trace id")
	}
	if !strings.Contains(string(body), "INSIGHT") {
		t.Fatalf("export file missing event type")
	}
}

func exportOptions(traceID, outDir string) Options {
	return Options{TraceID: traceID, OutputDir: outDir, Format: FormatHTML}
}

func TestExportTraceWritesMarkdownFile(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-export-md"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	outDir := t.TempDir()
	path, err := ExportTrace(ctx, Options{TraceID: traceID, OutputDir: outDir, Format: FormatMarkdown})
	if err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	if !strings.HasSuffix(path, "paseka-export-"+ctx.Slug+"-"+traceID+".md") {
		t.Fatalf("unexpected path %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"# Paseka export",
		traceID,
		"## Overview",
		"## Tasks",
		"## Runs",
		"## Timeline",
		"INSIGHT",
		"```json",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("markdown export missing %q", want)
		}
	}
}

func TestExportTraceDefaultOmitsConfigSnapshots(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-default"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	pasekaDir := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(filepath.Join(pasekaDir, "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "colony.yaml"), []byte("slug: export-test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "bees", "scout.yaml"), []byte("role: scout\nadapter: cursor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "bees", "scout.local.yaml"), []byte("prompt_template: secret.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	path, err := ExportTrace(ctx, Options{TraceID: traceID, OutputDir: outDir, Format: FormatMarkdown})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range []string{
		"## Colony",
		"## Bees",
		"role: scout",
		"secret.local",
		"scout.local.yaml",
		"#### Agent log",
		"omitted:",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("default export should not contain %q:\n%s", forbidden, text)
		}
	}
}

func TestExportTraceIncludeUsageAndDurations(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-usage"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-2*time.Minute), &protocol.Usage{
		InputTokens:  120,
		OutputTokens: 30,
	})

	include, err := ParseInclude([]string{"usage", "durations"})
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: outDir,
		Format:    FormatMarkdown,
		Include:   include,
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
		"**Usage:** in 120 / out 30",
		"**Duration:**",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
}

func TestExportTraceIncludeBeesOmitsLocalOverlay(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-bees"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	pasekaDir := filepath.Join(repo, ".paseka")
	if err := os.MkdirAll(filepath.Join(pasekaDir, "bees"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "bees", "scout.yaml"), []byte("role: scout\nadapter: cursor\nprompt_template: prompts/scout.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pasekaDir, "bees", "scout.local.yaml"), []byte("prompt_template: prompts/local-only.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	include, err := ParseInclude([]string{"bees"})
	if err != nil {
		t.Fatal(err)
	}
	outDir := t.TempDir()
	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: outDir,
		Format:    FormatMarkdown,
		Include:   include,
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
		"## Bees",
		"prompts/scout.md",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{
		"local-only.md",
		"scout.local.yaml",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("export should not contain %q:\n%s", forbidden, text)
		}
	}
}

func mustInclude(t *testing.T, flags ...string) IncludeSet {
	t.Helper()
	include, err := ParseInclude(flags)
	if err != nil {
		t.Fatal(err)
	}
	return include
}

func TestExportTraceIncludeColonyYAML(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-colony"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	if err := os.MkdirAll(filepath.Join(repo, ".paseka"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "colony.yaml"), []byte("slug: export-test\ndefaults:\n  default_bee: scout\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: outDir,
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "colony"),
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
		"## Colony",
		"```yaml",
		"default_bee: scout",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "No colony.yaml found") {
		t.Fatalf("unexpected missing note:\n%s", text)
	}
}

func TestExportTraceIncludeColonyMissing(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-colony-missing"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "colony"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "## Colony") || !strings.Contains(text, "_No colony.yaml found._") {
		t.Fatalf("missing colony note:\n%s", text)
	}
	if strings.Contains(text, "```yaml\n_No colony.yaml found._") {
		t.Fatalf("missing note should not be fenced as yaml:\n%s", text)
	}
}

func TestExportTraceIncludeColonyReadError(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-colony-err"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "colony.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "colony"),
	})
	if err == nil {
		t.Fatal("expected error when colony.yaml is a directory")
	}
}

func TestExportTraceIncludeArtifacts(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-artifacts-export"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(comb, "brief.md"), []byte("# Brief\ninline"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "artifacts"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"## Trail artifacts", "brief.md", "# Brief", "inline"} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
}

func TestExportTraceIncludeCues(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-cues"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)
	if err := os.MkdirAll(filepath.Join(repo, ".paseka", "cues"), 0o755); err != nil {
		t.Fatal(err)
	}
	cue := `description: Intake
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
`
	if err := os.WriteFile(filepath.Join(repo, ".paseka", "cues", "feature.yaml"), []byte(cue), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "cues"),
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
		"## Cues",
		"### feature",
		"kind: feature.requested",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("export missing %q:\n%s", want, text)
		}
	}
}

func TestExportTraceIncludeCuesEmpty(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-cues-empty"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "cues"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "## Cues") || !strings.Contains(text, "_No cues found._") {
		t.Fatalf("empty cues note missing:\n%s", text)
	}
}

func TestExportTraceIncludeBeesEmpty(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-bees-empty"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	path, err := ExportTrace(ctx, Options{
		TraceID:   traceID,
		OutputDir: t.TempDir(),
		Format:    FormatMarkdown,
		Include:   mustInclude(t, "bees"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "## Bees") || !strings.Contains(text, "_No bee YAML found._") {
		t.Fatalf("empty bees note missing:\n%s", text)
	}
}

func TestRenderHTMLIncludeSections(t *testing.T) {
	data := TraceExportData{
		Slug: "demo",
		Trace: hiveview.TraceDetailView{
			TraceSummaryView: hiveview.TraceSummaryView{TraceID: "trace-1"},
		},
		Include: mustInclude(t, "bees", "colony", "cues"),
		BeeYAML: []NamedYAML{{Name: "scout", Path: ".paseka/bees/scout.yaml", Content: "role: scout\n"}},
		ColonyYAML: &NamedYAML{
			Name:    "colony.yaml",
			Path:    ".paseka/colony.yaml",
			Content: "slug: demo\n",
		},
		CueYAML: []NamedYAML{{Name: "feature", Path: ".paseka/cues/feature.yaml", Content: "emit: signal\n"}},
	}
	html, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	body := string(html)
	for _, want := range []string{
		"<h2>Bees</h2>",
		"role: scout",
		"<h2>Colony</h2>",
		"slug: demo",
		"<h2>Cues</h2>",
		"emit: signal",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
}

func TestRenderHTMLColonyMissingNote(t *testing.T) {
	data := TraceExportData{
		Slug: "demo",
		Trace: hiveview.TraceDetailView{
			TraceSummaryView: hiveview.TraceSummaryView{TraceID: "trace-1"},
		},
		Include: mustInclude(t, "colony"),
		ColonyYAML: &NamedYAML{
			Name:    "colony.yaml",
			Path:    ".paseka/colony.yaml",
			Missing: true,
		},
	}
	html, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	body := string(html)
	if !strings.Contains(body, "No colony.yaml found.") {
		t.Fatalf("HTML missing missing-file note: %s", body)
	}
	if strings.Contains(body, "<pre class=\"timeline-raw\">_No colony.yaml found._</pre>") {
		t.Fatalf("missing note should not be in yaml pre: %s", body)
	}
}

type fakeSessionLogResolver struct {
	log adapters.SessionLog
	err error
}

func (f fakeSessionLogResolver) ResolveSessionLog(_ context.Context, _ adapters.SessionLogRequest) (adapters.SessionLog, error) {
	return f.log, f.err
}

func exportAgentLogs(t *testing.T, ctx colony.Context, traceID string, format Format, lookup SessionLogLookup) string {
	t.Helper()
	path, err := ExportTrace(ctx, Options{
		TraceID:     traceID,
		OutputDir:   t.TempDir(),
		Format:      format,
		Include:     mustInclude(t, "agent-logs"),
		SessionLogs: lookup,
	})
	if err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestExportTraceIncludeAgentLogsOmitsWithoutProviderSessionID(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-agent-log-noid"
	writeCompletedRun(t, repo, traceID, time.Now().UTC().Add(-time.Minute), nil)

	text := exportAgentLogs(t, ctx, traceID, FormatMarkdown, nil)
	if !strings.Contains(text, "#### Agent log") {
		t.Fatalf("missing Agent log:\n%s", text)
	}
	if !strings.Contains(text, "_omitted: no providerSessionId_") {
		t.Fatalf("missing omit reason:\n%s", text)
	}
}

func TestExportTraceIncludeAgentLogsCursorStoreNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	ctx, repo := setupExportColony(t)
	traceID := "trace-agent-log-cursor-miss"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
	})

	md := exportAgentLogs(t, ctx, traceID, FormatMarkdown, nil)
	if !strings.Contains(md, "_omitted: store not found_") {
		t.Fatalf("cursor store omit missing:\n%s", md)
	}
}

func TestExportTraceIncludeAgentLogsPiStubNotImplemented(t *testing.T) {
	ctx, repo := setupExportColony(t)
	writeCompletedRunCfg(t, repo, "trace-agent-log-pi", time.Now().UTC().Add(-time.Minute), completedRunCfg{
		AgentID:           "agent-pi",
		Adapter:           "pi",
		ProviderSessionID: "pi-session",
	})
	piMD := exportAgentLogs(t, ctx, "trace-agent-log-pi", FormatMarkdown, nil)
	if !strings.Contains(piMD, "_omitted: not implemented_") {
		t.Fatalf("pi stub omit missing:\n%s", piMD)
	}
}

func TestExportTraceIncludeAgentLogsUnsupportedAdapter(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-agent-log-script"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		Adapter:           "script",
		ProviderSessionID: "sess-1",
	})

	text := exportAgentLogs(t, ctx, traceID, FormatMarkdown, nil)
	if !strings.Contains(text, "_omitted: unsupported_") {
		t.Fatalf("unsupported omit missing:\n%s", text)
	}
}

func TestExportTraceIncludeAgentLogsResolverError(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-agent-log-err"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})

	text := exportAgentLogs(t, ctx, traceID, FormatMarkdown, func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{err: errors.New("store boom")}
	})
	if !strings.Contains(text, "_omitted: resolve error_") {
		t.Fatalf("resolve error omit missing:\n%s", text)
	}
}

func TestExportTraceIncludeAgentLogsFakeToolCalls(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-agent-log-tools"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		ProviderSessionID: "cursor-uuid",
	})

	lookup := func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{log: adapters.SessionLog{
			ToolCalls: []adapters.ToolCall{{
				Name:   "Read",
				CallID: "call-abc",
				Args:   "docs/guide/cli.md",
			}},
		}}
	}
	md := exportAgentLogs(t, ctx, traceID, FormatMarkdown, lookup)
	for _, want := range []string{
		"#### Agent log",
		"| Read | docs/guide/cli.md | call-abc |",
		"## Timeline",
		"INSIGHT",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "TOOL_CALL") {
		t.Fatalf("timeline must not dump tool calls:\n%s", md)
	}

	html := exportAgentLogs(t, ctx, traceID, FormatHTML, lookup)
	for _, want := range []string{
		"<h3>Agent log</h3>",
		"Read",
		"call-abc",
		"docs/guide/cli.md",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
	if strings.Contains(html, "TOOL_CALL") {
		t.Fatalf("HTML timeline must not dump tool calls")
	}
}
