package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
)

func exportFrozenSkillDir(repo, traceID, ref, name string) string {
	return filepath.Join(repo, ".paseka", "runs", traceID, "config", "snapshots", ref,
		trailconfig.SnapshotTreeDirName, ".paseka", "skills", name)
}

// usageTranscriptResolver resolves a different transcript per provider session,
// which is how a trace with several runs is exercised without sharing evidence.
type usageTranscriptResolver struct {
	bySession map[string]adapters.Transcript
}

func (r usageTranscriptResolver) ResolveTranscript(_ context.Context, req adapters.TranscriptRequest) (adapters.Transcript, error) {
	return r.bySession[req.ProviderSessionID], nil
}

func usageLookup(bySession map[string]adapters.Transcript) TranscriptLookup {
	return func(string) adapters.TranscriptResolver {
		return usageTranscriptResolver{bySession: bySession}
	}
}

func exportSkills(t *testing.T, ctx colony.Context, traceID string, format Format, lookup TranscriptLookup) string {
	t.Helper()
	path, err := ExportTrace(ctx, Options{
		TraceID:     traceID,
		OutputDir:   t.TempDir(),
		Format:      format,
		Include:     mustInclude(t, "skills"),
		Transcripts: lookup,
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

func TestExportSkillsUsedFromTranscript(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-transcript"
	writeSkillsExportColony(t, repo, []string{"alpha"}, map[string]string{
		"alpha": "Alpha procedure.",
	})
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}

	started := time.Now().UTC().Add(-3 * time.Minute)
	writeCompletedRunCfg(t, repo, traceID, started, completedRunCfg{
		AgentID: "agent-read", ConfigRef: manifest.InitialConfigRef, ConfigRevision: manifest.ConfigRevision,
		ProviderSessionID: "sess-read",
	})
	writeCompletedRunCfg(t, repo, traceID, started.Add(time.Minute), completedRunCfg{
		AgentID: "agent-none", ConfigRef: manifest.InitialConfigRef, ConfigRevision: manifest.ConfigRevision,
		ProviderSessionID: "sess-none",
	})

	frozen := exportFrozenSkillDir(repo, traceID, manifest.InitialConfigRef, "alpha")
	lookup := usageLookup(map[string]adapters.Transcript{
		"sess-read": toolCallTranscript(
			adapters.ToolCall{Name: "Read", CallID: "call-skill", Args: `{"path":"` + filepath.Join(frozen, skills.ManifestName) + `"}`},
			adapters.ToolCall{Name: "Bash", CallID: "call-script", Args: `sh ` + filepath.Join(frozen, "scripts", "run.sh")},
			adapters.ToolCall{Name: "Read", CallID: "call-other", Args: `{"path":"` + filepath.Join(repo, "README.md") + `"}`},
		),
		"sess-none": toolCallTranscript(
			adapters.ToolCall{Name: "Read", CallID: "call-other", Args: `{"path":"` + filepath.Join(repo, "README.md") + `"}`},
		),
	})

	md := exportSkills(t, ctx, traceID, FormatMarkdown, lookup)
	for _, want := range []string{
		"## Skills used",
		"agent-read",
		"**alpha**",
		"transcript: Read read " + skills.ManifestName,
		"transcript: Bash read scripts/run.sh",
		"alpha body",
		"none — the run's transcript and load records reference no skills.",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown used-skills export missing %q:\n%s", want, md)
		}
	}

	html := exportSkills(t, ctx, traceID, FormatHTML, lookup)
	for _, want := range []string{
		"<h2>Skills used</h2>",
		"agent-read",
		"<strong>alpha</strong>",
		"transcript: Read read " + skills.ManifestName,
		"transcript: Bash read scripts/run.sh",
		"alpha body",
		"none — the run's transcript and load records reference no skills.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html used-skills export missing %q:\n%s", want, html)
		}
	}
}

