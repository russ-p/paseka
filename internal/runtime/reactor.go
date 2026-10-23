package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/russ-p/paseka/internal/bus"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/review"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/taskledger"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/russ-p/paseka/internal/trailview"
)

// Reactor subscribes to colony events, updates the task ledger, and dispatches ready tasks.
type Reactor struct {
	colony           colony.Context
	publisher        bus.Publisher
	subscriber       bus.EventSubscriber
	close            func()
	subjectPrefix    string
	replayer         bus.TraceReplayer
	dispatcher       *Dispatcher
	ledger           taskledger.Ledger
	registry         *BeeRegistry
	mu               sync.Mutex
	inflight         map[string]context.CancelFunc
	directInflight   map[string]context.CancelFunc
	directProcessed  map[string]struct{}
	recentLocal      map[string]time.Time // fingerprints of events applied before publish
	asyncDispatch    bool
	invitePublisher  bus.Publisher // test override for auto-invite publish
	autoInvites      []colony.AutoInviteRule
	configMu         sync.Mutex
	configRegistries map[string]*BeeRegistry
}

type trailExecutionContext struct {
	Colony   colony.Context
	Manifest colony.Colony
	Registry *BeeRegistry
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

	registry, err := BuildBeeRegistryFromContext(ctxColony)
	if err != nil {
		busClient.Close()
		return nil, err
	}

	manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
	if err != nil {
		busClient.Close()
		return nil, err
	}

	d := NewDispatcher()
	d.SetPublisher(busClient, true)
	d.SetBeeRegistry(registry)

	return &Reactor{
		colony:           ctxColony,
		publisher:        busClient,
		subscriber:       busClient,
		close:            busClient.Close,
		subjectPrefix:    busClient.Config().SubjectPrefix,
		replayer:         busClient,
		dispatcher:       d,
		ledger:           taskledger.NewKVLedger(kv),
		registry:         registry,
		inflight:         make(map[string]context.CancelFunc),
		directInflight:   make(map[string]context.CancelFunc),
		directProcessed:  make(map[string]struct{}),
		recentLocal:      make(map[string]time.Time),
		asyncDispatch:    true,
		autoInvites:      manifest.AutoInvites,
		configRegistries: make(map[string]*BeeRegistry),
	}, nil
}

// Run blocks until ctx is cancelled, consuming bus events and dispatching ready tasks.
func (r *Reactor) Run(ctx context.Context) error {
	if r.close != nil {
		defer r.close()
	}
	if r.subscriber == nil {
		return fmt.Errorf("runtime: bus subscriber not configured")
	}
	subject := bus.EventsWildcard(r.subjectPrefix)
	runtimeLog.Info("listening",
		logging.F("subject", subject),
		logging.F("colony", r.colony.Slug),
	)
	sub, err := r.subscriber.SubscribeEvents("", r.handleEvent)
	if err != nil {
		return err
	}
	defer func() { _ = sub.Unsubscribe() }()

	// A pending configuration transition and its admission barrier survive a
	// restart. Recover them before any new event can be admitted.
	if err := r.RecoverPendingConfigs(ctx); err != nil {
		runtimeLog.Warn("recover pending configuration failed", logging.F("error", err.Error()))
	}
	// An interrupted fork approval resumes under its recorded request identity
	// before any new event can plan the same initial task again.
	r.recoverInterruptedForkCreations(ctx)

	go r.runPullRequestReconcile(ctx)

	<-ctx.Done()
	return ctx.Err()
}

const pullRequestReconcileInterval = 30 * time.Second

func (r *Reactor) runPullRequestReconcile(ctx context.Context) {
	ticker := time.NewTicker(pullRequestReconcileInterval)
	defer ticker.Stop()
	_ = review.ReconcileAllPublished(ctx, r.colony, r.publisher, r.ledger, review.WriteOptions{AfterApply: r.rememberLocalEvent})
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := review.ReconcileAllPublished(ctx, r.colony, r.publisher, r.ledger, review.WriteOptions{AfterApply: r.rememberLocalEvent}); err != nil {
				runtimeLog.Warn("pull request reconcile failed", logging.F("error", err.Error()))
			}
		}
	}
}

func (r *Reactor) handleEvent(ev protocol.Event) error {
	return r.processEvent(context.Background(), ev)
}

