package colony

import (
	"github.com/paseka/paseka/internal/ids"
)

const taskIDPrefix = "task-"

// NewTaskID returns a lexicographically sortable task identifier for a trace-local subtask.
// Layout: task- + 16 lowercase hex chars (48-bit UTC ms + 16-bit random).
func NewTaskID() (string, error) {
	return ids.Prefixed(taskIDPrefix)
}
