package runtime

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
	"github.com/paseka/paseka/internal/taskledger"
)

// Reactor subscribes to colony events, updates the task ledger, and dispatches ready tasks.
type Reactor struct {
	colony          colony.Context
	bus             *bus.Client
	dispatcher      *Dispatcher
	ledger          taskledger.Ledger
	registry        *BeeRegistry
	mu              sync.Mutex
	inflight        map[string]struct{}
	directInflight  map[string]struct{}
	directProcessed map[string]struct{}
	asyncDispatch   bool
}

// ReactorOptions configures a hive runtime reactor.
type ReactorOptions struct {
	StartDir string
}

// NewReactor wires bus, dispatcher, and ledger for paseka run.
func NewReactor(opts ReactorOptions) (*Reactor, error) {
	ctxColony, err := colony.ResolveContext(opts.StartDir)
	if err != nil {
		return nil, err
	}
	busClient, err := bus.ConnectColony(ctxColony, true)
	if err != nil {
		return nil, err
	}
	if busClient == nil {
		return nil, fmt.Errorf("runtime: nats url not configured (run paseka init)")
	}

	kv, err := busClient.JetStream().KeyValue(bus.TaskLedgerBucket(ctxColony.Slug))
	if err != nil {
		busClient.Close()
		return nil, fmt.Errorf("runtime: task ledger kv: %w", err)
	}

	registry, err := BuildBeeRegistry(ctxColony.ColonyRoot)
	if err != nil {
		busClient.Close()
		return nil, err
	}

	d := NewDispatcher()
	d.SetPublisher(busClient, true)
	d.SetBeeRegistry(registry)

	return &Reactor{
		colony:          ctxColony,
		bus:             busClient,
		dispatcher:      d,
		ledger:          taskledger.NewKVLedger(kv),
		registry:        registry,
		inflight:        make(map[string]struct{}),
		directInflight:  make(map[string]struct{}),
		directProcessed: make(map[string]struct{}),
		asyncDispatch:   true,
	}, nil
}

// Run blocks until ctx is cancelled, consuming bus events and dispatching ready tasks.
func (r *Reactor) Run(ctx context.Context) error {
	subject := bus.EventsWildcard(r.bus.Config().SubjectPrefix)
	log.Printf("runtime: listening subject=%s colony=%s", subject, r.colony.Slug)
	sub, err := r.bus.SubscribeEvents("", r.handleEvent)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	defer r.bus.Close()

	<-ctx.Done()
	return ctx.Err()
}

func (r *Reactor) handleEvent(ev protocol.Event) error {
	return r.processEvent(context.Background(), ev)
}

func (r *Reactor) processEvent(ctx context.Context, ev protocol.Event) error {
	logEventReceived(ev)

	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	logLedgerOutcome(ev.TraceID, len(res.Ready))
	return r.executeDispatches(ctx, ev, res.Ready)
}

func (r *Reactor) executeDispatches(ctx context.Context, ev protocol.Event, ready []taskledger.TaskSnapshot) error {
	dispatched := false

	for _, task := range ready {
		bee := taskBeeName(task)
		if r.registry.CanDispatchTaskReady(bee) {
			logTaskDispatchPlan(ev.TraceID, task.TaskID, bee)
			dispatched = true
		}
		task := task
		if err := r.runDispatch(ctx, func() error {
			return r.dispatchReady(ctx, ev.TraceID, task)
		}); err != nil {
			return err
		}
	}

	directBees := r.registry.DirectSubscribers(ev)
	if len(directBees) > 0 {
		logDirectDispatchPlan(ev.TraceID, ev, directBees)
		dispatched = true
	}
	for _, beeRole := range directBees {
		beeRole := beeRole
		if err := r.runDispatch(ctx, func() error {
			return r.dispatchDirect(ctx, ev, beeRole)
		}); err != nil {
			return err
		}
	}

	if !dispatched {
		logNoDispatch(ev)
	}
	return nil
}

// runDispatch executes a dispatch synchronously or in the background (NATS path).
func (r *Reactor) runDispatch(ctx context.Context, fn func() error) error {
	if r.asyncDispatch {
		go func() {
			if err := fn(); err != nil {
				log.Printf("runtime: dispatch error: %v", err)
			}
		}()
		return nil
	}
	return fn()
}

