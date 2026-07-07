package colony

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const taskIDPrefix = "task-"

// NewTaskID returns a unique task identifier for a trace-local subtask.
// Layout: task- + 8 lowercase hex chars.
func NewTaskID() (string, error) {
	var rnd [4]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", fmt.Errorf("colony: task id: %w", err)
	}
	return taskIDPrefix + hex.EncodeToString(rnd[:]), nil
}
