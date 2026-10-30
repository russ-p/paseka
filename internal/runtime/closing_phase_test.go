package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

const closingLifecycleColony = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 2
  task:
    bee: reviewer
    intent: closing
    title: Review the trail
    review: required
  max_rounds: 3
`

func newClosingTestReactor(t *testing.T, colonyYAML string, bees map[string]colony.Bee) (*runtime.Reactor, *recordingPublisher, *recordingAdapter) {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, bees)
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(colonyYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	adapter := &recordingAdapter{}
	d.RegisterAdapter("cursor", adapter)
	pub := &recordingPublisher{}
	d.SetPublisher(pub, false)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
		Publisher:  pub,
	}), pub, adapter
}

func TestReactorClosingLifecycleEndToEnd(t *testing.T) {
	r, pub, _ := newClosingTestReactor(t, closingLifecycleColony, map[string]colony.Bee{
		"builder": {Role: "builder"},
		"reviewer": {Role: "reviewer", Publishes: []colony.PublicationRule{{
			EventRule: colony.EventRule{Type: string(protocol.EventVerification), Kind: string(protocol.TaskEventCompleted)},
		}}},
	})
	traceID := "trace-closing-e2e"
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
	snapshot := func() taskledger.TraceSnapshot {
		t.Helper()
		snap, err := r.Ledger().Snapshot(traceID)
		if err != nil {
			t.Fatal(err)
		}
		return snap
	}
	assertGateClosed := func(stage string) {
		t.Helper()
		snap := snapshot()
		if gate, ok := snap.Tasks[taskledger.FinalReviewTaskID]; ok {
			switch gate.Status {
			case protocol.TaskStatusWaitingReview, protocol.TaskStatusCompleted:
				t.Fatalf("%s: merge gate opened early (%s)", stage, gate.Status)
			}
		}
		for _, task := range snap.Tasks {
			if taskledger.IsMergeTask(task) && (task.Status == protocol.TaskStatusWaitingReview || task.Status == protocol.TaskStatusCompleted) {
				t.Fatalf("%s: merge task %q opened early (%s)", stage, task.TaskID, task.Status)
			}
		}
	}

	plan("scout", protocol.TaskSpec{TaskID: "task-1", Title: "First slice", Bee: "builder"})
	plan("scout", protocol.TaskSpec{TaskID: "task-2", Title: "Second slice", Bee: "builder"})
	proposal("task-1")
	complete("builder-agent", "task-1")
	complete("builder-agent", "task-2")

	// The last work task completes: the runtime seeds the first closing round
	// and dispatches it with the ordinary AFK path. Its review: required policy
	// leaves it waiting while this test drives the rest of the trail.
	snap := snapshot()
	closing1, ok := snap.Tasks["closing-1"]
	if !ok {
		t.Fatalf("closing-1 was not seeded: %+v", snap.Tasks)
	}
	if taskledger.TaskPhaseOf(closing1) != protocol.TaskPhaseClosing {
		t.Fatalf("closing-1 phase = %q", closing1.Phase)
	}
	if closing1.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("closing-1 status = %q, want waiting_review after dispatch", closing1.Status)
	}
	assertGateClosed("after the first closing round starts")
	assertClosingPlanPublished(t, pub, "closing-1")

	plan("reviewer-agent-1", protocol.TaskSpec{
		TaskID:    "fix-1",
		Title:     "Fix what closing found",
		Bee:       "builder",
		Review:    protocol.TaskReviewRequired,
		DependsOn: []string{"closing-1"},
	})
	complete("reviewer-agent-1", "closing-1")

	snap = snapshot()
	if _, inFlight := snap.Tasks["closing-2"]; inFlight {
		t.Fatal("seeded a second round before the follow-up work was finished")
	}
	assertGateClosed("after the first closing round completes with follow-up work open")

	complete("builder-agent", "fix-1")

	snap = snapshot()
	closing2, ok := snap.Tasks["closing-2"]
	if !ok {
		t.Fatalf("closing-2 was not seeded after follow-up work: %+v", snap.Tasks)
	}
	if closing2.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("closing-2 status = %q, want waiting_review after dispatch", closing2.Status)
	}
	assertGateClosed("after the second closing round starts")

	complete("reviewer-agent-2", "closing-2")

	snap = snapshot()
	gate := snap.Tasks[taskledger.FinalReviewTaskID]
	if gate.Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("gate status = %q, want waiting_review after the last closing round", gate.Status)
	}
	if taskledger.TaskPhaseOf(gate) != protocol.TaskPhaseMerge {
		t.Fatalf("gate phase = %q, want merge", gate.Phase)
	}
	var planRounds []string
	for _, task := range snap.Tasks {
		if taskledger.IsClosingTask(task) {
			planRounds = append(planRounds, task.TaskID)
		}
	}
	if len(planRounds) != 2 {
		t.Fatalf("closing rounds = %v, want closing-1 and closing-2", planRounds)
	}
}

func TestReactorDoesNotSeedClosingWhenPredicateFails(t *testing.T) {
	colonyYAML := `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 3
  task:
    bee: reviewer
    review: required
  max_rounds: 2
