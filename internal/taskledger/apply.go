package taskledger

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/paseka/paseka/internal/protocol"
)

// ApplyEvent is a pure reducer: given a trace snapshot and one bus event,
// returns the updated snapshot and any tasks that became ready.
// No persistence — suitable for tests and future Ledger implementations.
func ApplyEvent(trace TraceSnapshot, event protocol.Event) (ApplyResult, error) {
	if event.TraceID != "" && trace.TraceID != "" && event.TraceID != trace.TraceID {
		return ApplyResult{}, fmt.Errorf("taskledger: trace mismatch: event %q vs snapshot %q", event.TraceID, trace.TraceID)
	}
	if trace.TraceID == "" && event.TraceID != "" {
		trace.TraceID = event.TraceID
	}
	if trace.Tasks == nil {
		trace.Tasks = make(map[string]TaskSnapshot)
	}

	now := event.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	var ready []TaskSnapshot
	changed := false

	switch event.Type {
	case protocol.EventInsight:
		var payload protocol.TaskPlanPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return ApplyResult{}, fmt.Errorf("taskledger: parse task.plan: %w", err)
		}
		if payload.Kind != protocol.TaskEventPlan {
			return ApplyResult{Trace: trace}, nil
		}
		for _, spec := range payload.Tasks {
			if spec.TaskID == "" {
				continue
			}
			if _, exists := trace.Tasks[spec.TaskID]; exists {
				continue
			}
			trace.Tasks[spec.TaskID] = TaskSnapshot{
				TaskID:    spec.TaskID,
				Title:     spec.Title,
				Body:      spec.Body,
				Bee:       spec.Bee,
				Intent:    spec.Intent,
				Status:    protocol.TaskStatusPlanned,
				DependsOn: append([]string(nil), spec.DependsOn...),
				UpdatedAt: now,
			}
			changed = true
		}

	case protocol.EventSignal:
		var payload protocol.TaskReadyPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return ApplyResult{}, fmt.Errorf("taskledger: parse task.ready: %w", err)
		}
		if payload.Kind != protocol.TaskEventReady {
			return ApplyResult{Trace: trace}, nil
		}
		if payload.TaskID == "" {
			return ApplyResult{}, fmt.Errorf("taskledger: task.ready missing taskId")
		}
		task, ok := trace.Tasks[payload.TaskID]
		if !ok {
			task = TaskSnapshot{TaskID: payload.TaskID}
		}
		if payload.Title != "" {
			task.Title = payload.Title
		}
		if payload.Body != "" {
			task.Body = payload.Body
		}
		if payload.Bee != "" {
			task.Bee = payload.Bee
		}
		if payload.Intent != "" {
			task.Intent = payload.Intent
		}
		if task.Status != protocol.TaskStatusReady {
			task.Status = protocol.TaskStatusReady
			ready = append(ready, task)
			changed = true
		}
		task.UpdatedAt = now
		trace.Tasks[payload.TaskID] = task

	case protocol.EventVerification:
		var payload protocol.TaskCompletedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return ApplyResult{}, fmt.Errorf("taskledger: parse task.completed: %w", err)
		}
		if payload.Kind != protocol.TaskEventCompleted {
			return ApplyResult{Trace: trace}, nil
		}
		if payload.TaskID == "" {
			return ApplyResult{}, fmt.Errorf("taskledger: task.completed missing taskId")
		}
		task, ok := trace.Tasks[payload.TaskID]
		if !ok {
			task = TaskSnapshot{TaskID: payload.TaskID}
		}
		status := payload.Status
		if status == "" {
			status = protocol.TaskStatusCompleted
		}
		task.Status = status
		if payload.Summary != "" {
			task.Summary = payload.Summary
		}
		if payload.Commit != "" {
			task.Commit = payload.Commit
		}
		task.UpdatedAt = now
		if !payload.CompletedAt.IsZero() {
			task.UpdatedAt = payload.CompletedAt
		}
		trace.Tasks[payload.TaskID] = task
		changed = true

		// Unlock dependents whose prerequisites are now completed.
		for id, t := range trace.Tasks {
			if t.Status != protocol.TaskStatusPlanned {
				continue
			}
			if !allDepsCompleted(trace, t.DependsOn) {
				continue
			}
			t.Status = protocol.TaskStatusReady
			t.UpdatedAt = now
			trace.Tasks[id] = t
			ready = append(ready, t)
			changed = true
		}
	}

	return ApplyResult{
		Trace:   trace,
		Ready:   ready,
		Changed: changed,
	}, nil
}

func allDepsCompleted(trace TraceSnapshot, deps []string) bool {
	for _, dep := range deps {
		t, ok := trace.Tasks[dep]
		if !ok || t.Status != protocol.TaskStatusCompleted {
			return false
		}
	}
	return true
}

// ApplyEvents folds a sequence of events over an initial trace snapshot.
func ApplyEvents(trace TraceSnapshot, events []protocol.Event) (TraceSnapshot, error) {
	for _, ev := range events {
		res, err := ApplyEvent(trace, ev)
		if err != nil {
			return TraceSnapshot{}, err
		}
		trace = res.Trace
	}
	return trace, nil
}
