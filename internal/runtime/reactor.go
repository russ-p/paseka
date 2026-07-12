package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
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
	recentLocal     map[string]time.Time // fingerprints of events applied before publish
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
		recentLocal:     make(map[string]time.Time),
		asyncDispatch:   true,
	}, nil
}

// Run blocks until ctx is cancelled, consuming bus events and dispatching ready tasks.
func (r *Reactor) Run(ctx context.Context) error {
	subject := bus.EventsWildcard(r.bus.Config().SubjectPrefix)
	runtimeLog.Info("listening",
		logging.F("subject", subject),
		logging.F("colony", r.colony.Slug),
	)
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

	// Runtime-generated events are applied before publish; skip the JetStream echo
	// so non-idempotent reducers (energy.consume) are not applied twice.
	if r.takeLocalEcho(ev) {
		logLedgerOutcome(ev.TraceID, 0)
		return nil
	}

	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	logLedgerOutcome(ev.TraceID, len(res.Ready))
	if energyAddDetected(ev) {
		if err := r.unblockEnergyBlockedTasks(ctx, ev.TraceID); err != nil {
			return err
		}
	}
	if err := r.handleReviewSideEffects(ctx, ev); err != nil {
		return err
	}
	if err := r.executeDispatches(ctx, ev, res.Ready); err != nil {
		return err
	}
	if ev.Type == protocol.EventVerification && protocol.PayloadKind(ev.Payload) == string(protocol.TaskEventCompleted) {
		return r.maybeActivateFinalReview(ctx, ev.TraceID)
	}
	return nil
}

func eventFingerprint(ev protocol.Event) string {
	return fmt.Sprintf("%s\x00%s\x00%d\x00%s", ev.TraceID, ev.Type, ev.CreatedAt.UnixNano(), string(ev.Payload))
}

func (r *Reactor) rememberLocalEvent(ev protocol.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recentLocal == nil {
		r.recentLocal = make(map[string]time.Time)
	}
	now := time.Now()
	r.recentLocal[eventFingerprint(ev)] = now
	for key, at := range r.recentLocal {
		if now.Sub(at) > 2*time.Minute {
			delete(r.recentLocal, key)
		}
	}
}

func (r *Reactor) takeLocalEcho(ev protocol.Event) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.recentLocal == nil {
		return false
	}
	key := eventFingerprint(ev)
	if _, ok := r.recentLocal[key]; !ok {
		return false
	}
	delete(r.recentLocal, key)
	return true
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
				runtimeLog.Error("dispatch error", logging.F("error", err.Error()))
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

	snap, err := r.ledger.Snapshot(traceID)
	if err != nil {
		return err
	}
	taskSnap := snap.Tasks[task.TaskID]
	if taskSnap.TaskID != "" {
		task = taskSnap
	}
	if taskledger.ShouldSkipDispatch(task) {
		logDispatchSkip("final review gate — no AFK dispatch", traceID, task.TaskID, bee)
		return r.setTaskStatus(ctx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, "Trace ready for human review and merge")
	}

	ok, err := r.gateDispatchEnergy(ctx, traceID, task.TaskID, "task.dispatch")
	if err != nil {
		return err
	}
	if !ok {
		logDispatchSkip("honey reserve exhausted", traceID, task.TaskID, bee)
		return nil
	}

	if err := r.setTaskStatus(ctx, traceID, task.TaskID, protocol.TaskStatusRunning, ""); err != nil {
		return err
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
		Sector:  task.Sector,
		Intent:  task.Intent,
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

	summary := strings.TrimSpace(res.Result.Summary)
	if protocol.NormalizeTaskReviewPolicy(task.Review) == protocol.TaskReviewRequired {
		return r.setTaskStatus(ctx, traceID, task.TaskID, protocol.TaskStatusWaitingReview, summary)
	}
	return r.completeTask(ctx, traceID, task.TaskID, summary, "")
}

func (r *Reactor) dispatchDirect(ctx context.Context, ev protocol.Event, beeRole string) error {
	taskID, taskBody, err := eventDispatchContext(ev)
	if err != nil {
		runtimeLog.Warn("direct dispatch skipped",
			logging.F("bee", beeRole),
			logging.F("error", err.Error()),
		)
		return nil
	}

	if publisherBee := r.publisherBee(ev); publisherBee != "" && publisherBee == beeRole {
		logDispatchSkip("publisher is same bee role", ev.TraceID, taskID, beeRole)
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

	ok, err := r.gateDispatchEnergy(ctx, ev.TraceID, taskID, "direct.dispatch")
	if err != nil {
		return err
	}
	if !ok {
		logDispatchSkip("honey reserve exhausted", ev.TraceID, taskID, beeRole)
		return nil
	}

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

// publisherBee resolves the bee role that emitted an event from its run metadata.
func (r *Reactor) publisherBee(ev protocol.Event) string {
	if r.colony.ColonyRoot == "" || ev.TraceID == "" || ev.AgentID == "" {
		return ""
	}
	meta, ok, err := runs.FindRun(r.colony.ColonyRoot, ev.TraceID, ev.AgentID)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(meta.Bee)
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

// Colony returns the resolved colony context for this reactor.
func (r *Reactor) Colony() colony.Context {
	return r.colony
}

// ProcessEvent applies routing for one bus event (for tests).
func (r *Reactor) ProcessEvent(ctx context.Context, ev protocol.Event) error {
	return r.processEvent(ctx, ev)
}

// RememberLocalEvent records an event fingerprint as already applied (for tests).
func (r *Reactor) RememberLocalEvent(ev protocol.Event) {
	r.rememberLocalEvent(ev)
}

// ApplyAndSyncForTest applies then publishes via the reactor sync path (for tests).
func (r *Reactor) ApplyAndSyncForTest(ctx context.Context, ev protocol.Event) error {
	return r.applyAndSync(ctx, ev)
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
		recentLocal:     make(map[string]time.Time),
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
		runtimeLog.Warn("task projection sync failed", logging.F("error", err.Error()))
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
		runtimeLog.Warn("task run projection failed", logging.F("error", err.Error()))
	}
}

func (r *Reactor) recordTaskRunFinish(traceID, taskID, agentID, runStatus string, finishedAt time.Time) {
	if taskID == "" || agentID == "" {
		return
	}
	if err := runs.UpdateTaskRunStatus(r.colony.ColonyRoot, traceID, taskID, agentID, runStatus, finishedAt); err != nil {
		runtimeLog.Warn("task run projection failed", logging.F("error", err.Error()))
	}
}
