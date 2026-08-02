package cues_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/cues"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func writeCueFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".paseka", "cues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFeatureCue(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `description: Intake idea
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)

	cue, err := cues.Load(root, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if cue.ID != "feature" || cue.Emit != cues.EmitSignal {
		t.Fatalf("cue = %+v", cue)
	}
	if cue.SignalKind != "feature.requested" {
		t.Fatalf("kind = %q", cue.SignalKind)
	}
}

func TestLoadMissingCue(t *testing.T) {
	_, err := cues.Load(t.TempDir(), "feature")
	if err == nil || !strings.Contains(err.Error(), `cue "feature": not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadInvalidCue(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "broken.yaml", `emit: signal
type: SIGNAL
`)

	_, err := cues.Load(root, "broken")
	if err == nil || !strings.Contains(err.Error(), `cue "broken": kind is required`) {
		t.Fatalf("err = %v", err)
	}
}

func TestListSortedByID(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "zebra.yaml", `description: z
emit: signal
type: SIGNAL
kind: test.kind
title: "{{.Title}}"
`)
	writeCueFile(t, root, "alpha.yaml", `description: a
emit: signal
type: SIGNAL
kind: test.kind
title: "{{.Title}}"
`)

	items, err := cues.List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "alpha" || items[1].ID != "zebra" {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Description != "a" {
		t.Fatalf("description = %q", items[0].Description)
	}
}

