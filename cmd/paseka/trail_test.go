package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/trailfork"
)

func TestPrintForkRequestsShowsCreatingRequest(t *testing.T) {
	creating := trailfork.Request{
		Version:   1,
		RequestID: "req-creating",
		State:     trailfork.StateCreating,
		Origin:    "cli",
		Evidence: trailfork.Lineage{
			SourceTrace:   "trace-source",
			SourceTask:    "task-1",
			SourceAttempt: "attempt-recorded",
			Delta:         trailfork.Delta{Form: trailfork.DeltaNone},
			Budget:        8,
			Task:          trailfork.TaskLineage{Title: "carry this task"},
		},
		Validation: "ok",
		LastError:  "bus unavailable during task.ready",
		CreatedAt:  time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC),
		UpdatedAt:  time.Date(2026, 10, 28, 9, 1, 0, 0, time.UTC),
	}
	pending := trailfork.Request{
		Version:   1,
		RequestID: "req-pending",
		State:     trailfork.StatePending,
		Origin:    "agent",
		Sender:    trailfork.Sender{TraceID: "trace-source", AgentID: "agent-sender"},
		Evidence: trailfork.Lineage{
			SourceTrace:   "trace-source",
			SourceTask:    "task-2",
			SourceAttempt: "attempt-pending",
			Delta:         trailfork.Delta{Form: trailfork.DeltaPatch},
			Budget:        4,
		},
		Validation: "ok",
		ExpiresAt:  time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC),
	}

	var out bytes.Buffer
	printForkRequests(&out, []trailfork.Request{creating, pending})
	text := out.String()

	for _, want := range []string{
		"REQUEST", "STATE", "ATTEMPT", "DELTA", "BUDGET", "VALIDATION", "LAST ERROR",
		"req-creating", trailfork.StateCreating, "attempt-recorded", "none", "8",
		"bus unavailable during task.ready",
		"req-pending", trailfork.StatePending, "agent-sender", "patch", "4",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("output does not mention %q:\n%s", want, text)
		}
	}
}
