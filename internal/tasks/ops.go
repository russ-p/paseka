package tasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
)

// Source identifies where a trace snapshot was loaded from.
type Source string

const (
	SourceKV Source = "jetstream-kv"
	SourceFS Source = "filesystem"
)

// LedgerSession holds a connected task ledger and NATS client for mutations.
type LedgerSession struct {
	Colony colony.Context
	Client *bus.Client
	Ledger taskledger.Ledger
	close  func()
}

// Close releases the NATS connection when present.
func (s *LedgerSession) Close() {
	if s.close != nil {
		s.close()
	}
}

// OpenLedger connects to NATS and opens the JetStream KV task ledger.
func OpenLedger(ctx colony.Context) (*LedgerSession, error) {
	client, err := bus.ConnectColony(ctx, false)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return &LedgerSession{Colony: ctx}, nil
	}
	kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(ctx.Slug))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("task ledger kv: %w", err)
	}
	return &LedgerSession{
		Colony: ctx,
		Client: client,
		Ledger: taskledger.NewKVLedger(kv),
		close:  func() { client.Close() },
	}, nil
}

// LoadTrace returns the task snapshot for a trace, preferring KV when populated.
func LoadTrace(ctx colony.Context, ledger taskledger.Ledger, traceID string) (taskledger.TraceSnapshot, Source, error) {
	if traceID == "" {
		return taskledger.TraceSnapshot{}, "", fmt.Errorf("trace id is required")
	}
	if ledger != nil {
		snap, err := ledger.Snapshot(traceID)
		if err != nil {
			return taskledger.TraceSnapshot{}, "", err
		}
		if len(snap.Tasks) > 0 {
			return snap, SourceKV, nil
		}
	}
	snap, err := runs.LoadTraceTasksFromFS(ctx.ColonyRoot, traceID)
	if err != nil {
		return taskledger.TraceSnapshot{}, "", err
	}
	return snap, SourceFS, nil
}

// CreateInput describes a new task to publish via task.plan.
type CreateInput struct {
	TraceID   string
	TaskID    string
	Title     string
	Body      string
	Bee       string
	Sector    string
	Intent    string
	DependsOn []string
	Autorun   bool
	AgentID   string
}

// CreateResult is returned after a task is created.
type CreateResult struct {
	TraceID string
	TaskID  string
	Bee     string
	Autorun bool
}

// Create publishes task.plan and optionally task.ready for a new task.
func Create(ctx context.Context, session *LedgerSession, in CreateInput) (CreateResult, error) {
	if session == nil || session.Client == nil {
		return CreateResult{}, fmt.Errorf("nats url not configured (task create requires NATS)")
	}

	resolvedTitle := DeriveTitle(in.Title, in.Body)
	if resolvedTitle == "" && strings.TrimSpace(in.Body) == "" {
		return CreateResult{}, fmt.Errorf("title and/or body is required")
	}

	traceID := in.TraceID
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return CreateResult{}, err
		}
		traceID = id
	}
	taskID := in.TaskID
	if taskID == "" {
		id, err := colony.NewTaskID()
		if err != nil {
			return CreateResult{}, err
		}
		taskID = id
	}
	bee := in.Bee
	if bee == "" {
		bee = "builder"
	}
	if in.Sector != "" {
		manifest, err := colony.LoadColony(session.Colony.ColonyRoot)
		if err != nil {
			return CreateResult{}, err
		}
		if _, err := manifest.ResolveSector(in.Sector); err != nil {
			return CreateResult{}, err
		}
	}

	agentID := in.AgentID
	if agentID == "" {
		agentID = "cli"
	}

	spec := protocol.TaskSpec{
		TaskID:    taskID,
		Title:     resolvedTitle,
		Body:      strings.TrimSpace(in.Body),
		Bee:       bee,
		Sector:    in.Sector,
		Intent:    in.Intent,
		DependsOn: ParseDependsOn(in.DependsOn),
	}
	planEv, err := PlanEvent(traceID, agentID, spec)
	if err != nil {
		return CreateResult{}, err
	}
	if err := session.Client.PublishEvent(ctx, planEv); err != nil {
		return CreateResult{}, err
	}

	if in.Autorun {
		readyEv, err := ReadyEvent(traceID, agentID, taskledger.TaskSnapshot{
			TaskID: taskID,
			Title:  resolvedTitle,
			Body:   strings.TrimSpace(in.Body),
			Bee:    bee,
			Sector: in.Sector,
			Intent: in.Intent,
		})
		if err != nil {
			return CreateResult{}, err
		}
		if err := session.Client.PublishEvent(ctx, readyEv); err != nil {
			return CreateResult{}, err
		}
	}

	return CreateResult{
		TraceID: traceID,
		TaskID:  taskID,
		Bee:     bee,
		Autorun: in.Autorun,
	}, nil
}

