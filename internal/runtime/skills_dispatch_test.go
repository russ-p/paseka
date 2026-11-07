package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/adapters/script"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// writeSkillDispatchColony extends the standard dispatch fixture with a system
// template, a role that lists one skill, and that skill's store entry.
func writeSkillDispatchColony(t *testing.T, root string) {
	t.Helper()
	writeColony(t, root)
	files := map[string]string{
		".paseka/bees/builder.yaml": `role: builder
adapter: cursor
prompt_template: builder.md
system_template: builder-system.md
skills:
  - paseka-release
params:
  model: composer-2.5
  yolo: true
`,
		".paseka/prompts/builder-system.md": "You are the builder. Build the thing.\n",
		".paseka/skills/paseka-release/SKILL.md": `---
name: paseka-release
description: Run the colony release procedure.
---

RELEASE-BODY-MUST-NOT-BE-INLINED
`,
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDispatchAdvertisesAndRecordsSkills(t *testing.T) {
	root := t.TempDir()
	writeSkillDispatchColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skills",
		Task:       "ship it",
	}); err != nil {
		t.Fatal(err)
	}

	if got := rec.lastReq.Skills; len(got) != 1 || got[0] != "paseka-release" {
		t.Fatalf("launch request skills = %v", got)
	}

	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-skills", AgentID: rec.lastReq.AgentID}
	system, err := os.ReadFile(runDir.SystemPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(system)
	instructions := strings.Index(text, "You are the builder.")
	section := strings.Index(text, skills.SectionHeading)
	if instructions < 0 || section < 0 || section < instructions {
		t.Fatalf("system.txt does not carry the section after the role instructions:\n%s", text)
	}
	for _, want := range []string{"paseka-release", "Run the colony release procedure."} {
		if !strings.Contains(text, want) {
			t.Fatalf("system.txt missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "RELEASE-BODY-MUST-NOT-BE-INLINED") {
		t.Fatalf("system.txt inlined the skill body:\n%s", text)
	}
	if !strings.Contains(text, filepath.Join(".paseka", "skills", "paseka-release", skills.ManifestName)) {
		t.Fatalf("system.txt does not advertise the resolved SKILL.md path:\n%s", text)
	}

	req, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Skills) != 1 || req.Skills[0] != "paseka-release" {
		t.Fatalf("request.json skills = %v", req.Skills)
	}

	// The record survives a later role-list change: it names what this run was
	// offered, not what the role offers now.
	if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"),
		[]byte("role: builder\nadapter: cursor\nprompt_template: builder.md\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := runDir.ReadRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Skills) != 1 || again.Skills[0] != "paseka-release" {
		t.Fatalf("recorded skills changed with the role list: %v", again.Skills)
	}
}

func TestDispatchNoSystemTemplateUsesSectionAsWholePrompt(t *testing.T) {
	root := t.TempDir()
	writeSkillDispatchColony(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"), []byte(`role: builder
adapter: cursor
prompt_template: builder.md
skills:
  - paseka-release
params:
  model: composer-2.5
  yolo: true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skills-none",
		Task:       "ship it",
	}); err != nil {
		t.Fatal(err)
	}
	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-skills-none", AgentID: rec.lastReq.AgentID}
	system, err := os.ReadFile(runDir.SystemPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(system), skills.SectionHeading) {
		t.Fatalf("system.txt = %q, want the section as the whole prompt", string(system))
	}
}

// TestDispatchScriptBeeReceivesSkillManifest drives a real script bee through
// dispatch and reads the environment the child received. The manifest is data
// in both delivery modes, with the same name, description, and absolute frozen
// path the advertisement carries, so a script never depends on `load`.
func TestDispatchScriptBeeReceivesSkillManifest(t *testing.T) {
	for _, mode := range []string{"path", "command"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			writeSkillDispatchColony(t, root)

			outPath := filepath.Join(root, "manifest.json")
			scriptPath := filepath.Join(root, "capture.sh")
			body := "#!/bin/sh\n" +
				"printf '%s' \"$PASEKA_SKILLS\" > \"" + outPath + "\"\n" +
				"echo done\n"
			if err := os.WriteFile(scriptPath, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}

			colonyDoc := "defaults:\n  prompt_template: default.md\n"
			if mode == "command" {
				colonyDoc += "  skills_delivery: command\n"
			}
			if err := os.WriteFile(filepath.Join(root, ".paseka", "colony.yaml"), []byte(colonyDoc), 0o644); err != nil {
				t.Fatal(err)
			}
			beeDoc := "role: builder\n" +
				"adapter: script\n" +
				"command:\n  - " + scriptPath + "\n" +
				"system_template: builder-system.md\n" +
				"skills:\n  - paseka-release\n"
			if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"), []byte(beeDoc), 0o644); err != nil {
				t.Fatal(err)
			}

			d := runtime.NewDispatcher()
			d.RegisterAdapter("script", script.New())
			if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
				ColonyRoot: root,
				Bee:        "builder",
				TraceID:    "trace-script-skills",
				Task:       "ship it",
			}); err != nil {
				t.Fatal(err)
			}

			data, err := os.ReadFile(outPath)
			if err != nil {
				t.Fatal(err)
			}
			var got []adapters.SkillRef
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("PASEKA_SKILLS is not JSON: %v\n%s", err, data)
			}
			wantPath := filepath.Join(root, ".paseka", "skills", "paseka-release", skills.ManifestName)
			if len(got) != 1 {
				t.Fatalf("manifest = %+v, want one entry", got)
			}
			if got[0].Name != "paseka-release" || got[0].Description != "Run the colony release procedure." {
				t.Fatalf("manifest entry = %+v", got[0])
			}
			if got[0].Path != wantPath || !filepath.IsAbs(got[0].Path) {
				t.Fatalf("manifest path = %q, want absolute %q", got[0].Path, wantPath)
			}

			agentID := ""
			entries, err := os.ReadDir(filepath.Join(root, ".paseka", "runs", "trace-script-skills"))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if _, err := os.Stat(filepath.Join(root, ".paseka", "runs", "trace-script-skills", entry.Name(), "request.json")); err == nil {
					agentID = entry.Name()
				}
			}
			if agentID == "" {
				t.Fatal("run directory not found")
			}
			system, err := os.ReadFile(runs.Dir{ColonyRoot: root, TraceID: "trace-script-skills", AgentID: agentID}.SystemPath())
			if err != nil {
				t.Fatal(err)
			}
			hasPath := strings.Contains(string(system), "path: ")
			if mode == "path" && !hasPath {
				t.Fatalf("path mode advertisement has no path:\n%s", system)
			}
			if mode == "command" && hasPath {
				t.Fatalf("command mode advertisement leaked a path:\n%s", system)
			}
			if !strings.Contains(string(system), "paseka-release") {
				t.Fatalf("advertisement is missing the skill:\n%s", system)
			}
		})
	}
}