func TestExportSkillsUsedFromCommandModeLoadRecord(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-load"
	writeSkillsExportColony(t, repo, []string{"alpha"}, map[string]string{
		"alpha": "Alpha procedure.",
	})
	// Command mode is the delivery that makes a load deterministic, so the run
	// attributes from its own load record even with no transcript capability.
	colonyPath := filepath.Join(repo, ".paseka", "colony.yaml")
	if err := os.WriteFile(colonyPath, []byte("slug: export-test\ndefaults:\n  skills_delivery: command\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}

	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		AgentID: "agent-load", Adapter: "script", ConfigRef: manifest.InitialConfigRef,
		ConfigRevision: manifest.ConfigRevision,
	})
	runDir := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-load"}
	if err := skills.AppendUsage(runDir.SkillsUsedPath(), skills.UsageRecord{
		Name: "alpha", ContentHash: strings.Repeat("a", 64), At: "2026-11-06T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	md := exportSkills(t, ctx, traceID, FormatMarkdown, nil)
	for _, want := range []string{
		"## Skills used",
		"agent-load",
		"**alpha**",
		"load record: served at 2026-11-06T10:00:00Z",
		"alpha body",
		"transcript usage unavailable (no providerSessionId)",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("command-mode used export missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "used skills unavailable") {
		t.Fatalf("a run with a load record must not read as unavailable:\n%s", md)
	}

	html := exportSkills(t, ctx, traceID, FormatHTML, nil)
	for _, want := range []string{
		"<h2>Skills used</h2>",
		"<strong>alpha</strong>",
		"load record: served at 2026-11-06T10:00:00Z",
		"alpha body",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("command-mode html used export missing %q:\n%s", want, html)
		}
	}
}

func TestExportSkillsUsedBodyComesFromFrozenCopy(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-frozen-body"
	writeSkillsExportColony(t, repo, []string{"alpha"}, map[string]string{
		"alpha": "Alpha procedure.",
	})
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		AgentID: "agent-frozen", Adapter: "script", ConfigRef: manifest.InitialConfigRef,
		ConfigRevision: manifest.ConfigRevision,
	})
	runDir := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-frozen"}
	if err := skills.AppendUsage(runDir.SkillsUsedPath(), skills.UsageRecord{
		Name: "alpha", ContentHash: "hash", At: "2026-11-06T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	// The live store changes after the trail started. The export must still read
	// the body the run was offered, not the edit.
	live := filepath.Join(repo, ".paseka", "skills", "alpha", skills.ManifestName)
	if err := os.WriteFile(live, []byte("---\nname: alpha\ndescription: Alpha procedure.\n---\n\nchanged after the trail\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	md := exportSkills(t, ctx, traceID, FormatMarkdown, nil)
	if !strings.Contains(md, "alpha body") {
		t.Fatalf("used body did not come from the frozen copy:\n%s", md)
	}
	if strings.Contains(md, "changed after the trail") {
		t.Fatalf("used body leaked from the live store:\n%s", md)
	}
}

func TestExportSkillsUsedUnionDeduplicatesAndFirstLoadWins(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-union"
	writeSkillsExportColony(t, repo, []string{"alpha", "beta"}, map[string]string{
		"alpha": "Alpha procedure.",
		"beta":  "Beta procedure.",
	})
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		AgentID: "agent-union", ConfigRef: manifest.InitialConfigRef, ConfigRevision: manifest.ConfigRevision,
		ProviderSessionID: "sess-union",
	})

	frozen := exportFrozenSkillDir(repo, traceID, manifest.InitialConfigRef, "alpha")
	lookup := usageLookup(map[string]adapters.Transcript{
		"sess-union": toolCallTranscript(
			adapters.ToolCall{Name: "Read", CallID: "call-alpha", Args: `{"path":"` + filepath.Join(frozen, skills.ManifestName) + `"}`},
		),
	})

	runDir := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-union"}
	for _, record := range []skills.UsageRecord{
		{Name: "alpha", ContentHash: "first-alpha", At: "2026-11-06T10:00:00Z"},
		{Name: "alpha", ContentHash: "second-alpha", At: "2026-11-06T11:00:00Z"},
		{Name: "beta", ContentHash: "first-beta", At: "2026-11-06T12:00:00Z"},
	} {
		if err := skills.AppendUsage(runDir.SkillsUsedPath(), record); err != nil {
			t.Fatal(err)
		}
	}

	md := exportSkills(t, ctx, traceID, FormatMarkdown, lookup)
	section := md[strings.Index(md, "## Skills used"):]
	if got := strings.Count(section, "**alpha**"); got != 1 {
		t.Fatalf("alpha appears %d times, want once:\n%s", got, section)
	}
	if got := strings.Count(section, "**beta**"); got != 1 {
		t.Fatalf("beta appears %d times, want once:\n%s", got, section)
	}
	if !strings.Contains(section, "transcript: Read read "+skills.ManifestName) {
		t.Fatalf("transcript evidence missing from the union:\n%s", section)
	}
	if !strings.Contains(section, "load record: served at 2026-11-06T10:00:00Z") {
		t.Fatalf("first load evidence missing:\n%s", section)
	}
	if strings.Contains(section, "2026-11-06T11:00:00Z") || strings.Contains(section, "second-alpha") {
		t.Fatalf("a repeat load must not add evidence:\n%s", section)
	}
	if !strings.Contains(section, "beta body") {
		t.Fatalf("beta body missing from the frozen copy:\n%s", section)
	}
}

func TestExportSkillsUsedUnavailableReason(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-unavailable"
	writeSkillsExportColony(t, repo, []string{"alpha"}, map[string]string{
		"alpha": "Alpha procedure.",
	})
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-2 * time.Minute)
	writeCompletedRunCfg(t, repo, traceID, started, completedRunCfg{
		AgentID: "agent-unsupported", Adapter: "mystery", ConfigRef: manifest.InitialConfigRef,
		ConfigRevision: manifest.ConfigRevision, ProviderSessionID: "sess-mystery",
	})
	writeCompletedRunCfg(t, repo, traceID, started.Add(time.Minute), completedRunCfg{
		AgentID: "agent-no-session", ConfigRef: manifest.InitialConfigRef,
		ConfigRevision: manifest.ConfigRevision,
	})

	md := exportSkills(t, ctx, traceID, FormatMarkdown, nil)
	for _, want := range []string{
		"used skills unavailable: " + adapters.SessionLogOmittedUnsupported,
		"used skills unavailable: " + adapters.SessionLogOmittedNoProviderSessionID,
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("explicit unavailable reason missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "none — the run's transcript and load records reference no skills.") {
		t.Fatalf("absence of evidence must not read as absence of use:\n%s", md)
	}

	html := exportSkills(t, ctx, traceID, FormatHTML, nil)
	for _, want := range []string{
		"used skills unavailable: " + adapters.SessionLogOmittedUnsupported,
		"used skills unavailable: " + adapters.SessionLogOmittedNoProviderSessionID,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("html explicit unavailable reason missing %q:\n%s", want, html)
		}
	}
}

func TestExportSkillsUsedDoesNotLeakBetweenRuns(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-skills-used-isolation"
	writeSkillsExportColony(t, repo, []string{"alpha", "beta"}, map[string]string{
		"alpha": "Alpha procedure.",
		"beta":  "Beta procedure.",
	})
	manifest, err := trailconfig.Ensure(ctx, traceID)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-2 * time.Minute)
	for i, agentID := range []string{"agent-alpha", "agent-beta"} {
		writeCompletedRunCfg(t, repo, traceID, started.Add(time.Duration(i)*time.Minute), completedRunCfg{
			AgentID: agentID, ConfigRef: manifest.InitialConfigRef,
			ConfigRevision: manifest.ConfigRevision, ProviderSessionID: "sess-" + agentID,
		})
	}

	alphaDir := exportFrozenSkillDir(repo, traceID, manifest.InitialConfigRef, "alpha")
	betaDir := exportFrozenSkillDir(repo, traceID, manifest.InitialConfigRef, "beta")
	lookup := usageLookup(map[string]adapters.Transcript{
		"sess-agent-alpha": toolCallTranscript(
			adapters.ToolCall{Name: "Read", CallID: "call-alpha", Args: `{"path":"` + filepath.Join(alphaDir, skills.ManifestName) + `"}`},
		),
		"sess-agent-beta": toolCallTranscript(
			adapters.ToolCall{Name: "Read", CallID: "call-beta", Args: `{"path":"` + filepath.Join(betaDir, skills.ManifestName) + `"}`},
		),
	})

	md := exportSkills(t, ctx, traceID, FormatMarkdown, lookup)
	section := md[strings.Index(md, "## Skills used"):]
	alphaStart := strings.Index(section, "agent-alpha")
	betaStart := strings.Index(section, "agent-beta")
	if alphaStart < 0 || betaStart < 0 || alphaStart > betaStart {
		t.Fatalf("run blocks are not in order (alpha=%d beta=%d):\n%s", alphaStart, betaStart, section)
	}
	alphaBlock := section[alphaStart:betaStart]
	if !strings.Contains(alphaBlock, "**alpha**") || strings.Contains(alphaBlock, "**beta**") {
		t.Fatalf("agent-alpha carries another run's evidence:\n%s", alphaBlock)
	}
	betaBlock := section[betaStart:]
	if !strings.Contains(betaBlock, "**beta**") || strings.Contains(betaBlock, "**alpha**") {
		t.Fatalf("agent-beta carries another run's evidence:\n%s", betaBlock)
	}
}

