package review_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestValidateCommentsPacketRequiresContent(t *testing.T) {
	if err := review.ValidateCommentsPacket(review.CommentsPacket{}); err == nil {
		t.Fatal("expected error for empty packet")
	}
	if err := review.ValidateCommentsPacket(review.CommentsPacket{Summary: "fix tests"}); err != nil {
		t.Fatal(err)
	}
}

func TestValidateReviewCommentSideAndLines(t *testing.T) {
	err := review.ValidateCommentsPacket(review.CommentsPacket{
		Comments: []review.ReviewComment{{
			Path: "a.go", Side: "bad", StartLine: 1, Body: "note",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "side") {
		t.Fatalf("err = %v", err)
	}
}

func TestRenderCommentsMarkdown(t *testing.T) {
	md := string(review.RenderCommentsMarkdown(review.CommentsPacket{
		HeadSHA: "abc123",
		Summary: "Please tighten error handling",
		Comments: []review.ReviewComment{{
			Path: "internal/foo.go", Side: "new", StartLine: 10, EndLine: 12,
			Snippet: "if err != nil", Body: "Handle this error",
		}},
	}))
	for _, want := range []string{
		"# Review comments",
		"headSha: `abc123`",
		"Please tighten error handling",
		"## `internal/foo.go`",
		"**new** L10–12",
		"Handle this error",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestSubmitAnnotatedReviewWritesCombAndPublishes(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-review-comments"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	res, err := review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID:  traceID,
		TaskID:   "task-1",
		AgentID:  "console",
		Producer: artifacts.ProducerConsole,
		Packet: review.CommentsPacket{
			HeadSHA: "deadbeef",
			Summary: "Needs work",
			Comments: []review.ReviewComment{{
				Path: "main.go", Side: "new", StartLine: 3, Body: "rename symbol",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := res.CombRef
	if res.ReworkTaskID != "" {
		t.Fatalf("rework task = %q, want none for review: required", res.ReworkTaskID)
	}
	if !strings.Contains(ref, review.ReviewCommentsCombRel) {
		t.Fatalf("ref = %q", ref)
	}
	combPath := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", review.ReviewCommentsCombRel)
	data, err := os.ReadFile(combPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "rename symbol") {
		t.Fatalf("comb body = %s", data)
	}
	if len(pub.events) != 2 {
		t.Fatalf("events = %d", len(pub.events))
	}
	artifactEv := pub.events[0]
	if artifactEv.Type != protocol.EventSignal || protocol.PayloadKind(artifactEv.Payload) != string(protocol.SignalArtifactWritten) {
		t.Fatalf("artifact event = %+v", artifactEv)
	}
	if artifactEv.AgentID != artifacts.ProducerConsole {
		t.Fatalf("producer = %q", artifactEv.AgentID)
	}
	var written protocol.ArtifactWrittenPayload
	if err := json.Unmarshal(artifactEv.Payload, &written); err != nil {
		t.Fatal(err)
	}
	if len(written.Artifacts) != 1 || written.Artifacts[0].ArtifactKind != "review-comments" {
		t.Fatalf("artifacts = %+v", written.Artifacts)
	}
	feedbackEv := pub.events[1]
	if protocol.PayloadKind(feedbackEv.Payload) != string(protocol.InsightHumanFeedback) {
		t.Fatalf("feedback event = %+v", feedbackEv)
	}
	var feedback protocol.HumanFeedbackPayload
	if err := json.Unmarshal(feedbackEv.Payload, &feedback); err != nil {
		t.Fatal(err)
	}
	if feedback.Message != "Needs work" || feedback.Ref != ref {
		t.Fatalf("feedback = %+v", feedback)
	}
}

func TestSubmitAnnotatedReviewFailClosedWhenTaskNotWaiting(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-planned"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}

	_, err = review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID: traceID,
		TaskID:  "task-1",
		Packet: review.CommentsPacket{
			Comments: []review.ReviewComment{{
				Path: "main.go", Side: "new", StartLine: 1, Body: "note",
			}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "waiting_review") {
		t.Fatalf("err = %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("expected no bus events")
	}
	combPath := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", review.ReviewCommentsCombRel)
	if _, err := os.Stat(combPath); err == nil {
		t.Fatal("expected no comb file when task is not waiting_review")
	}
}

func TestSubmitAnnotatedReviewFailClosedOnInvalidComment(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-bad"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}

	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-1", Title: "Work", Review: protocol.TaskReviewRequired}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(plan); err != nil {
		t.Fatal(err)
	}
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-1", Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(waiting); err != nil {
		t.Fatal(err)
	}

	_, err = review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID: traceID,
		TaskID:  "task-1",
		Packet: review.CommentsPacket{
			Comments: []review.ReviewComment{{
				Path: "", Side: "new", StartLine: 1, Body: "bad",
			}},
		},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(pub.events) != 0 {
		t.Fatal("expected no bus events on failure")
	}
}

func TestShortFeedbackMessageDefault(t *testing.T) {
	if got := review.ShortFeedbackMessage(""); got != "Review comments written to comb" {
		t.Fatalf("got %q", got)
	}
	if got := review.ShortFeedbackMessage("  summary "); got != "summary" {
		t.Fatalf("got %q", got)
	}
}

func TestRejectResponseMessageFinalAnnotated(t *testing.T) {
	msg := review.RejectResponseMessage(true, true, "task-abc")
	if !strings.Contains(msg, "merge gate remains open") || !strings.Contains(msg, "task-abc") {
		t.Fatalf("msg = %q", msg)
	}
}

func mustApplyReviewLedger(t *testing.T, ledger taskledger.Ledger, ev protocol.Event) {
	t.Helper()
	if _, err := ledger.Apply(ev); err != nil {
		t.Fatal(err)
	}
}

func setupFinalGateWithIsolatedBuilder(t *testing.T, ledger taskledger.Ledger, traceID string) {
	t.Helper()
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: "task-1", Title: "Build feature", Body: "Add the badge", Bee: "builder", Intent: "feature"},
			{TaskID: taskledger.FinalReviewTaskID, Title: "Merge", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, plan)
	mutation, err := protocol.NewEvent(traceID, "builder-1", 0, protocol.EventMutation, protocol.MutationPayload{
		Kind:      protocol.MutationCodeProposalIsolated,
		TaskID:    "task-1",
		Workspace: protocol.ProposalWorkspaceIsolated,
		Diff:      "+line",
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, mutation)
	completed, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind: protocol.TaskEventCompleted, TaskID: "task-1", Status: protocol.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, completed)
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: taskledger.FinalReviewTaskID, Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, waiting)
}

func TestSubmitAnnotatedReviewPlansReworkOnFinalGate(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-final-rework"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	setupFinalGateWithIsolatedBuilder(t, ledger, traceID)

	res, err := review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID:  traceID,
		TaskID:   taskledger.FinalReviewTaskID,
		AgentID:  "console",
		Producer: artifacts.ProducerConsole,
		Packet: review.CommentsPacket{
			Summary: "Fix the error path",
			Comments: []review.ReviewComment{{
				Path: "main.go", Side: "new", StartLine: 3, Body: "handle err",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ReworkTaskID == "" {
		t.Fatal("expected rework task id")
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Tasks[taskledger.FinalReviewTaskID].Status != protocol.TaskStatusWaitingReview {
		t.Fatalf("final status = %q", snap.Tasks[taskledger.FinalReviewTaskID].Status)
	}

	if len(pub.events) != 4 {
		t.Fatalf("events = %d, want 4 (artifact, feedback, plan, ready)", len(pub.events))
	}
	if protocol.PayloadKind(pub.events[2].Payload) != string(protocol.TaskEventPlan) {
		t.Fatalf("event[2] = %s", protocol.PayloadKind(pub.events[2].Payload))
	}
	var plan protocol.TaskPlanPayload
	if err := json.Unmarshal(pub.events[2].Payload, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Tasks) != 1 || plan.Tasks[0].TaskID != res.ReworkTaskID {
		t.Fatalf("plan tasks = %+v", plan.Tasks)
	}
	if plan.Tasks[0].Bee != "builder" || protocol.NormalizeTaskReviewPolicy(plan.Tasks[0].Review) != protocol.TaskReviewNone {
		t.Fatalf("rework spec = %+v", plan.Tasks[0])
	}
	if !strings.Contains(plan.Tasks[0].Body, "review-comments.md") || !strings.Contains(plan.Tasks[0].Body, "task-1") {
		t.Fatalf("rework body = %s", plan.Tasks[0].Body)
	}
	if protocol.PayloadKind(pub.events[3].Payload) != string(protocol.TaskEventReady) {
		t.Fatalf("event[3] = %s", protocol.PayloadKind(pub.events[3].Payload))
	}
}

func TestSubmitAnnotatedReviewFinalFailsClosedWithoutIsolatedBee(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-no-isolated"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	plan, err := protocol.NewEvent(traceID, "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{
			{TaskID: taskledger.FinalReviewTaskID, Title: "Merge", Review: protocol.TaskReviewFinal},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, plan)
	waiting, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: taskledger.FinalReviewTaskID, Status: protocol.TaskStatusWaitingReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, waiting)

	_, err = review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
		Packet: review.CommentsPacket{
			Comments: []review.ReviewComment{{Path: "main.go", Side: "new", StartLine: 1, Body: "note"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "isolated") {
		t.Fatalf("err = %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("expected no bus events")
	}
	combPath := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", review.ReviewCommentsCombRel)
	if _, err := os.Stat(combPath); err == nil {
		t.Fatal("expected no comb file")
	}
}

func TestSubmitAnnotatedReviewFinalFailsClosedWhenReworkInFlight(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-inflight"
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	setupFinalGateWithIsolatedBuilder(t, ledger, traceID)
	rework, err := protocol.NewEvent(traceID, "console", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{TaskID: "task-rework", Title: "Rework", Bee: "builder"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, rework)
	running, err := protocol.NewEvent(traceID, "runtime", 0, protocol.EventSignal, protocol.TaskStatusPayload{
		Kind: protocol.TaskEventStatus, TaskID: "task-rework", Status: protocol.TaskStatusRunning,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustApplyReviewLedger(t, ledger, running)

	_, err = review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
		TraceID: traceID,
		TaskID:  taskledger.FinalReviewTaskID,
		Packet: review.CommentsPacket{
			Comments: []review.ReviewComment{{Path: "main.go", Side: "new", StartLine: 1, Body: "note"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "in flight") {
		t.Fatalf("err = %v", err)
	}
	if len(pub.events) != 0 {
		t.Fatal("expected no bus events")
	}
}

func TestRejectOnFinalDoesNotPlanRework(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	pub := &recordingPublisher{}
	traceID := "trace-plain-reject"
	setupFinalGateWithIsolatedBuilder(t, ledger, traceID)
	if err := review.Reject(context.Background(), pub, ledger, review.RejectInput{
		TraceID:  traceID,
		TaskID:   taskledger.FinalReviewTaskID,
		Feedback: "stop this approach",
		AgentID:  "console",
	}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 {
		t.Fatalf("events = %d, want 1 feedback only", len(pub.events))
	}
	if protocol.PayloadKind(pub.events[0].Payload) != string(protocol.InsightHumanFeedback) {
		t.Fatalf("event = %s", protocol.PayloadKind(pub.events[0].Payload))
	}
}
