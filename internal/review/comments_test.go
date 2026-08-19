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

	ref, err := review.SubmitAnnotatedReview(context.Background(), pub, root, ledger, review.AnnotatedReviewInput{
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
	msg := review.RejectResponseMessage(true, true)
	if !strings.Contains(msg, "Merge gate remains open") {
		t.Fatalf("msg = %q", msg)
	}
}
