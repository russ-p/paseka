package colony_test

import (
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestNewTaskIDFormat(t *testing.T) {
	id, err := colony.NewTaskID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "task-") {
		t.Fatalf("id = %q, want task- prefix", id)
	}
	if len(id) != len("task-")+16 {
		t.Fatalf("id = %q, want 16 hex chars after prefix", id)
	}
}
