package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// Reactor subscribes to colony events, updates the task ledger, and dispatches ready tasks.
type Reactor struct {
	colony     colony.Context
	bus        *bus.Client
	dispatcher *Dispatcher
	ledger     taskledger.Ledger
	mu         sync.Mutex
	inflight   map[string]struct{}
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

	d := NewDispatcher()
	d.SetPublisher(busClient, true)

	return &Reactor{
		colony:     ctxColony,
		bus:        busClient,
		dispatcher: d,
		ledger:     taskledger.NewKVLedger(kv),
		inflight:   make(map[string]struct{}),
	}, nil
}

// Run blocks until ctx is cancelled, consuming bus events and dispatching ready tasks.
func (r *Reactor) Run(ctx context.Context) error {
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
	res, err := r.ledger.Apply(ev)
	if err != nil {
		return err
	}
	for _, task := range res.Ready {
		if err := r.dispatchReady(context.Background(), ev.TraceID, task); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reactor) dispatchReady(ctx context.Context, traceID string, task taskledger.TaskSnapshot) error {
	key := traceID + ":" + task.TaskID
	r.mu.Lock()
	if _, ok := r.inflight[key]; ok {
		r.mu.Unlock()
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
	body := task.Body
	if body == "" {
		body = task.Title
	}
	if body == "" {
		body = fmt.Sprintf("Execute task %s", task.TaskID)
	}

	res, err := r.dispatcher.BeeRun(ctx, BeeRunRequest{
		StartDir: r.colony.ColonyRoot,
		Bee:      bee,
		TraceID:  traceID,
		TaskID:   task.TaskID,
		Task:     body,
		NoBus:    true,
	})
	if err != nil {
		return err
	}
	if res.Result == nil || res.Result.Status != string(protocol.StatusCompleted) {
		return nil
	}

	completed, err := protocol.NewEvent(traceID, res.AgentID, 0, protocol.EventVerification, protocol.TaskCompletedPayload{
		Kind:    protocol.TaskEventCompleted,
		TaskID:  task.TaskID,
		Status:  protocol.TaskStatusCompleted,
		Summary: strings.TrimSpace(res.Result.Summary),
	})
	if err != nil {
		return err
	}
	if err := r.bus.PublishEvent(ctx, completed); err != nil {
		return err
	}
	applyRes, err := r.ledger.Apply(completed)
	if err != nil {
		return err
	}
	for _, t := range applyRes.Ready {
		if err := r.dispatchReady(ctx, traceID, t); err != nil {
			return err
		}
	}
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
