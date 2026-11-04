package cues_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
)

func decodeCueIngress(t *testing.T, ev protocol.Event) protocol.CueIngressPayload {
	t.Helper()
	var payload protocol.CueIngressPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatalf("parse cue.ingress: %v", err)
	}
	return payload
}

func eventIndex(events []protocol.Event, id string) int {
	for i, ev := range events {
		if ev.ID == id {
			return i
		}
	}
	return -1
}

// A task-emitting cue records one invocation and publishes its observation
// before the task.plan it announces, on a brand new trace.
func TestRunTaskRecordsCueInvocationIngress(t *testing.T) {
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
	if res.InvocationID == "" {
		t.Fatal("invocation id is empty")
	}
	if n := countKind(pub.events, protocol.SignalCueIngress); n != 1 {
		t.Fatalf("cue.ingress count = %d, want one: %v", n, kindsOf(pub.events))
	}
	ingressEv, ok := eventByKind(pub.events, protocol.SignalCueIngress)
	if !ok {
		t.Fatalf("missing cue.ingress: %v", kindsOf(pub.events))
	}
	planEv, ok := eventByKind(pub.events, string(protocol.TaskEventPlan))
	if !ok {
		t.Fatalf("missing task.plan: %v", kindsOf(pub.events))
	}
	readyEv, ok := eventByKind(pub.events, string(protocol.TaskEventReady))
	if !ok {
		t.Fatalf("missing task.ready: %v", kindsOf(pub.events))
	}
	if eventIndex(pub.events, ingressEv.ID) > eventIndex(pub.events, planEv.ID) {
		t.Fatalf("cue.ingress published after task.plan: %v", kindsOf(pub.events))
	}
	if ingressEv.TraceID != res.TraceID || !strings.HasPrefix(res.TraceID, "trace-") {
		t.Fatalf("trace = %q", res.TraceID)
	}

	payload := decodeCueIngress(t, ingressEv)
	if payload.InvocationID != res.InvocationID {
		t.Fatalf("invocation = %q, result = %q", payload.InvocationID, res.InvocationID)
	}
	if payload.CueID != "hotfix" || payload.Source != "cli" || payload.Mode != protocol.CueIngressModeTask {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.InvokedAt.IsZero() {
		t.Fatal("invocation time is missing")
	}
	if !reflect.DeepEqual(payload.EventIDs, []string{planEv.ID, readyEv.ID}) {
		t.Fatalf("event ids = %v, want %v", payload.EventIDs, []string{planEv.ID, readyEv.ID})
	}
	if !reflect.DeepEqual(payload.TaskIDs, []string{res.TaskID}) {
		t.Fatalf("task ids = %v, want %v", payload.TaskIDs, []string{res.TaskID})
	}

	record, ok, err := runs.ReadCueInvocation(root, res.TraceID, res.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("durable invocation record missing")
	}
	if record.Mode != string(cues.EmitTask) || record.CueID != "hotfix" || record.Source != "cli" {
		t.Fatalf("record = %+v", record)
	}
	if record.Ingress.ID != ingressEv.ID {
		t.Fatalf("record ingress id = %q, want %q", record.Ingress.ID, ingressEv.ID)
	}
	if !record.Published() {
		t.Fatalf("record not fully published: %+v", record)
	}

	var plan protocol.TaskPlanPayload
	if err := json.Unmarshal(planEv.Payload, &plan); err != nil {
		t.Fatal(err)
	}
	var ready protocol.TaskReadyPayload
	if err := json.Unmarshal(readyEv.Payload, &ready); err != nil {
		t.Fatal(err)
	}
	if plan.Tasks[0].CueInvocationID != res.InvocationID {
		t.Fatalf("task spec invocation = %q", plan.Tasks[0].CueInvocationID)
	}
	if ready.CueInvocationID != res.InvocationID {
		t.Fatalf("task.ready invocation = %q", ready.CueInvocationID)
	}

	if _, err := ledger.Apply(planEv); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(readyEv); err != nil {
		t.Fatal(err)
	}
	snap, err := ledger.Snapshot(res.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if got := snap.Tasks[res.TaskID].CueInvocationID; got != res.InvocationID {
		t.Fatalf("ledger invocation = %q, want %q", got, res.InvocationID)
	}

	if err := runs.WriteTaskSnapshot(root, res.TraceID, snap.Tasks[res.TaskID]); err != nil {
		t.Fatal(err)
	}
	taskDir, err := runs.NewTaskDir(root, res.TraceID, res.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	fm, _, err := taskDir.ReadTask()
	if err != nil {
		t.Fatal(err)
	}
	if fm.CueInvocationID != res.InvocationID {
		t.Fatalf("task.md invocation = %q, want %q", fm.CueInvocationID, res.InvocationID)
	}
}

// A signal-emitting cue records the invocation even though it creates no task.
func TestRunSignalRecordsCueInvocationIngressOnExistingTrace(t *testing.T) {
	root := t.TempDir()
	writeCueFile(t, root, "feature.yaml", `emit: signal
type: SIGNAL
kind: feature.requested
title: "{{.Title}}"
body: "{{.Body}}"
`)

	pub := &recordingPublisher{}
	res, err := cues.Run(context.Background(), pub, taskledger.NewMemoryLedger(), cues.RunInput{
		ColonyRoot: root,
		CueID:      "feature",
		Text:       "OAuth callback\nAdd OAuth support",
		TraceID:    "trace-existing",
		Source:     "console",
		AgentID:    "console",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.TraceID != "trace-existing" || res.InvocationID == "" {
		t.Fatalf("result = %+v", res)
	}
	if n := countKind(pub.events, protocol.SignalCueIngress); n != 1 {
		t.Fatalf("cue.ingress count = %d: %v", n, kindsOf(pub.events))
	}
	ingressEv, _ := eventByKind(pub.events, protocol.SignalCueIngress)
	signalEv, ok := eventByKind(pub.events, "feature.requested")
	if !ok {
		t.Fatalf("missing signal: %v", kindsOf(pub.events))
	}
	payload := decodeCueIngress(t, ingressEv)
	if payload.Mode != protocol.CueIngressModeSignal {
		t.Fatalf("mode = %q", payload.Mode)
	}
	if payload.Source != "console" {
		t.Fatalf("source = %q", payload.Source)
	}
	if len(payload.TaskIDs) != 0 {
		t.Fatalf("task ids = %v, want none", payload.TaskIDs)
	}
	if !reflect.DeepEqual(payload.EventIDs, []string{signalEv.ID}) {
		t.Fatalf("event ids = %v, want %v", payload.EventIDs, []string{signalEv.ID})
	}

	record, ok, err := runs.ReadCueInvocation(root, res.TraceID, res.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("durable invocation record missing")
	}
	if record.Mode != string(cues.EmitSignal) || len(record.TaskIDs) != 0 {
		t.Fatalf("record = %+v", record)
	}
}

// Standing ticks are invocations too: each tick gets its own identity on the
// shared trail, and both remain retained.
func TestRunStandingTicksRecordDistinctCueInvocations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	writeCueFile(t, root, "daily-triage.yaml", standingSignalYAML)

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()
	first, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "daily-triage",
		Text:       "tick 1",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "daily-triage",
		Text:       "tick 2",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.InvocationID == "" || second.InvocationID == "" {
		t.Fatalf("invocations = %q, %q", first.InvocationID, second.InvocationID)
	}
	if first.InvocationID == second.InvocationID {
		t.Fatalf("two ticks received one invocation identity %q", first.InvocationID)
	}
	if n := countKind(pub.events, protocol.SignalCueIngress); n != 2 {
		t.Fatalf("cue.ingress count = %d, want two: %v", n, kindsOf(pub.events))
	}

	invocations, err := runs.ListCueInvocations(root, first.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 2 {
		t.Fatalf("retained invocations = %d, want two", len(invocations))
	}
	seen := map[string]bool{}
	for _, inv := range invocations {
		seen[inv.InvocationID] = true
		if inv.Mode != string(cues.EmitSignal) || len(inv.TaskIDs) != 0 {
			t.Fatalf("record = %+v", inv)
		}
		var payload protocol.CueIngressPayload
		if err := json.Unmarshal(inv.Ingress.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.EventIDs) != 1 {
			t.Fatalf("event ids = %v, want one", payload.EventIDs)
		}
	}
	if !seen[first.InvocationID] || !seen[second.InvocationID] {
		t.Fatalf("retained = %v", seen)
	}
}

type refusingPublisher struct {
	events   []protocol.Event
	refuseAt int
	calls    int
}

func (p *refusingPublisher) PublishEvent(_ context.Context, ev protocol.Event) error {
	p.calls++
	if p.calls == p.refuseAt {
		return errors.New("broker refused")
	}
	p.events = append(p.events, ev)
	return nil
}

// A retried publication replays the recorded invocation and event identities;
// it never mints a second invocation.
func TestRepublishInvocationKeepsInvocationAndEventIdentities(t *testing.T) {
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

	first := &refusingPublisher{refuseAt: 3} // seed and cue.ingress succeed, task.plan is refused
	_, err := cues.Run(context.Background(), first, taskledger.NewMemoryLedger(), cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Login crash\nRepro steps",
		TraceID:    "trace-retry",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err == nil || !strings.Contains(err.Error(), "broker refused") {
		t.Fatalf("err = %v", err)
	}
	if n := countKind(first.events, protocol.SignalCueIngress); n != 1 {
		t.Fatalf("cue.ingress count = %d, want one: %v", n, kindsOf(first.events))
	}

	invocations, err := runs.ListCueInvocations(root, "trace-retry")
	if err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 1 {
		t.Fatalf("retained invocations = %d, want one", len(invocations))
	}
	record := invocations[0]
	recordedIDs := record.EventIDs()
	if len(recordedIDs) != 2 {
		t.Fatalf("recorded event ids = %v, want plan and ready", recordedIDs)
	}
	if !record.IngressPublished {
		t.Fatal("cue.ingress publication was not recorded")
	}
	if len(record.Events) != 2 || record.Events[0].Published {
		t.Fatalf("record events = %+v, want the plan still pending", record.Events)
	}

	retry := &recordingPublisher{}
	// A retry must replay the recorded invocation, not today's cue definition.
	writeCueFile(t, root, "hotfix.yaml", `emit: task
bee: builder
intent: bugfix
review: none
autorun: true
title: "Changed title"
body: "Changed body"
`)
	res, err := cues.RepublishInvocation(context.Background(), retry, root, "trace-retry", record.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if res.InvocationID != record.InvocationID || res.TraceID != "trace-retry" || res.TaskID == "" {
		t.Fatalf("result = %+v", res)
	}
	if n := countKind(retry.events, protocol.SignalCueIngress); n != 0 {
		t.Fatalf("retry republished cue.ingress %d times", n)
	}
	retriedIDs := make([]string, 0, len(retry.events))
	for _, ev := range retry.events {
		retriedIDs = append(retriedIDs, ev.ID)
	}
	if !reflect.DeepEqual(retriedIDs, recordedIDs) {
		t.Fatalf("retried ids = %v, want %v", retriedIDs, recordedIDs)
	}
	var retriedPlan protocol.TaskPlanPayload
	if err := json.Unmarshal(retry.events[0].Payload, &retriedPlan); err != nil {
		t.Fatal(err)
	}
	if retriedPlan.Tasks[0].Title != "Login crash" {
		t.Fatalf("retry used a later cue definition: title = %q", retriedPlan.Tasks[0].Title)
	}

	reloaded, ok, err := runs.ReadCueInvocation(root, "trace-retry", record.InvocationID)
	if err != nil || !ok {
		t.Fatalf("reload: ok=%v err=%v", ok, err)
	}
	if !reloaded.Published() {
		t.Fatalf("retried invocation is still pending: %+v", reloaded)
	}

	again := &recordingPublisher{}
	if _, err := cues.RepublishInvocation(context.Background(), again, root, "trace-retry", record.InvocationID); err != nil {
		t.Fatal(err)
	}
	if len(again.events) != 0 {
		t.Fatalf("second retry republished %v", kindsOf(again.events))
	}

	fresh := &recordingPublisher{}
	freshRes, err := cues.Run(context.Background(), fresh, taskledger.NewMemoryLedger(), cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Another task\nSecond run",
		TraceID:    "trace-retry",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}
	if freshRes.InvocationID == record.InvocationID {
		t.Fatal("a new cue run reused the previous invocation identity")
	}
}

// Task projection records written before invocation provenance existed still
// decode, and the field stays empty instead of being invented.
func TestLegacyTaskMarkdownWithoutCueInvocationDecodes(t *testing.T) {
	root := t.TempDir()
	dir := runs.TaskDir{ColonyRoot: root, TraceID: "trace-legacy", TaskID: "task-legacy"}
	content := "---\ntraceId: trace-legacy\ntaskId: task-legacy\ntitle: Legacy\ncueId: old-cue\nstatus: planned\n---\n\nbody\n"
	if err := os.MkdirAll(dir.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir.Root(), runs.TaskFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	fm, _, err := dir.ReadTask()
	if err != nil {
		t.Fatal(err)
	}
	if fm.CueID != "old-cue" {
		t.Fatalf("cue id = %q", fm.CueID)
	}
	if fm.CueInvocationID != "" {
		t.Fatalf("invented invocation = %q", fm.CueInvocationID)
	}

	// A manually-created task snapshot writes no invocation field either.
	if err := runs.WriteTaskSnapshot(root, "trace-legacy", taskledger.TaskSnapshot{
		TaskID: "task-manual",
		Title:  "Manual",
		Status: protocol.TaskStatusPlanned,
	}); err != nil {
		t.Fatal(err)
	}
	manual, err := runs.NewTaskDir(root, "trace-legacy", "task-manual")
	if err != nil {
		t.Fatal(err)
	}
	manualFM, _, err := manual.ReadTask()
	if err != nil {
		t.Fatal(err)
	}
	if manualFM.CueID != "" || manualFM.CueInvocationID != "" {
		t.Fatalf("manual task provenance = cue %q invocation %q", manualFM.CueID, manualFM.CueInvocationID)
	}
}