`
	r, _, _ := newClosingTestReactor(t, colonyYAML, map[string]colony.Bee{
		"builder":  {Role: "builder"},
		"reviewer": {Role: "reviewer"},
	})
	traceID := "trace-closing-predicate"
	plan := func(spec protocol.TaskSpec) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
			Kind:  protocol.TaskEventPlan,
			Tasks: []protocol.TaskSpec{spec},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	plan(protocol.TaskSpec{TaskID: "task-1", Title: "First", Bee: "builder"})
	plan(protocol.TaskSpec{TaskID: "task-2", Title: "Second", Bee: "builder"})
	proposal, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), proposal); err != nil {
		t.Fatal(err)
	}
	for _, taskID := range []string{"task-1", "task-2"} {
		ev, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
			Kind:   protocol.TaskEventCompleted,
			TaskID: taskID,
			Status: protocol.TaskStatusCompleted,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}

	snap, err := r.Ledger().Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range snap.Tasks {
		if taskledger.IsClosingTask(task) {
			t.Fatalf("closing round %q seeded below min_work_tasks", task.TaskID)
		}
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("merge gate status = %q, want waiting_review after the predicate skipped closing", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}
}

func TestClosingPromptReceivesTrailInventory(t *testing.T) {
	const closingYAML = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 1
  task:
    bee: reviewer
  max_rounds: 1
`
	r, _, adapter := newClosingTestReactor(t, closingYAML, map[string]colony.Bee{
		"builder":  {Role: "builder"},
		"reviewer": {Role: "reviewer"},
	})
	inventoryPrompt := "{{range .TrailInventory.Tasks}}{{.TaskID}}|{{.Phase}}|{{.Status}}|{{.Commit}}|{{.Title}}\n{{end}}"
	promptsDir := filepath.Join(r.ColonyRoot(), ".paseka", "prompts")
	if err := os.WriteFile(filepath.Join(promptsDir, "default.md"), []byte(inventoryPrompt), 0o644); err != nil {
		t.Fatal(err)
	}

	traceID := "trace-closing-inventory"
	plan := func(spec protocol.TaskSpec) {
		t.Helper()
		ev, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
			Kind:  protocol.TaskEventPlan,
			Tasks: []protocol.TaskSpec{spec},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatal(err)
		}
	}
	plan(protocol.TaskSpec{TaskID: "task-1", Title: "First slice", Bee: "builder"})
	completed, err := protocol.NewEvent(traceID, "builder-agent", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-1",
		Status: protocol.TaskStatusCompleted,
		Commit: "abc123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), completed); err != nil {
		t.Fatal(err)
	}

	var closingPrompt string
	for _, prompt := range adapter.prompts {
		if strings.Contains(prompt, "closing-1|closing|") {
			closingPrompt = prompt
			break
		}
	}
	if closingPrompt == "" {
		t.Fatalf("no closing prompt with an inventory: %q", adapter.prompts)
	}
	for _, want := range []string{
		"task-1|work|completed|abc123|First slice",
		"closing-1|closing|running||Review the trail and prepare the pull request",
	} {
		if !strings.Contains(closingPrompt, want) {
			t.Fatalf("closing prompt missing %q: %q", want, closingPrompt)
		}
	}
}