// Start publishes task.ready for one eligible task in a trace.
func Start(ctx context.Context, session *LedgerSession, traceID, taskID, agentID string) ([]taskledger.TaskSnapshot, error) {
	if session == nil || session.Client == nil {
		return nil, fmt.Errorf("nats url not configured (task start requires JetStream KV)")
	}
	if traceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}

	snap, err := session.Ledger.Snapshot(traceID)
	if err != nil {
		return nil, err
	}
	toStart, err := TasksToStart(snap, taskID)
	if err != nil {
		return nil, err
	}
	if agentID == "" {
		agentID = "cli"
	}

	var started []taskledger.TaskSnapshot
	for _, task := range toStart {
		ev, err := ReadyEvent(traceID, agentID, task)
		if err != nil {
			return nil, err
		}
		if err := session.Client.PublishEvent(ctx, ev); err != nil {
			return nil, err
		}
		started = append(started, task)
	}
	return started, nil
}

// TasksToStart resolves which task(s) should receive task.ready.
func TasksToStart(snap taskledger.TraceSnapshot, taskID string) ([]taskledger.TaskSnapshot, error) {
	if taskID != "" {
		task, err := taskledger.CanStart(snap, taskID)
		if err == taskledger.ErrTaskAlreadyReady {
			return nil, fmt.Errorf("task %q is already ready", taskID)
		}
		if err != nil {
			return nil, err
		}
		return []taskledger.TaskSnapshot{task}, nil
	}
	eligible := taskledger.EligiblePlanned(snap)
	if len(eligible) == 0 {
		return nil, taskledger.ErrNoEligibleTasks
	}
	return []taskledger.TaskSnapshot{eligible[0]}, nil
}

// PlanEvent builds an INSIGHT/task.plan event.
func PlanEvent(traceID, agentID string, spec protocol.TaskSpec) (protocol.Event, error) {
	return protocol.NewEvent(traceID, agentID, 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind:  protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{spec},
	})
}

// ReadyEvent builds a SIGNAL/task.ready event.
func ReadyEvent(traceID, agentID string, task taskledger.TaskSnapshot) (protocol.Event, error) {
	bee := task.Bee
	if bee == "" {
		bee = "builder"
	}
	return protocol.NewEvent(traceID, agentID, 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: task.TaskID,
		Title:  task.Title,
		Body:   task.Body,
		Bee:    bee,
		Sector: task.Sector,
		Intent: task.Intent,
	})
}

// DeriveTitle returns an explicit title or the first non-empty body line.
func DeriveTitle(title, body string) string {
	if strings.TrimSpace(title) != "" {
		return strings.TrimSpace(title)
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:120]
			}
			return line
		}
	}
	return ""
}

// ParseDependsOn splits comma-separated dependency lists.
func ParseDependsOn(values []string) []string {
	var out []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// CanStartTask reports whether a task may be started from the UI.
func CanStartTask(snap taskledger.TraceSnapshot, taskID string) bool {
	_, err := taskledger.CanStart(snap, taskID)
	return err == nil
}