func (r *Reactor) processEvent(ctx context.Context, ev protocol.Event) error {
	logEventReceived(ev)

	// Fork control is admission, not trail work. Intercept it before the trail
	// binding is required so a rejected request leaves no runnable trace.
	if handled, err := r.handleForkControl(ctx, ev); err != nil {
		return err
	} else if handled {
		return nil
	}

	// A new trail owns its configuration from its first event onward. Capture
	// that binding before any configuration-dependent validation or routing, so
	// the event cannot be judged by live files that a later edit would replace.
	// A pre-existing unbound trail keeps routing on the live colony.
	execCtx, err := r.executionContext(ev.TraceID)
	if err != nil {
		return fmt.Errorf("runtime: trail config: %w", err)
	}
	r.stampIncomingProvenance(&ev)

	if err := r.validateIncomingTaskPlan(ev, execCtx); err != nil {
		return err
	}

	// Runtime-generated events are applied before publish; skip the JetStream echo
	// so non-idempotent reducers (energy.consume) are not applied twice.
	if r.takeLocalEcho(ev) {
		logLedgerOutcome(ev.TraceID, 0)
		return nil
	}

	stale, err := r.staleTaskCompletion(ev)
	if err != nil {
		return err
	}
	if stale {
		return nil
	}
	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	if res.Changed {
		r.syncTaskProjection(res.Trace)
	}
	if taskID := protocol.PayloadTaskID(ev.Payload); taskID != "" {
		if task, ok := res.Trace.Tasks[taskID]; ok && terminalTaskStatus(task.Status) {
			if err := r.closeCompletedTaskAttempt(ev.TraceID, taskID, ev.AttemptID); err != nil {
				runtimeLog.Warn("close task attempt failed", logging.F("error", err.Error()))
			}
		}
	}
	if systemKillDetected(ev) {
		if err := trailconfig.CloseTraceAttempts(r.colony.ColonyRoot, ev.TraceID, "trace killed"); err != nil {
			runtimeLog.Warn("close attempts after kill failed", logging.F("error", err.Error()))
		}
		r.cancelPendingConfig(ev.TraceID, "trace killed")
	}
	logLedgerOutcome(ev.TraceID, len(res.Ready))

	// A transition that became safe at this boundary switches future work
	// before the event's ready tasks are admitted. Existing attempt-owned work
	// still resolves its own recorded context.
	if _, err := r.tryActivateConfig(ctx, ev.TraceID, false); err != nil {
		return err
	}
	ready := res.Ready
	refreshed, configChanged, err := r.refreshExecutionIfChanged(execCtx, ev.TraceID)
	if err != nil {
		return err
	}
	if configChanged {
		execCtx = refreshed
		if ready, err = r.readyTasksInLedger(ev.TraceID); err != nil {
			return err
		}
	}
	if err := r.handlePostApplySideEffects(ctx, ev, execCtx); err != nil {
		return err
	}
	if err := r.executeDispatches(ctx, ev, ready, execCtx); err != nil {
		return err
	}
	if ev.Type == protocol.EventVerification && protocol.PayloadKind(ev.Payload) == string(protocol.TaskEventCompleted) {
		return r.maybeActivateFinalReview(ctx, ev.TraceID)
	}
	return nil
}

func (r *Reactor) executionContext(traceID string) (*trailExecutionContext, error) {
	if r.colony.ColonyRoot == "" {
		return nil, fmt.Errorf("colony root is required")
	}
	// Resolve the trail's lineage before admitting work. An experimental trail
	// must dispatch through its pinned binding; it must never fall back to the
	// live colony because a binding or lineage record is missing.
	scope, err := trailview.ResolveScope(r.colony, traceID)
	if err != nil {
		return nil, fmt.Errorf("runtime: resolve trail scope: %w", err)
	}
	// A dropped fork keeps its records but can never dispatch again, even if a
	// later ledger reset forgot the kill.
	if scope.AdmissionState == trailfork.StateDropped {
		return nil, fmt.Errorf("runtime: trail %s was dropped and cannot dispatch", traceID)
	}
	bound, manifest, isBound, err := trailconfig.BindNewWork(r.colony, traceID)
	if err != nil {
		return nil, err
	}
	if !isBound {
		if scope.Experimental {
			return nil, fmt.Errorf("runtime: experimental trail %s has no configuration binding", traceID)
		}
		return r.liveExecutionContext()
	}
	if scope.Experimental {
		if manifest.Policy != trailconfig.PolicyPinned {
			return nil, fmt.Errorf("runtime: experimental trail %s has non-pinned policy %q", traceID, manifest.Policy)
		}
		if bound.ConfigRef != "" && scope.ConfigRef != "" && bound.ConfigRef != scope.ConfigRef {
			return nil, fmt.Errorf("runtime: experimental trail %s resolved a configuration the lineage does not pin", traceID)
		}
	}
	registry, err := r.registryForConfig(bound)
	if err != nil {
		return nil, err
	}
	colonyManifest, err := colony.LoadColony(bound.ConfigRootPath())
	if err != nil {
		return nil, err
	}
	return &trailExecutionContext{
		Colony:   bound,
		Manifest: colonyManifest,
		Registry: registry,
	}, nil
}

// liveExecutionContext keeps a pre-capture legacy trail on the colony's live
// configuration. It is not an immutable binding and must never be persisted as
// one.
func (r *Reactor) liveExecutionContext() (*trailExecutionContext, error) {
	colonyManifest, err := colony.LoadColony(r.colony.ConfigRootPath())
	if err != nil {
		return nil, err
	}
	registry, err := r.registryForConfig(r.colony)
	if err != nil {
		return nil, err
	}
	return &trailExecutionContext{
		Colony:   r.colony,
		Manifest: colonyManifest,
		Registry: registry,
	}, nil
}