func (r *Reactor) dispatchReady(ctx context.Context, traceID string, task taskledger.TaskSnapshot) error {
	key := traceID + ":" + task.TaskID
	r.mu.Lock()
	if _, ok := r.inflight[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already running", traceID, task.TaskID, taskBeeName(task))
		return nil
	}
	r.inflight[key] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.inflight, key)
		r.mu.Unlock()
	}()

	bee := task.Bee
	if bee == "" {
		bee = "builder"
	}
	if !r.registry.CanDispatchTaskReady(bee) {
		logDispatchSkip("bee not subscribed to task.ready", traceID, task.TaskID, bee)
		return nil
	}

	body := task.Body
	if body == "" {
		body = task.Title
	}
	if body == "" {
		body = fmt.Sprintf("Execute task %s", task.TaskID)
	}

	startedAt := time.Now().UTC()
	res, err := r.dispatcher.DispatchColonyBee(ctx, r.colony, ColonyDispatchRequest{
		Bee:     bee,
		TraceID: traceID,
		TaskID:  task.TaskID,
		Task:    body,
	}, DispatchModeTask)
	if err != nil {
		return err
	}
	r.recordTaskRunStart(traceID, task.TaskID, bee, res, startedAt)
	if res.Result == nil || res.Result.Status != string(protocol.StatusCompleted) {
		if res.Result != nil {
			r.recordTaskRunFinish(traceID, task.TaskID, res.AgentID, res.Result.Status, time.Now().UTC())
		}
		status := "unknown"
		if res.Result != nil {
			status = res.Result.Status
		}
		logDispatchDone(DispatchModeTask, bee, traceID, task.TaskID, res.AgentID, status)
		return nil
	}
	logDispatchDone(DispatchModeTask, bee, traceID, task.TaskID, res.AgentID, string(protocol.StatusCompleted))
	finishedAt := time.Now().UTC()
	r.recordTaskRunFinish(traceID, task.TaskID, res.AgentID, string(protocol.StatusCompleted), finishedAt)

	completed, err := protocol.NewEvent(traceID, res.AgentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  task.TaskID,
		Status:  protocol.TaskStatusCompleted,
		Summary: strings.TrimSpace(res.Result.Summary),
	})
	if err != nil {
		return err
	}
	if r.bus != nil {
		if err := r.bus.PublishEvent(ctx, completed); err != nil {
			return err
		}
	}
	applyRes, err := r.ledger.Apply(completed)
	if err != nil {
		return err
	}
	if applyRes.Changed {
		r.syncTaskProjection(applyRes.Trace)
	}
	for _, t := range applyRes.Ready {
		if err := r.dispatchReady(ctx, traceID, t); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) dispatchDirect(ctx context.Context, ev protocol.Event, beeRole string) error {
	taskID, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		log.Printf("runtime: direct dispatch skipped for bee %q: %v", beeRole, err)
		return nil
	}

	key := directDispatchKey(ev, beeRole)
	r.mu.Lock()
	if _, ok := r.directProcessed[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already processed", ev.TraceID, taskID, beeRole)
		return nil
	}
	if _, ok := r.directInflight[key]; ok {
		r.mu.Unlock()
		logDispatchSkip("already running", ev.TraceID, taskID, beeRole)
		return nil
	}
	r.directInflight[key] = struct{}{}
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.directInflight, key)
		r.mu.Unlock()
	}()

	res, err := r.dispatcher.DispatchColonyBee(ctx, r.colony, ColonyDispatchRequest{
		Bee:     beeRole,
		TraceID: ev.TraceID,
		TaskID:  taskID,
		Task:    taskBody,
	}, DispatchModeDirect)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.directProcessed[key] = struct{}{}
	r.mu.Unlock()
	status := "unknown"
	if res.Result != nil {
		status = res.Result.Status
	}
	logDispatchDone(DispatchModeDirect, beeRole, ev.TraceID, taskID, res.AgentID, status)
	return nil
}

// PublishEvent injects a domain event onto the bus (used by paseka signal).
func (r *Reactor) PublishEvent(ctx context.Context, event protocol.Event) error {
	return r.bus.PublishEvent(ctx, event)
}

// Ledger returns the reactor task ledger.
func (r *Reactor) Ledger() taskledger.Ledger {
	return r.ledger
}

// BusClient returns the underlying bus client for replay/doctor helpers.
func (r *Reactor) BusClient() *bus.Client {
	return r.bus
}

// Registry returns the bee routing registry.
func (r *Reactor) Registry() *BeeRegistry {
	return r.registry
}

// ColonyRoot returns the resolved colony root for this reactor.
func (r *Reactor) ColonyRoot() string {
	return r.colony.ColonyRoot
}

// ProcessEvent applies routing for one bus event (for tests).
func (r *Reactor) ProcessEvent(ctx context.Context, ev protocol.Event) error {
	return r.processEvent(ctx, ev)
}

// TestReactorOptions configures a reactor without NATS (unit tests).
type TestReactorOptions struct {
	ColonyRoot string
	Dispatcher *Dispatcher
	Registry   *BeeRegistry
	Ledger     taskledger.Ledger
}

// NewTestReactor builds a reactor with injected dependencies.
func NewTestReactor(opts TestReactorOptions) *Reactor {
	return &Reactor{
		colony:          colony.Context{ColonyRoot: opts.ColonyRoot},
		dispatcher:      opts.Dispatcher,
		ledger:          opts.Ledger,
		registry:        opts.Registry,
		inflight:        make(map[string]struct{}),
		directInflight:  make(map[string]struct{}),
		directProcessed: make(map[string]struct{}),
		asyncDispatch:   false,
	}
}

// Dispatcher returns the reactor dispatcher (for tests).
func (r *Reactor) Dispatcher() *Dispatcher {
	return r.dispatcher
}

func (r *Reactor) syncTaskProjection(trace taskledger.TraceSnapshot) {
	if r.colony.ColonyRoot == "" || trace.TraceID == "" {
		return
	}
	if err := runs.SyncTraceTasks(r.colony.ColonyRoot, trace); err != nil {
		log.Printf("runtime: task projection sync: %v", err)
	}
}

func (r *Reactor) recordTaskRunStart(traceID, taskID, bee string, res *BeeRunResult, startedAt time.Time) {
	if res == nil || taskID == "" {
		return
	}
	if err := runs.AppendTaskRun(r.colony.ColonyRoot, traceID, taskID, runs.TaskRunEntry{
		AgentID:   res.AgentID,
		Bee:       bee,
		RunDir:    res.RunDir,
		StartedAt: startedAt,
		RunStatus: "running",
	}); err != nil {
		log.Printf("runtime: task run projection: %v", err)
	}
}

func (r *Reactor) recordTaskRunFinish(traceID, taskID, agentID, runStatus string, finishedAt time.Time) {
	if taskID == "" || agentID == "" {
		return
	}
	if err := runs.UpdateTaskRunStatus(r.colony.ColonyRoot, traceID, taskID, agentID, runStatus, finishedAt); err != nil {
		log.Printf("runtime: task run projection: %v", err)
	}
}