func TestLastWorkTaskPRBodySuppressionFollowsPinnedPolicy(t *testing.T) {
	const (
		emitPrompt = `{{if .IsLastWorkTask}}{{if not .SuppressPRBody}}PR_BODY{{end}}TRACE_SUMMARY{{end}}`
		noClosing  = `defaults:
  prompt_template: default.md
`
		closingEnabled = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 1
  task:
    bee: reviewer
  max_rounds: 1
`
		predicateMisses = `defaults:
  prompt_template: default.md
closing:
  enabled: true
  when:
    min_work_tasks: 3
  task:
    bee: reviewer
  max_rounds: 1
`
	)
	for _, tc := range []struct {
		name       string
		colonyYAML string
		wantPRBody bool
	}{
		{name: "guaranteed closing round", colonyYAML: closingEnabled, wantPRBody: false},
		{name: "no closing policy", colonyYAML: noClosing, wantPRBody: true},
		{name: "predicate misses", colonyYAML: predicateMisses, wantPRBody: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _, adapter := newClosingTestReactor(t, tc.colonyYAML, map[string]colony.Bee{
				"builder":  {Role: "builder"},
				"reviewer": {Role: "reviewer"},
			})
			promptsDir := filepath.Join(r.ColonyRoot(), ".paseka", "prompts")
			if err := os.WriteFile(filepath.Join(promptsDir, "default.md"), []byte(emitPrompt), 0o644); err != nil {
				t.Fatal(err)
			}
			traceID := "trace-last-work-pr-body"
			plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
				Kind:  protocol.TaskEventPlan,
				Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "Only slice", Bee: "builder"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.ProcessEvent(context.Background(), plan); err != nil {
				t.Fatal(err)
			}
			ready, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventSignal, protocol.TaskReadyPayload{
				Kind:   protocol.TaskEventReady,
				TaskID: "task-1",
				Title:  "Only slice",
				Bee:    "builder",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := r.ProcessEvent(context.Background(), ready); err != nil {
				t.Fatal(err)
			}
			if len(adapter.prompts) == 0 {
				t.Fatal("the last work task was not dispatched")
			}
			lastWorkPrompt := adapter.prompts[0]
			if !strings.Contains(lastWorkPrompt, "TRACE_SUMMARY") {
				t.Fatalf("last work task lost trace.summary guidance: %q", lastWorkPrompt)
			}
			if got := strings.Contains(lastWorkPrompt, "PR_BODY"); got != tc.wantPRBody {
				t.Fatalf("pr.body guidance present = %v, want %v: %q", got, tc.wantPRBody, lastWorkPrompt)
			}
		})
	}
}

func assertClosingPlanPublished(t *testing.T, pub *recordingPublisher, taskID string) {
	t.Helper()
	for _, ev := range pub.events {
		if ev.Type != protocol.EventInsight || protocol.PayloadKind(ev.Payload) != string(protocol.TaskEventPlan) || ev.AgentID != protocol.RuntimeAgentID {
			continue
		}
		var payload protocol.TaskPlanPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		for _, spec := range payload.Tasks {
			if spec.TaskID != taskID {
				continue
			}
			if spec.Phase != protocol.TaskPhaseClosing {
				t.Fatalf("%s plan phase = %q, want closing", taskID, spec.Phase)
			}
			return
		}
	}
	t.Fatalf("no runtime task.plan published for %s", taskID)
}