func TestResolveRunUsageBodyOmission(t *testing.T) {
	repo := t.TempDir()
	traceID := "trace-skills-body-omission"
	runDir := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: "agent-omitted"}
	if err := skills.AppendUsage(runDir.SkillsUsedPath(), skills.UsageRecord{
		Name: "alpha", ContentHash: "hash", At: "2026-11-06T10:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	missing := filepath.Join(repo, "gone", "SKILL.md")
	usage := resolveRunUsage(repo, traceID, hiveview.RunView{
		AgentID: "agent-omitted", Adapter: "script",
	}, []skills.Skill{{
		Name: "alpha",
		Dir:  filepath.Dir(missing),
		Path: missing,
	}}, "", nil, 0)
	if len(usage.Used) != 1 {
		t.Fatalf("used = %+v", usage.Used)
	}
	if usage.Used[0].Body != "" || !strings.Contains(usage.Used[0].Omitted, "body unavailable") {
		t.Fatalf("unreadable frozen body did not record an omission: %+v", usage.Used[0])
	}

	data := TraceExportData{
		Include: mustInclude(t, "skills"),
		Skills: []RunSkillsExport{{
			AgentID: "agent-omitted",
			Used:    usage.Used,
		}},
	}
	md, err := RenderMarkdown(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "body omitted: body unavailable") {
		t.Fatalf("markdown missing body omission:\n%s", md)
	}
	html, err := RenderHTML(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(html), "body omitted: body unavailable") {
		t.Fatalf("html missing body omission:\n%s", html)
	}
}

func TestSkillFileInToolInputMatchesOnlyTheFrozenLocation(t *testing.T) {
	root := t.TempDir()
	skill := skills.Skill{
		Name: "alpha",
		Dir:  filepath.Join(root, "runs", "trace-a", "skills", "alpha"),
		Path: filepath.Join(root, "runs", "trace-a", "skills", "alpha", skills.ManifestName),
	}
	for _, tc := range []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "skill manifest", raw: `{"path":"` + skill.Path + `"}`, want: skills.ManifestName, ok: true},
		{name: "script", raw: `cat ` + filepath.Join(skill.Dir, "scripts", "run.sh"), want: "scripts/run.sh", ok: true},
		{name: "quoted script with space", raw: `{"path":"` + filepath.Join(skill.Dir, "scripts", "run tests.sh") + `"}`, want: "scripts/run tests.sh", ok: true},
		{name: "reference", raw: `{"file":"` + filepath.Join(skill.Dir, "references", "checklist.md") + `"}`, want: "references/checklist.md", ok: true},
		{name: "asset", raw: `{"file":"` + filepath.Join(skill.Dir, "assets", "logo.png") + `"}`, want: "assets/logo.png", ok: true},
		{name: "unrelated file in dir", raw: `{"path":"` + filepath.Join(skill.Dir, "notes.md") + `"}`, ok: false},
		{name: "other trace copy", raw: `{"path":"` + filepath.Join(root, "runs", "trace-b", "skills", "alpha", skills.ManifestName) + `"}`, ok: false},
		{name: "other skill", raw: `{"path":"` + filepath.Join(root, "runs", "trace-a", "skills", "beta", skills.ManifestName) + `"}`, ok: false},
		{name: "path escape", raw: `{"path":"` + filepath.Join(skill.Dir, "..", "beta", skills.ManifestName) + `"}`, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := skillFileInToolInput(tc.raw, skill, root)
			if ok != tc.ok || (tc.ok && got != tc.want) {
				t.Fatalf("skillFileInToolInput(%q) = (%q, %v), want (%q, %v)", tc.raw, got, ok, tc.want, tc.ok)
			}
		})
	}
}
