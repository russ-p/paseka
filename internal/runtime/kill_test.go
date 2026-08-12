package runtime_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/taskledger"
)

type ctxBlockingAdapter struct {
	started chan struct{}
	once    sync.Once
}

func (a *ctxBlockingAdapter) Name() string { return "cursor" }

func (a *ctxBlockingAdapter) Run(ctx context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	a.once.Do(func() { close(a.started) })
	<-ctx.Done()
	return &adapters.RunResult{Status: string(protocol.StatusCancelled)}, ctx.Err()
}

func TestReactorKillCancelsInflightDispatch(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}
	kill, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind:   protocol.SignalSystemKill,
		Reason: "stop loop",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newAsyncTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}

	blocker := &ctxBlockingAdapter{started: make(chan struct{})}
	r.Dispatcher().RegisterAdapter("cursor", blocker)

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- r.ProcessEvent(context.Background(), ready)
	}()

	select {
	case <-blocker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for adapter start")
	}

	if err := r.ProcessEvent(context.Background(), kill); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to finish after kill")
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Killed {
		t.Fatal("expected trace killed")
	}
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled", task.Status)
	}
	if task.Summary != "stop loop" {
		t.Fatalf("summary = %q", task.Summary)
	}
}

func TestReactorSkipsDispatchWhenTraceKilled(t *testing.T) {
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}
	killEv, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
		Kind: protocol.SignalSystemKill,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(killEv); err != nil {
		t.Fatal(err)
	}

	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want 0 on killed trace", rec.calls)
	}
}

// killThenCompleteAdapter applies system.kill mid-run, then returns completed — reproduces
// the race where a fast script bee finishes while kill is in flight.
type killThenCompleteAdapter struct {
	kill func()
}

func (a *killThenCompleteAdapter) Name() string { return "cursor" }

func (a *killThenCompleteAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	if a.kill != nil {
		a.kill()
	}
	return &adapters.RunResult{Status: string(protocol.StatusCompleted), Summary: "fast done"}, nil
}

// killThenErrorAdapter applies system.kill mid-run, then returns an error — covers the
// DispatchColonyBee err path (e.g. finalize failing after kill cancelled the ctx).
type killThenErrorAdapter struct {
	kill func()
}

func (a *killThenErrorAdapter) Name() string { return "cursor" }

func (a *killThenErrorAdapter) Run(_ context.Context, _ adapters.RunRequest) (*adapters.RunResult, error) {
	if a.kill != nil {
		a.kill()
	}
	return nil, context.Canceled
}

func TestReactorSuccessPathDoesNotOverwriteCancelAfterKill(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}

	r.Dispatcher().RegisterAdapter("cursor", &killThenCompleteAdapter{
		kill: func() {
			killEv, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
				Kind:   protocol.SignalSystemKill,
				Reason: "eval hard stop",
			})
			if err != nil {
				t.Errorf("kill event: %v", err)
				return
			}
			if err := r.ProcessEvent(context.Background(), killEv); err != nil {
				t.Errorf("apply kill: %v", err)
			}
		},
	})

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Killed {
		t.Fatal("expected trace killed")
	}
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled (success path must not overwrite kill)", task.Status)
	}
	if task.Summary != "eval hard stop" {
		t.Fatalf("summary = %q, want eval hard stop", task.Summary)
	}
}

func TestReactorDispatchErrorPathDoesNotOverwriteCancelAfterKill(t *testing.T) {
	plan, err := protocol.NewEvent("trace-1", "scout", 0, protocol.EventInsight, protocol.TaskPlanPayload{
		Kind: protocol.TaskEventPlan,
		Tasks: []protocol.TaskSpec{{
			TaskID: "task-1",
			Title:  "implement",
			Bee:    "builder",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := protocol.NewEvent("trace-1", "reactor", 0, protocol.EventSignal, protocol.TaskReadyPayload{
		Kind:   protocol.TaskEventReady,
		TaskID: "task-1",
		Bee:    "builder",
	})
	if err != nil {
		t.Fatal(err)
	}

	r := newTestReactor(t, map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "task.ready"}, Dispatch: colony.DispatchTask},
		}},
	})
	ledger := r.Ledger().(*taskledger.MemoryLedger)
	if err := ledger.SeedEnergy("trace-1", 12); err != nil {
		t.Fatal(err)
	}

	r.Dispatcher().RegisterAdapter("cursor", &killThenErrorAdapter{
		kill: func() {
			killEv, err := protocol.NewEvent("trace-1", "cli", 0, protocol.EventSignal, protocol.SystemKillPayload{
				Kind:   protocol.SignalSystemKill,
				Reason: "eval hard stop",
			})
			if err != nil {
				t.Errorf("kill event: %v", err)
				return
			}
			if err := r.ProcessEvent(context.Background(), killEv); err != nil {
				t.Errorf("apply kill: %v", err)
			}
		},
	})

	if err := r.ProcessEvent(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if err := r.ProcessEvent(context.Background(), ready); err != nil {
		t.Fatal(err)
	}

	snap, err := ledger.Snapshot("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Killed {
		t.Fatal("expected trace killed")
	}
	task := snap.Tasks["task-1"]
	if task.Status != protocol.TaskStatusCancelled {
		t.Fatalf("status = %q, want cancelled (dispatch err path must not overwrite kill)", task.Status)
	}
	if task.Summary != "eval hard stop" {
		t.Fatalf("summary = %q, want eval hard stop", task.Summary)
	}
}

func newAsyncTestReactor(t *testing.T, bees map[string]colony.Bee) *runtime.Reactor {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, bees)
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot:    root,
		Dispatcher:    d,
		Registry:      reg,
		Ledger:        taskledger.NewMemoryLedger(),
		AsyncDispatch: true,
	})
}
