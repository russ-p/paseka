package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/invites"
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
	inflight        map[string]context.CancelFunc
	directInflight  map[string]context.CancelFunc
	directProcessed map[string]struct{}
	recentLocal     map[string]time.Time // fingerprints of events applied before publish
	asyncDispatch   bool
	invitePublisher invites.EventPublisher // test override for auto-invite publish
	autoInvites     []colony.AutoInviteRule
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

	manifest, err := colony.LoadColony(ctxColony.ColonyRoot)
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
		inflight:        make(map[string]context.CancelFunc),
		directInflight:  make(map[string]context.CancelFunc),
		directProcessed: make(map[string]struct{}),
		recentLocal:     make(map[string]time.Time),
		asyncDispatch:   true,
		autoInvites:     manifest.AutoInvites,
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

	if err := r.validateIncomingTaskPlan(ev); err != nil {
		return err
	}

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
	if err := r.handlePostApplySideEffects(ctx, ev); err != nil {
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
	if r.traceKilled(ev.TraceID) {
		return nil
	}
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

func (r *Reactor) handlePostApplySideEffects(ctx context.Context, ev protocol.Event) error {
	if energyAddDetected(ev) {
		if err := r.unblockEnergyBlockedTasks(ctx, ev.TraceID); err != nil {
			return err
		}
	}
	if systemKillDetected(ev) {
		r.cancelInflightForTrace(ev.TraceID)
	}
	if err := r.handleReviewSideEffects(ctx, ev); err != nil {
		return err
	}
	if err := r.handleVerificationSuccess(ctx, ev); err != nil {
		return err
	}
	if err := r.handleInviteCompletion(ctx, ev); err != nil {
		return err
	}
	if err := r.handleAutoInvite(ctx, ev); err != nil {
		return err
	}
	return r.handleInviteProjection(ev)
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
		colony:          colony.Context{ColonyRoot: opts.ColonyRoot},
		dispatcher:      opts.Dispatcher,
		ledger:          opts.Ledger,
		registry:        opts.Registry,
		inflight:        make(map[string]context.CancelFunc),
		directInflight:  make(map[string]context.CancelFunc),
		directProcessed: make(map[string]struct{}),
		recentLocal:     make(map[string]time.Time),
		asyncDispatch:   opts.AsyncDispatch,
		autoInvites:     autoInvites,
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