func (r *Reactor) registryForConfig(ctx colony.Context) (*BeeRegistry, error) {
	if ctx.ConfigRef == "" {
		return r.registry, nil
	}
	r.configMu.Lock()
	defer r.configMu.Unlock()
	if r.configRegistries == nil {
		r.configRegistries = make(map[string]*BeeRegistry)
	}
	if cached := r.configRegistries[ctx.ConfigRef]; cached != nil {
		return cached, nil
	}
	registry, err := BuildBeeRegistryFromContext(ctx)
	if err != nil {
		return nil, err
	}
	r.configRegistries[ctx.ConfigRef] = registry
	return registry, nil
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

func (r *Reactor) executeDispatches(ctx context.Context, ev protocol.Event, ready []taskledger.TaskSnapshot, execCtx *trailExecutionContext) error {
	if r.traceKilled(ev.TraceID) {
		return nil
	}
	dispatched := false

	for _, task := range ready {
		bee := colony.EffectiveTaskBee(task.Bee, execCtx.Manifest.Defaults)
		if execCtx.Registry.CanDispatchTaskReady(bee) {
			logTaskDispatchPlan(ev.TraceID, task.TaskID, bee)
			dispatched = true
		}
		task := task
		execCtx := execCtx
		sourceEventID := eventIdentity(ev)
		if err := r.runDispatch(ctx, func() error {
			return r.dispatchReady(ctx, ev.TraceID, task, execCtx, sourceEventID)
		}); err != nil {
			return err
		}
	}

	directCtx, skipDirect, err := r.directRoutingContext(ev, execCtx)
	if err != nil {
		return err
	}
	if skipDirect {
		logDispatchSkip("attempt closed or unknown", ev.TraceID, protocol.PayloadTaskID(ev.Payload), "")
		return nil
	}
	directBees := directCtx.Registry.DirectSubscribers(ev)
	if len(directBees) > 0 {
		logDirectDispatchPlan(ev.TraceID, ev, directBees)
		dispatched = true
	}
	for _, beeRole := range directBees {
		beeRole := beeRole
		directCtx := directCtx
		if err := r.runDispatch(ctx, func() error {
			return r.dispatchDirect(ctx, ev, beeRole, directCtx)
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

func (r *Reactor) handlePostApplySideEffects(ctx context.Context, ev protocol.Event, execCtx *trailExecutionContext) error {
	if energyAddDetected(ev) {
		if err := r.unblockEnergyBlockedTasks(ctx, ev.TraceID); err != nil {
			return err
		}
	}
	if systemKillDetected(ev) {
		r.cancelInflightForTrace(ev.TraceID)
	}
	if err := r.handleReviewSideEffects(ctx, ev, execCtx); err != nil {
		return err
	}
	if err := r.handleVerificationSuccess(ctx, ev, execCtx); err != nil {
		return err
	}
	if err := r.handleInviteCompletion(ctx, ev); err != nil {
		return err
	}
	if err := r.handleAutoInvite(ctx, ev, execCtx); err != nil {
		return err
	}
	if err := r.handleWorktreeBranch(ctx, ev); err != nil {
		return err
	}
	return r.handleInviteProjection(ev)
}

// PublishEvent injects a domain event onto the bus (used by paseka signal).
func (r *Reactor) PublishEvent(ctx context.Context, event protocol.Event) error {
	if !bus.PublisherAvailable(r.publisher) {
		return fmt.Errorf("runtime: bus publisher not configured")
	}
	return r.publisher.PublishEvent(ctx, event)
}

// Ledger returns the reactor task ledger.
func (r *Reactor) Ledger() taskledger.Ledger {
	return r.ledger
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
	ColonyRoot    string
	Dispatcher    *Dispatcher
	Registry      *BeeRegistry
	Ledger        taskledger.Ledger
	AutoInvites   []colony.AutoInviteRule
	AsyncDispatch bool
}

// NewTestReactor builds a reactor with injected dependencies.
func NewTestReactor(opts TestReactorOptions) *Reactor {
	autoInvites := opts.AutoInvites
	if autoInvites == nil && opts.ColonyRoot != "" {
		if manifest, err := colony.LoadColony(opts.ColonyRoot); err == nil {
			if autoInvites == nil {
				autoInvites = manifest.AutoInvites
			}
		}
	}
	return &Reactor{
		colony:           colony.Context{ColonyRoot: opts.ColonyRoot},
		dispatcher:       opts.Dispatcher,
		ledger:           opts.Ledger,
		registry:         opts.Registry,
		inflight:         make(map[string]context.CancelFunc),
		directInflight:   make(map[string]context.CancelFunc),
		directProcessed:  make(map[string]struct{}),
		recentLocal:      make(map[string]time.Time),
		asyncDispatch:    opts.AsyncDispatch,
		autoInvites:      autoInvites,
		configRegistries: make(map[string]*BeeRegistry),
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
