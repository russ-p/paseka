package review_test

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestReworkTaskBodyPointsAtCombAndOriginalIntent(t *testing.T) {
	body := review.ReworkTaskBody(taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "Build feature",
		Body:   "Add the badge to the header.",
		Bee:    "builder",
	})
	for _, want := range []string{
		"review-comments.md",
		"ArtifactsDir",
		"task-1",
		"Build feature",
		"Add the badge to the header.",
		"Do not merge",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
}

func TestEnsureRequestChangesRequiresIsolatedBee(t *testing.T) {
	snap := taskledger.TraceSnapshot{
		TraceID: "t",
		Tasks: map[string]taskledger.TaskSnapshot{
			"_review": {TaskID: "_review", Review: protocol.TaskReviewFinal, Status: protocol.TaskStatusWaitingReview},
		},
	}
	if _, err := review.EnsureRequestChanges(snap); err == nil {
		t.Fatal("expected error")
	}
}
