package runtime_test

import (
	"context"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/runtime"
)

func TestDispatchRecordsAdmissionPurpose(t *testing.T) {
	root := t.TempDir()
	writeColony(t, root)

	rec := &recordingAdapter{}
	d := runtime.NewDispatcher()
	d.RegisterAdapter("cursor", rec)

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-purpose-default",
		Task:       "ordinary work",
	}); err != nil {
		t.Fatal(err)
	}
	meta, ok, err := runs.FindRun(root, "trace-purpose-default", rec.lastReq.AgentID)
	if err != nil || !ok {
		t.Fatalf("find default run: ok=%v err=%v", ok, err)
	}
	if meta.Purpose != protocol.PurposeWork || meta.AnalysisOf != "" {
		t.Fatalf("default run purpose = %q analysisOf = %q, want work", meta.Purpose, meta.AnalysisOf)
	}

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-purpose-analysis",
		Task:       "study the result",
		Purpose:    protocol.PurposeAnalysis,
		AnalysisOf: "completion-r1",
	}); err != nil {
		t.Fatal(err)
	}
	meta, ok, err = runs.FindRun(root, "trace-purpose-analysis", rec.lastReq.AgentID)
	if err != nil || !ok {
		t.Fatalf("find analysis run: ok=%v err=%v", ok, err)
	}
	if meta.Purpose != protocol.PurposeAnalysis || meta.AnalysisOf != "completion-r1" {
		t.Fatalf("analysis run purpose = %q analysisOf = %q", meta.Purpose, meta.AnalysisOf)
	}

	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-purpose-missing-milestone",
		Task:       "study without saying what",
		Purpose:    protocol.PurposeAnalysis,
	}); err == nil {
		t.Fatal("analysis without analysisOf was accepted")
	}
	if _, err := d.Dispatch(context.Background(), runtime.DispatchRequest{
		ColonyRoot: root,
		Bee:        "builder",
		TraceID:    "trace-purpose-misattributed",
		Task:       "ordinary work labelled as studying",
		Purpose:    protocol.PurposeWork,
		AnalysisOf: "completion-r1",
	}); err == nil {
		t.Fatal("analysisOf without analysis purpose was accepted")
	}
}

func TestTaskDispatchPurposeFollowsTaskPhase(t *testing.T) {
	r, _, _ := newClosingTestReactor(t, closingLifecycleColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
		"reviewer": {Role: "reviewer", Publishes: []colony.PublicationRule{{
			EventRule: colony.EventRule{Type: string(protocol.EventVerification), Kind: string(protocol.TaskEventCompleted)},
		}}},
	})
	traceID := "trace-purpose-phase"
	base := time.Now().UTC()
	var step int
	at := func() time.Time {
		step++
		return base.Add(time.Duration(step) * time.Minute)
	}
	plan := func(agentID string, specs ...protocol.TaskSpec) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
			Kind:  protocol.TaskEventPlan,
			Tasks: specs,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	proposal := func(taskID string) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventMutation, protocol.MutationPayload{
			Kind:      protocol.MutationCodeProposalIsolated,
			TaskID:    taskID,
			Workspace: protocol.ProposalWorkspaceIsolated,
			Diff:      "+line",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	complete := func(agentID, taskID string) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
			Kind:        protocol.TaskEventCompleted,
			TaskID:      taskID,
			Status:      protocol.TaskStatusCompleted,
			CompletedAt: at(),
		})
		if err != nil {
			t.Fatal(err)
		}
		ev.CreatedAt = at()
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	plan("scout", protocol.TaskSpec{TaskID: "task-1", Title: "First slice", Bee: "builder"})
	plan("scout", protocol.TaskSpec{TaskID: "task-2", Title: "Second slice", Bee: "builder"})
	proposal("task-1")
	complete("builder-agent", "task-1")
	complete("builder-agent", "task-2")

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Tasks["closing-1"]; !ok {
		t.Fatalf("closing-1 was not seeded: %+v", snap.Tasks)
	}

	metas, err := runs.ListRunsForTrace(r.ColonyRoot(), traceID)
	if err != nil {
		t.Fatal(err)
	}
	purposes := map[string][]protocol.ExecutionPurpose{}
	for _, meta := range metas {
		purposes[meta.TaskID] = append(purposes[meta.TaskID], meta.Purpose)
	}
	workDispatches := 0
	for taskID, found := range purposes {
		if taskID == "closing-1" || taskID == "" {
			continue
		}
		for _, purpose := range found {
			workDispatches++
			if purpose != protocol.PurposeWork {
				t.Fatalf("task %s dispatch purpose = %q, want work", taskID, purpose)
			}
		}
	}
	if workDispatches == 0 {
		t.Fatalf("no work task recorded a dispatch: %+v", purposes)
	}
	closing := purposes["closing-1"]
	if len(closing) == 0 {
		t.Fatalf("closing-1 has no recorded dispatch: %+v", purposes)
	}
	for _, purpose := range closing {
		if purpose != protocol.PurposeClosing {
			t.Fatalf("closing-1 dispatch purpose = %q, want closing", purpose)
		}
	}
}
