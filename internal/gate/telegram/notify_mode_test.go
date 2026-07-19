package telegram

import (
	"testing"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestClassifyWaitingReview(t *testing.T) {
	tests := []struct {
		name string
		task taskledger.TaskSnapshot
		want NotifyCategory
	}{
		{
			name: "required",
			task: taskledger.TaskSnapshot{Review: protocol.TaskReviewRequired},
			want: NotifyCategoryReviewRequired,
		},
		{
			name: "final",
			task: taskledger.TaskSnapshot{TaskID: "_review", Review: protocol.TaskReviewFinal},
			want: NotifyCategoryReviewFinal,
		},
		{
			name: "commit gate",
			task: taskledger.TaskSnapshot{Review: protocol.TaskReviewNone},
			want: NotifyCategoryCommitGate,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, ok := classifyTaskStatus(tc.task, protocol.TaskStatusWaitingReview)
			if !ok || cat != tc.want {
				t.Fatalf("got cat=%v ok=%v want %v", cat, ok, tc.want)
			}
		})
	}
}

func TestClassifyTaskStatusBlockedFailed(t *testing.T) {
	task := taskledger.TaskSnapshot{TaskID: "t1"}
	cat, ok := classifyTaskStatus(task, protocol.TaskStatusBlocked)
	if !ok || cat != NotifyCategoryBlocked {
		t.Fatalf("blocked: cat=%v ok=%v", cat, ok)
	}
	cat, ok = classifyTaskStatus(task, protocol.TaskStatusFailed)
	if !ok || cat != NotifyCategoryFailed {
		t.Fatalf("failed: cat=%v ok=%v", cat, ok)
	}
	_, ok = classifyTaskStatus(task, protocol.TaskStatusCompleted)
	if ok {
		t.Fatal("completed status should not classify via task.status path")
	}
}
