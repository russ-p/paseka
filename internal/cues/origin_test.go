package cues_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/cues"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
)

func writeTaskCue(t *testing.T, root, id string) {
	t.Helper()
	writeCueFile(t, root, id+".yaml", `emit: task
bee: builder
intent: feature
review: none
title: "{{.Title}}"
body: "{{.Body}}"
`)
}

func TestCueOriginRetainsTheCreatingInvocation(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, 8)
	writeTaskCue(t, root, "feature")
	writeTaskCue(t, root, "hotfix")

	pub := &recordingPublisher{}
	ledger := taskledger.NewMemoryLedger()
	first, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "feature",
		Text:       "Build the first task\nThe initial trail goal",
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}

	origin, ok, err := runs.ReadTrailOrigin(root, first.TraceID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("cue-created trail has no origin")
	}
	if origin.Kind != runs.OriginKindCue || origin.CueID != "feature" || origin.InvocationID != first.InvocationID {
		t.Fatalf("cue origin = %+v, invocation = %q", origin, first.InvocationID)
	}

	second, err := cues.Run(context.Background(), pub, ledger, cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Different cue\nA later task on the same trail",
		TraceID:    first.TraceID,
		Source:     "console",
		AgentID:    "console",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.TaskID == first.TaskID {
		t.Fatal("later cue reused the first task id")
	}

	after, ok, err := runs.ReadTrailOrigin(root, first.TraceID)
	if err != nil || !ok {
		t.Fatalf("origin after later cue: ok=%v err=%v", ok, err)
	}
	if after.Kind != runs.OriginKindCue || after.CueID != "feature" || after.InvocationID != first.InvocationID {
		t.Fatalf("later cue replaced the origin: %+v", after)
	}

	var later protocol.TaskSpec
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventPlan) {
			continue
		}
		var plan protocol.TaskPlanPayload
		if err := json.Unmarshal(ev.Payload, &plan); err != nil {
			t.Fatal(err)
		}
		for _, spec := range plan.Tasks {
			if spec.TaskID == second.TaskID {
				later = spec
			}
		}
	}
	if later.TaskID == "" {
		t.Fatal("later task plan was not published")
	}
	if later.CueID != "hotfix" {
		t.Fatalf("later task cue = %q, want hotfix", later.CueID)
	}
	if later.CueInvocationID != second.InvocationID {
		t.Fatalf("later task invocation = %q, want %q", later.CueInvocationID, second.InvocationID)
	}
}

func TestLaterCueOnManualTrailLeavesOriginUnchanged(t *testing.T) {
	root := t.TempDir()
	writeColonyManifest(t, root, 8)
	writeTaskCue(t, root, "hotfix")

	const traceID = "trace-manual"
	if _, _, err := runs.EnsureTrailOrigin(root, traceID, runs.TrailOrigin{
		Kind:   runs.OriginKindManual,
		Source: "task.create",
	}); err != nil {
		t.Fatal(err)
	}

	_, err := cues.Run(context.Background(), &recordingPublisher{}, taskledger.NewMemoryLedger(), cues.RunInput{
		ColonyRoot: root,
		CueID:      "hotfix",
		Text:       "Add work\nThe trail already exists",
		TraceID:    traceID,
		Source:     "cli",
		AgentID:    "cli",
	})
	if err != nil {
		t.Fatal(err)
	}

	origin, ok, err := runs.ReadTrailOrigin(root, traceID)
	if err != nil || !ok {
		t.Fatalf("manual origin after cue: ok=%v err=%v", ok, err)
	}
	if origin.Kind != runs.OriginKindManual || origin.CueID != "" || origin.InvocationID != "" {
		t.Fatalf("later cue changed a manual origin: %+v", origin)
	}
}
