package taskledger

import "github.com/paseka/paseka/internal/protocol"

// IsLastWorkTask reports whether currentTaskID is the sole incomplete non-final-review
// task in the trace ledger. Used at AFK ledger dispatch to gate trace.summary guidance.
func IsLastWorkTask(trace TraceSnapshot, currentTaskID string) bool {
	if currentTaskID == "" {
		return false
	}
	var soleIncompleteID string
	incomplete := 0
	for _, task := range trace.Tasks {
		if IsFinalReviewTask(task) {
			continue
		}
		if task.Status == protocol.TaskStatusCompleted {
			continue
		}
		incomplete++
		soleIncompleteID = task.TaskID
	}
	return incomplete == 1 && soleIncompleteID == currentTaskID
}
