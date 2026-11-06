package runtime_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
