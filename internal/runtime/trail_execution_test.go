package runtime_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestTrailExecutionContextIsolatesRoutingAndPrompts(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "feature.requested"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)

	writeTrailConfigFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: cursor\nprompt_template: builder.md\nsubscribes:\n  - type: SIGNAL\n    kind: feature.requested\n    dispatch: direct\n")
	writeTrailConfigFile(t, root, ".paseka/prompts/builder.md", "ORIGINAL {{.Task}}")

	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	rec := &recordingAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})

	if err := r.ProcessEvent(context.Background(), featureEvent(t, "trace-a", "agent-a1", "first")); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "builder" || !strings.Contains(rec.lastReq.Prompt, "ORIGINAL first") {
		t.Fatalf("first trail dispatch = bee %q prompt %q", rec.lastReq.Bee, rec.lastReq.Prompt)
	}

	writeTrailConfigFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: cursor\nprompt_template: builder.md\n")
	writeTrailConfigFile(t, root, ".paseka/bees/writer.yaml", "role: writer\nadapter: cursor\nprompt_template: writer.md\nsubscribes:\n  - type: SIGNAL\n    kind: feature.requested\n    dispatch: direct\n")
	writeTrailConfigFile(t, root, ".paseka/prompts/writer.md", "REPLACEMENT {{.Task}}")

	if err := r.ProcessEvent(context.Background(), featureEvent(t, "trace-b", "agent-b1", "second")); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "writer" || !strings.Contains(rec.lastReq.Prompt, "REPLACEMENT second") {
		t.Fatalf("second trail dispatch = bee %q prompt %q", rec.lastReq.Bee, rec.lastReq.Prompt)
	}

	if err := r.ProcessEvent(context.Background(), featureEvent(t, "trace-a", "agent-a2", "third")); err != nil {
		t.Fatal(err)
	}
	if rec.lastReq.Bee != "builder" || !strings.Contains(rec.lastReq.Prompt, "ORIGINAL third") {
		t.Fatalf("captured trail dispatch changed: bee %q prompt %q", rec.lastReq.Bee, rec.lastReq.Prompt)
	}
}

func featureEvent(t *testing.T, traceID, agentID, title string) protocol.Event {
	t.Helper()
	ev, err := protocol.NewEvent(traceID, agentID, 0, protocol.EventSignal, map[string]any{
		"kind":  "feature.requested",
		"title": title,
		"body":  title,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ev
}

func writeTrailConfigFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

type trailCaptureAdapter struct {
	mu      sync.Mutex
	prompts map[string][]string
	bees    map[string][]string
}

func (a *trailCaptureAdapter) Name() string { return "cursor" }

func (a *trailCaptureAdapter) Run(_ context.Context, req adapters.RunRequest) (*adapters.RunResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.prompts == nil {
		a.prompts = make(map[string][]string)
		a.bees = make(map[string][]string)
	}
	a.prompts[req.TraceID] = append(a.prompts[req.TraceID], req.Prompt)
	a.bees[req.TraceID] = append(a.bees[req.TraceID], req.Bee)
	return &adapters.RunResult{Status: "completed", Output: "ok"}, nil
}

func TestConcurrentTrailsKeepTheirExecutionContexts(t *testing.T) {
	root := t.TempDir()
	bees := map[string]colony.Bee{
		"builder": {Role: "builder", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "feature.requested"}, Dispatch: colony.DispatchDirect},
		}},
	}
	mustWriteTestColony(t, root, bees)
	writeTrailConfigFile(t, root, ".paseka/bees/builder.yaml", "role: builder\nadapter: cursor\nprompt_template: builder.md\nsubscribes:\n  - type: SIGNAL\n    kind: feature.requested\n    dispatch: direct\n")
	writeTrailConfigFile(t, root, ".paseka/prompts/builder.md", "TRACE_A {{.Task}}")

	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	rec := &trailCaptureAdapter{}
	d.RegisterAdapter("cursor", rec)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     taskledger.NewMemoryLedger(),
	})

	if err := r.ProcessEvent(context.Background(), featureEvent(t, "trace-a", "agent-a-seed", "seed-a")); err != nil {
		t.Fatal(err)
	}
	writeTrailConfigFile(t, root, ".paseka/prompts/builder.md", "TRACE_B {{.Task}}")

	const perTrail = 6
	var wg sync.WaitGroup
	errs := make(chan error, perTrail*2)
	type pendingEvent struct {
		ev protocol.Event
	}
	events := make([]pendingEvent, 0, perTrail*2)
	for i := 0; i < perTrail; i++ {
		for _, traceID := range []string{"trace-a", "trace-b"} {
			events = append(events, pendingEvent{
				ev: featureEvent(t, traceID, fmt.Sprintf("agent-%s-%d", traceID, i), fmt.Sprintf("task-%d", i)),
			})
		}
	}
	for _, pending := range events {
		pending := pending
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.ProcessEvent(context.Background(), pending.ev); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, prompt := range rec.prompts["trace-a"] {
		if !strings.Contains(prompt, "TRACE_A") || strings.Contains(prompt, "TRACE_B") {
			t.Fatalf("trace-a used another trail's prompt: %q", prompt)
		}
	}
	for _, prompt := range rec.prompts["trace-b"] {
		if !strings.Contains(prompt, "TRACE_B") || strings.Contains(prompt, "TRACE_A") {
			t.Fatalf("trace-b used another trail's prompt: %q", prompt)
		}
	}
	if len(rec.prompts["trace-a"]) != perTrail+1 || len(rec.prompts["trace-b"]) != perTrail {
		t.Fatalf("dispatch counts = %d/%d", len(rec.prompts["trace-a"]), len(rec.prompts["trace-b"]))
	}
}