func TestBuildSignalPayload(t *testing.T) {
	cue := cues.Cue{
		ID:            "feature",
		Emit:          cues.EmitSignal,
		SignalKind:    "feature.requested",
		TitleTemplate: "{{.Title}}",
		BodyTemplate:  "{{.Body}}",
		Static: map[string]string{
			"priority": "medium",
		},
	}
	ctx := cues.NewRenderContext("Live bees\nShow active bees in header.", "cli", "trace-1", nil)
	raw, err := cues.BuildSignalPayload(cue, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "feature.requested" {
		t.Fatalf("kind = %q", payload["kind"])
	}
	if payload["title"] != "Live bees" {
		t.Fatalf("title = %q", payload["title"])
	}
	if payload["body"] != "Live bees\nShow active bees in header." {
		t.Fatalf("body = %q", payload["body"])
	}
	if payload["source"] != "cli" {
		t.Fatalf("source = %q", payload["source"])
	}
	if payload["priority"] != "medium" {
		t.Fatalf("priority = %q", payload["priority"])
	}
}

func TestBuildSignalPayloadUserWinsOverStatic(t *testing.T) {
	cue := cues.Cue{
		ID:            "feature",
		Emit:          cues.EmitSignal,
		SignalKind:    "feature.requested",
		TitleTemplate: "{{.Title}}",
		BodyTemplate:  "{{.Body}}",
		Static: map[string]string{
			"title": "static title",
			"body":  "static body",
		},
	}
	ctx := cues.NewRenderContext("Operator title\nOperator body", "cli", "trace-1", nil)
	raw, err := cues.BuildSignalPayload(cue, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "Operator title" || payload["body"] != "Operator title\nOperator body" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestBuildSignalPayloadSetOverrides(t *testing.T) {
	cue := cues.Cue{
		ID:            "feature",
		Emit:          cues.EmitSignal,
		SignalKind:    "feature.requested",
		TitleTemplate: "{{.Title}}",
		BodyTemplate:  "{{.CustomBody}}",
	}
	ctx := cues.NewRenderContext("ignored", "cli", "trace-1", map[string]string{
		"CustomBody": "from --set",
		"Unused":     "ignored",
	})
	raw, err := cues.BuildSignalPayload(cue, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["body"] != "from --set" {
		t.Fatalf("body = %q", payload["body"])
	}
	if _, ok := payload["Unused"]; ok {
		t.Fatalf("unused --set leaked into payload: %#v", payload)
	}
}

func TestBuildSignalPayloadMissingTemplateKey(t *testing.T) {
	cue := cues.Cue{
		ID:           "feature",
		Emit:         cues.EmitSignal,
		SignalKind:   "feature.requested",
		BodyTemplate: "{{.MissingKey}}",
	}
	ctx := cues.NewRenderContext("hello", "cli", "trace-1", nil)
	_, err := cues.BuildSignalPayload(cue, ctx)
	if err == nil || !strings.Contains(err.Error(), `cue "feature"`) || !strings.Contains(err.Error(), "MissingKey") {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildSignalPayloadEmptyTextUsageError(t *testing.T) {
	cue := cues.Cue{
		ID:            "feature",
		Emit:          cues.EmitSignal,
		SignalKind:    "feature.requested",
		TitleTemplate: "{{.Title}}",
		BodyTemplate:  "{{.Body}}",
	}
	ctx := cues.NewRenderContext("", "cli", "trace-1", nil)
	_, err := cues.BuildSignalPayload(cue, ctx)
	if err == nil || !strings.Contains(err.Error(), `cue "feature": text is required`) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSetFlags(t *testing.T) {
	vars, err := cues.ParseSetFlags([]string{"foo=bar", "empty="})
	if err != nil {
		t.Fatal(err)
	}
	if vars["foo"] != "bar" || vars["empty"] != "" {
		t.Fatalf("vars = %#v", vars)
	}
	_, err = cues.ParseSetFlags([]string{"invalid"})
	if err == nil {
		t.Fatal("expected invalid --set error")
	}
}

type recordingPublisher struct {
	events []protocol.Event
}

func (p *recordingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.events = append(p.events, ev)
	return nil
}

func TestRunSignalPublishHappyPath(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `description: Intake idea
emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()

	res, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "feature",
		Text:       "OAuth callback\nAdd OAuth support",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "feature.requested" || res.EventType != "SIGNAL" {
		t.Fatalf("result = %+v", res)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d", len(pub.events))
	}
	ev := pub.events[0]
	if ev.Type != protocol.EventSignal || ev.AgentID != "cli" {
		t.Fatalf("event = %+v", ev)
	}
	var payload map[string]string
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["kind"] != "feature.requested" || payload["source"] != "cli" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload["title"] != "OAuth callback" {
		t.Fatalf("title = %q", payload["title"])
	}
}

func TestRunSignalWithTraceAndEnergyBudget(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "cheap.yaml", `emit: signal
type: SIGNAL
kind: feature.requested
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()

	_, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "cheap",
		Text:       "Fix login",
		TraceID:    "trace-existing",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-existing")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 3 {
		t.Fatalf("energy budget = %d, want 3", snap.EnergyBudget)
	}

	ledger2 := taskledger.NewMemoryLedger()
	_ = ledger2.SeedEnergy("trace-seeded", 10)
	_, err = cues.Run(context.Background(), pub, ledger2, cues.RunInput{
		ColonyRoot: root,
		CueID:      "cheap",
		Text:       "Fix login",
		TraceID:    "trace-seeded",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err = ledger2.Snapshot("trace-seeded")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 10 {
		t.Fatalf("seeded trace budget changed = %d", snap.EnergyBudget)
	}
}

func writeColonyManifest(t *testing.T, root string, energyBudget int) {
	t.Helper()
	dir := filepath.Join(root, ".paseka")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if energyBudget <= 0 {
		energyBudget = protocol.DefaultEnergyBudget
	}
	content := "slug: test-colony\ndefaults:\n  energy_budget: " + strconv.Itoa(energyBudget) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "colony.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadHotfixCue(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "hotfix.yaml", `description: Urgent fix via builder (small honey reserve)
emit: task
bee: builder
intent: bugfix
review: none
autorun: true
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
`)

	cue, err := cues.Load(root, "hotfix")
	if err != nil {
		t.Fatal(err)
	}
	if cue.Emit != cues.EmitTask || cue.Bee != "builder" || cue.Intent != "bugfix" {
		t.Fatalf("cue = %+v", cue)
	}
	if !cue.Autorun || cue.EnergyBudget != 3 {
		t.Fatalf("autorun/budget = %v/%d", cue.Autorun, cue.EnergyBudget)
	}
}

func TestLoadInvalidEnergyBudget(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"zero.yaml", "negative.yaml"} {
		var budget string
		if name == "zero.yaml" {
			budget = "0"
		} else {
			budget = "-1"
		}
		writeCueFile(t, root, name, `emit: signal
type: SIGNAL
kind: feature.requested
energy_budget: `+budget+`
title: "{{.Title}}"
`)
		id := strings.TrimSuffix(name, ".yaml")
		_, err := cues.Load(root, id)
		if err == nil || !strings.Contains(err.Error(), "energy_budget must be a positive integer") {
			t.Fatalf("load %s: err = %v", id, err)
		}
	}
}

func TestRunTaskPublishWithAutorun(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, protocol.DefaultEnergyBudget)
	writeCueFile(t, root, "hotfix.yaml", `emit: task
bee: builder
intent: bugfix
review: none
autorun: true
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()

	res, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Null pointer in login\nRepro: click submit with empty email",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TaskID == "" || res.Kind != string(protocol.TaskEventPlan) {
		t.Fatalf("result = %+v", res)
	}
	if len(pub.events) != 2 {
		t.Fatalf("events = %d, want plan+ready", len(pub.events))
	}
	if pub.events[0].Type != protocol.EventInsight {
		t.Fatalf("plan type = %s", pub.events[0].Type)
	}
	if pub.events[1].Type != protocol.EventSignal {
		t.Fatalf("ready type = %s", pub.events[1].Type)
	}
	var plan protocol.TaskPlanPayload
	if err := json.Unmarshal(pub.events[0].Payload, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].Bee != "builder" || plan.Tasks[0].Intent != "bugfix" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Tasks[0].Title != "Null pointer in login" {
		t.Fatalf("title = %q", plan.Tasks[0].Title)
	}

	var ready protocol.TaskReadyPayload
	if err := json.Unmarshal(pub.events[1].Payload, &ready); err != nil {
		t.Fatal(err)
	}
	if ready.Kind != protocol.TaskEventReady || ready.TaskID != res.TaskID {
		t.Fatalf("ready = %+v", ready)
	}
}

func TestRunTaskEnergyBudgetSeedsFreshTrace(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, 12)
	writeCueFile(t, root, "hotfix.yaml", `emit: task
bee: builder
intent: bugfix
autorun: false
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()

	_, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Fix crash",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(pub.events[0].TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 3 {
		t.Fatalf("energy budget = %d, want 3", snap.EnergyBudget)
	}
}

func TestRunTaskOmittedBudgetUsesColonyDefault(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, 9)
	writeCueFile(t, root, "task.yaml", `emit: task
bee: builder
intent: feature
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()

	_, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "task",
		Text:       "Add feature",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(pub.events[0].TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 9 {
		t.Fatalf("energy budget = %d, want colony default 9", snap.EnergyBudget)
	}
}

func TestRunTaskEnergyBudgetIgnoredOnSeededTrace(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, 12)
	writeCueFile(t, root, "hotfix.yaml", `emit: task
bee: builder
intent: bugfix
autorun: false
energy_budget: 3
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-seeded", 10); err != nil {
		t.Fatal(err)
	}

	_, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Fix crash",
		TraceID:    "trace-seeded",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot("trace-seeded")
	if err != nil {
		t.Fatal(err)
	}
	if snap.EnergyBudget != 10 {
		t.Fatalf("seeded trace budget changed = %d", snap.EnergyBudget)
	}
}