func TestDispatchRunWithNoSkillsHasNoSection(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skills-empty",
		Task:       "ship it",
	}); err != nil {
		t.Fatal(err)
	}
	if len(rec.lastReq.Skills) != 0 {
		t.Fatalf("skills = %v", rec.lastReq.Skills)
	}
	if _, err := os.Stat(runs.Dir{ColonyRoot: root, TraceID: "trace-skills-empty", AgentID: rec.lastReq.AgentID}.SystemPath()); !os.IsNotExist(err) {
		t.Fatalf("a run with no system template and no skills must have no system.txt: %v", err)
	}
}

func TestDispatchFailsFastBeforeAdapter(t *testing.T) {
	root := t.TempDir()
	writeSkillDispatchColony(t, root)
	if err := os.WriteFile(filepath.Join(root, ".paseka", "bees", "builder.yaml"), []byte(`role: builder
adapter: cursor
prompt_template: builder.md
skills:
  - paseka-missing
params:
  model: composer-2.5
  yolo: true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	_, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skills-bad",
		Task:       "ship it",
	})
	if err == nil {
		t.Fatal("an invalid skill set must stop the run")
	}
	if !strings.Contains(err.Error(), "paseka-missing") {
		t.Fatalf("error does not name the reference: %v", err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter ran %d time(s) for an invalid skill set", rec.calls)
	}
}

func TestDispatchReadsSkillsFromTheFrozenTrail(t *testing.T) {
	root := t.TempDir()
	writeSkillDispatchColony(t, root)

	ctx := colony.Context{ColonyRoot: root}
	if _, err := trailconfig.Ensure(ctx, "trace-skills-frozen"); err != nil {
		t.Fatal(err)
	}
	bound, _, err := trailconfig.Bind(ctx, "trace-skills-frozen")
	if err != nil {
		t.Fatal(err)
	}

	// A store edit after trail start must not change the run's set.
	if err := os.WriteFile(filepath.Join(root, ".paseka", "skills", "paseka-release", skills.ManifestName),
		[]byte("---\nname: paseka-release\ndescription: Changed after trail start.\n---\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".paseka", "skills", "paseka-extra"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka", "skills", "paseka-extra", skills.ManifestName),
		[]byte("---\nname: paseka-extra\ndescription: Added after trail start.\n---\nextra\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)
	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot:     root,
		ConfigRoot:     bound.ConfigRoot,
		ConfigRef:      bound.ConfigRef,
		ConfigRevision: bound.ConfigRevision,
		Bee:            "builder",
		TraceID:        "trace-skills-frozen",
		Task:           "ship it",
	}); err != nil {
		t.Fatal(err)
	}
	if got := rec.lastReq.Skills; len(got) != 1 || got[0] != "paseka-release" {
		t.Fatalf("skills = %v", got)
	}
	system, err := os.ReadFile(runs.Dir{ColonyRoot: root, TraceID: "trace-skills-frozen", AgentID: rec.lastReq.AgentID}.SystemPath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(system), "Changed after trail start.") || strings.Contains(string(system), "paseka-extra") {
		t.Fatalf("system.txt followed the live store instead of the frozen trail:\n%s", system)
	}
	if !strings.Contains(string(system), "Run the colony release procedure.") {
		t.Fatalf("system.txt does not carry the frozen description:\n%s", system)
	}
}

// skillLoadAdapter writes the load records a real `paseka skills load` would
// append while the adapter is running, so finalization folds them into the run
// result exactly as it would for an agent that loaded procedures.
type skillLoadAdapter struct {
	lastReq adapters.RunRequest
	loads   []skills.UsageRecord
}

func (a *skillLoadAdapter) Name() string { return "cursor" }

func (a *skillLoadAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	a.lastReq = req
	dir := runs.Dir{ColonyRoot: req.ColonyRoot, TraceID: req.TraceID, AgentID: req.AgentID}
	for _, record := range a.loads {
		if err := skills.AppendUsage(dir.SkillsUsedPath(), record); err != nil {
			return nil, err
		}
	}
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func TestFinalizeRecordsTheDeduplicatedUsedSet(t *testing.T) {
	root := t.TempDir()
	writeSkillDispatchColony(t, root)

	adapter := &skillLoadAdapter{loads: []skills.UsageRecord{
		{Name: "paseka-release", ContentHash: "first", At: "2026-11-07T10:00:00Z"},
		{Name: "legacy", ContentHash: "legacy", At: "2026-11-07T10:00:01Z"},
		{Name: "paseka-release", ContentHash: "second", At: "2026-11-07T10:00:02Z"},
	}}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", adapter)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-skills-used",
		Task:       "ship it",
	}); err != nil {
		t.Fatal(err)
	}

	runDir := runs.Dir{ColonyRoot: root, TraceID: "trace-skills-used", AgentID: adapter.lastReq.AgentID}
	res, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"paseka-release", "legacy"}
	if len(res.SkillsUsed) != len(want) {
		t.Fatalf("skillsUsed = %v, want the deduplicated set %v", res.SkillsUsed, want)
	}
	for i := range want {
		if res.SkillsUsed[i] != want[i] {
			t.Fatalf("skillsUsed = %v, want first-load order %v", res.SkillsUsed, want)
		}
	}

	// One record per load is kept; folding deduplicates the result, not the log.
	records, err := skills.ReadUsage(runDir.SkillsUsedPath())
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("load log holds %d records, want one per load", len(records))
	}
	if records[0].ContentHash != "first" || records[2].ContentHash != "second" {
		t.Fatalf("load log = %+v, want every load kept in order", records)
	}
}
