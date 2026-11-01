package runtime_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/taskledger"
)

func newEnergyObservationReactor(t *testing.T, ledger taskledger.Ledger, replayer fakeReplayer, bees map[string]colony.Bee, pub *recordingPublisher) *runtime.Reactor {
	t.Helper()
	root := t.TempDir()
	mustWriteTestColony(t, root, bees)
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(bees)
	d.SetBeeRegistry(reg)
	return runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     ledger,
		Publisher:  pub,
		Replayer:   replayer,
	})
}

func observationEvents(pub *recordingPublisher, kind protocol.EnergyEventKind) []protocol.Event {
	var out []protocol.Event
	for _, ev := range pub.events {
		if protocol.PayloadKind(ev.Payload) == string(kind) {
			out = append(out, ev)
		}
	}
	return out
}

func decodeObservation[T any](t *testing.T, ev protocol.Event) T {
	t.Helper()
	var payload T
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

// One pause episode produces exactly one paused observation, even when more
// deferrals arrive and even when a second reactor takes over the same ledger.
func TestEnergyPauseObservationIsExactlyOncePerEpisode(t *testing.T) {
	evA := mustCodeProposal(t, "trace-1", "task-a", 1, "+func A() {}")
	evB := mustCodeProposal(t, "trace-1", "task-b", 2, "+func B() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-a", "task-b")
	exhaustEnergy(t, ledger)

	pub := &recordingPublisher{}
	first := newEnergyObservationReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA, evB}}, directGuardBees(), pub)
	firstRec := &recordingAdapter{}
	first.Dispatcher().RegisterAdapter("cursor", firstRec)
	if err := first.ProcessEvent(context.Background(), evA); err != nil {
		t.Fatal(err)
	}

	paused := observationEvents(pub, protocol.SignalEnergyPaused)
	if len(paused) != 1 {
		t.Fatalf("paused observations = %d, want exactly one", len(paused))
	}
	payload := decodeObservation[protocol.EnergyPausedPayload](t, paused[0])
	if payload.EpisodeID == "" {
		t.Fatal("paused observation carries no episode id")
	}
	if payload.Reason != protocol.HoneyReserveExhaustedSummary {
		t.Fatalf("reason = %q", payload.Reason)
	}
	if payload.Remaining != 0 {
		t.Fatalf("remaining = %d, want the exhausted reserve", payload.Remaining)
	}
	if payload.PendingCount != 1 {
		t.Fatalf("pendingCount = %d, want the deferral that opened the episode", payload.PendingCount)
	}
	snap := mustSnapshot(t, ledger, "trace-1")
	if snap.EnergyPause == nil || snap.EnergyPause.EpisodeID != payload.EpisodeID {
		t.Fatalf("ledger pause = %+v, want episode %q", snap.EnergyPause, payload.EpisodeID)
	}

	// A second reactor over the same ledger defers new work into the same
	// episode. The published flag is durable, so it sends no second pause.
	second := newEnergyObservationReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA, evB}}, directGuardBees(), pub)
	secondRec := &recordingAdapter{}
	second.Dispatcher().RegisterAdapter("cursor", secondRec)
	if err := second.ProcessEvent(context.Background(), evB); err != nil {
		t.Fatal(err)
	}
	if got := len(observationEvents(pub, protocol.SignalEnergyPaused)); got != 1 {
		t.Fatalf("paused observations = %d after restart, want exactly one", got)
	}
	if got := len(taskledger.ActivePendingDispatches(mustSnapshot(t, ledger, "trace-1"))); got != 2 {
		t.Fatalf("active pending = %d, want both deferrals", got)
	}
}

// An episode opened while the reactor was down is published once on startup,
// before the drain admits work, and the drain's increase then closes it.
func TestStartupPublishesUnpublishedPauseBeforeDraining(t *testing.T) {
	evA := mustCodeProposal(t, "trace-1", "task-a", 1, "+func A() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-a")
	exhaustEnergy(t, ledger)
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "direct-a",
		TaskID:        "task-a",
		Bee:           "guard",
		SourceEventID: evA.ID,
	})
	if pause := mustSnapshot(t, ledger, "trace-1").EnergyPause; pause == nil || pause.Published {
		t.Fatalf("seeded pause = %+v, want an unpublished episode", pause)
	}

	// A top-up applied with no reactor running raises the reserve; the episode
	// waits for the next startup to announce it.
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":2}`)
	if _, err := ledger.Apply(add); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{}
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA}}, directGuardBees(), pub)
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	if err := r.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}

	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want the approved work resumed", rec.calls)
	}
	pausedIdx, resumedIdx := -1, -1
	for i, ev := range pub.events {
		switch protocol.PayloadKind(ev.Payload) {
		case string(protocol.SignalEnergyPaused):
			if pausedIdx == -1 {
				pausedIdx = i
			}
		case string(protocol.SignalEnergyResumed):
			if resumedIdx == -1 {
				resumedIdx = i
			}
		}
	}
	if pausedIdx == -1 || resumedIdx == -1 {
		t.Fatalf("published observations = %v, want paused and resumed", pub.events)
	}
	if pausedIdx > resumedIdx {
		t.Fatalf("paused at %d, resumed at %d: the pause must be announced before the drain resumes work", pausedIdx, resumedIdx)
	}
	resumed := decodeObservation[protocol.EnergyResumedPayload](t, pub.events[resumedIdx])
	if resumed.Source != protocol.EnergyResumeSourceAdd {
		t.Fatalf("source = %q, want energy.add", resumed.Source)
	}
	if resumed.Dispatched != 1 {
		t.Fatalf("dispatched = %d, want the resumed dispatch", resumed.Dispatched)
	}
	if resumed.PendingBefore != 1 || resumed.PendingAfter != 0 {
		t.Fatalf("pending = %d -> %d, want 1 -> 0", resumed.PendingBefore, resumed.PendingAfter)
	}
	if snap := mustSnapshot(t, ledger, "trace-1"); snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the episode closed after the drain", snap.EnergyPause)
	}

	// A second startup finds both the episode gone and the work closed, so it
	// publishes nothing more.
	before := len(pub.events)
	if err := r.RecoverPendingDispatches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != before {
		t.Fatalf("published %d observations on the second startup, want none", len(pub.events)-before)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d after the second startup, want no duplicate dispatch", rec.calls)
	}
}

// An energy.add that leaves no pending item runnable still closes the episode
// and announces the resume.
func TestEnergyAddClosesPauseWithoutRunnableWork(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTask(t, ledger, "task-a")
	exhaustEnergy(t, ledger)
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID:            "direct-a",
		TaskID:        "task-a",
		Bee:           "guard",
		SourceEventID: "evt-missing",
	})

	pub := &recordingPublisher{}
	// The deferred source event is not replayable, so the entry becomes
	// unresolved instead of dispatching anything.
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{}, directGuardBees(), pub)
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}

	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want no runnable work", rec.calls)
	}
	resumed := observationEvents(pub, protocol.SignalEnergyResumed)
	if len(resumed) != 1 {
		t.Fatalf("resumed observations = %d, want one even with nothing runnable", len(resumed))
	}
	payload := decodeObservation[protocol.EnergyResumedPayload](t, resumed[0])
	if payload.Source != protocol.EnergyResumeSourceAdd {
		t.Fatalf("source = %q", payload.Source)
	}
	if payload.Dispatched != 0 {
		t.Fatalf("dispatched = %d, want 0", payload.Dispatched)
	}
	if payload.PendingBefore != 1 || payload.PendingAfter != 0 {
		t.Fatalf("pending = %d -> %d, want the unresolved entry out of the runnable queue", payload.PendingBefore, payload.PendingAfter)
	}
	if snap := mustSnapshot(t, ledger, "trace-1"); len(snap.PendingDispatches) != 1 ||
		snap.PendingDispatches[0].Status != taskledger.PendingDispatchUnresolved {
		t.Fatalf("pending entries = %+v, want the missing evidence still visible", snap.PendingDispatches)
	}
	if payload.Remaining != 1 {
		t.Fatalf("remaining = %d, want the top-up available again", payload.Remaining)
	}
	if snap := mustSnapshot(t, ledger, "trace-1"); snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the episode closed", snap.EnergyPause)
	}
}

// A stipend closes the episode in its own reducer. The runtime announces the
// close as a resumed observation whose source and abandoned count say the new
// allocation superseded the previous tick instead of resuming it.
func TestStipendClosePublishesResumedWithAbandonedCount(t *testing.T) {
	evA := mustCodeProposal(t, "trace-1", "task-a", 1, "+func A() {}")
	evB := mustCodeProposal(t, "trace-1", "task-b", 2, "+func B() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-a", "task-b")
	exhaustEnergy(t, ledger)
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID: "direct-a", TaskID: "task-a", Bee: "guard", SourceEventID: evA.ID, CreatedAt: time.Now().UTC(),
	})
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID: "direct-b", TaskID: "task-b", Bee: "guard", SourceEventID: evB.ID, CreatedAt: time.Now().UTC().Add(time.Second),
	})

	pub := &recordingPublisher{}
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA, evB}}, directGuardBees(), pub)
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	stipend := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.stipend","amount":4}`)
	if err := r.ProcessEvent(context.Background(), stipend); err != nil {
		t.Fatal(err)
	}

	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want a stipend to supersede rather than dispatch", rec.calls)
	}
	if got := len(observationEvents(pub, protocol.SignalEnergyPaused)); got != 0 {
		t.Fatalf("paused observations = %d, want none for a reducer-owned close", got)
	}
	resumed := observationEvents(pub, protocol.SignalEnergyResumed)
	if len(resumed) != 1 {
		t.Fatalf("resumed observations = %d, want exactly one", len(resumed))
	}
	payload := decodeObservation[protocol.EnergyResumedPayload](t, resumed[0])
	if payload.Source != protocol.EnergyResumeSourceStipend {
		t.Fatalf("source = %q, want energy.stipend", payload.Source)
	}
	if payload.Abandoned != 2 {
		t.Fatalf("abandoned = %d, want both superseded entries", payload.Abandoned)
	}
	if payload.Remaining != 4 {
		t.Fatalf("remaining = %d, want the stipend amount", payload.Remaining)
	}
	snap := mustSnapshot(t, ledger, "trace-1")
	if snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the stipend reducer to clear it", snap.EnergyPause)
	}
	if active := taskledger.ActivePendingDispatches(snap); len(active) != 0 {
		t.Fatalf("active pending = %+v, want the previous tick abandoned", active)
	}
}

// A partial drain that leaves the reserve exhausted keeps the episode active and
// publishes no resumed observation. Only an increase that leaves honey behind
// closes the pause.
func TestPartialDrainKeepsPauseAndPublishesNoResume(t *testing.T) {
	evA := mustCodeProposal(t, "trace-1", "task-a", 1, "+func A() {}")
	evB := mustCodeProposal(t, "trace-1", "task-b", 2, "+func B() {}")
	ledger := taskledger.NewMemoryLedger()
	seedWaitingReviewTasks(t, ledger, "trace-1", "task-a", "task-b")
	exhaustEnergy(t, ledger)
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID: "direct-a", TaskID: "task-a", Bee: "guard", SourceEventID: evA.ID, CreatedAt: time.Now().UTC(),
	})
	mustDeferDirect(t, ledger, "trace-1", taskledger.PendingDispatch{
		ID: "direct-b", TaskID: "task-b", Bee: "guard", SourceEventID: evB.ID, CreatedAt: time.Now().UTC().Add(time.Second),
	})

	pub := &recordingPublisher{}
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{events: []protocol.Event{evA, evB}}, directGuardBees(), pub)
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)
	add := mustEvent(t, "trace-1", "cli", protocol.EventSignal, `{"kind":"energy.add","amount":1}`)
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 1 {
		t.Fatalf("adapter calls = %d, want the one dispatch the honey paid for", rec.calls)
	}
	if got := len(observationEvents(pub, protocol.SignalEnergyResumed)); got != 0 {
		t.Fatalf("resumed observations = %d, want none while the reserve is exhausted", got)
	}
	snap := mustSnapshot(t, ledger, "trace-1")
	if snap.EnergyPause == nil {
		t.Fatal("pause episode closed although the reserve is still exhausted")
	}
	if remainingHoney(t, ledger, "trace-1") != 0 {
		t.Fatalf("remaining honey = %d, want 0", remainingHoney(t, ledger, "trace-1"))
	}

	// A second single honey runs the second entry and still leaves nothing
	// behind, so the episode stays open.
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	if rec.calls != 2 {
		t.Fatalf("adapter calls = %d, want the second entry resumed", rec.calls)
	}
	if got := len(observationEvents(pub, protocol.SignalEnergyResumed)); got != 0 {
		t.Fatalf("resumed observations = %d after the second partial drain, want none", got)
	}
	if snap := mustSnapshot(t, ledger, "trace-1"); snap.EnergyPause == nil {
		t.Fatal("pause episode closed after a second partial drain")
	}

	// With the queue empty the next honey stays in the reserve, so the trail can
	// admit work again and the episode closes.
	if err := r.ProcessEvent(context.Background(), add); err != nil {
		t.Fatal(err)
	}
	resumed := observationEvents(pub, protocol.SignalEnergyResumed)
	if len(resumed) != 1 {
		t.Fatalf("resumed observations = %d, want one once the reserve admits work", len(resumed))
	}
	payload := decodeObservation[protocol.EnergyResumedPayload](t, resumed[0])
	if payload.Dispatched != 0 || payload.PendingBefore != 0 {
		t.Fatalf("resumed payload = %+v, want a closing increase with nothing to dispatch", payload)
	}
	if snap := mustSnapshot(t, ledger, "trace-1"); snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the episode closed", snap.EnergyPause)
	}
}

// Observations are platform signals, not work: they never reach a direct
// subscriber, never enter the deferred queue, and never change the reserve.
func TestEnergyObservationsAreNotRoutedAsWork(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 3); err != nil {
		t.Fatal(err)
	}
	bees := map[string]colony.Bee{
		"guard": {Role: "guard", Subscribes: []colony.SubscriptionRule{
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "energy.paused"}, Dispatch: colony.DispatchDirect},
			{EventRule: colony.EventRule{Type: "SIGNAL", Kind: "energy.resumed"}, Dispatch: colony.DispatchDirect},
		}},
	}
	pub := &recordingPublisher{}
	r := newEnergyObservationReactor(t, ledger, fakeReplayer{}, bees, pub)
	rec := &recordingAdapter{}
	r.Dispatcher().RegisterAdapter("cursor", rec)

	observations := []string{
		`{"kind":"energy.paused","episodeId":"pause-1","reason":"Honey reserve exhausted","remaining":0,"pendingCount":1}`,
		`{"kind":"energy.resumed","episodeId":"pause-1","source":"energy.add","remaining":3,"pendingBefore":1,"pendingAfter":0,"dispatched":1}`,
	}
	for _, payload := range observations {
		ev := mustEvent(t, "trace-1", "runtime", protocol.EventSignal, payload)
		if err := r.ProcessEvent(context.Background(), ev); err != nil {
			t.Fatalf("%s was rejected by the runtime: %v", protocol.PayloadKind(ev.Payload), err)
		}
	}
	if rec.calls != 0 {
		t.Fatalf("adapter calls = %d, want observations to stay out of dispatch", rec.calls)
	}
	snap := mustSnapshot(t, ledger, "trace-1")
	if snap.EnergyRemaining != 3 || snap.EnergyBudget != 3 {
		t.Fatalf("reserve = %+v, want observations not to change it", snap)
	}
	if snap.EnergyPause != nil {
		t.Fatalf("pause = %+v, want observations not to open an episode", snap.EnergyPause)
	}
	if len(snap.PendingDispatches) != 0 {
		t.Fatalf("pending = %+v, want observations not deferred", snap.PendingDispatches)
	}
}

// A manifest auto-invite rule that names an energy observation must not turn the
// observation into an invite.
func TestEnergyObservationIsExcludedFromAutoInvite(t *testing.T) {
	ledger := taskledger.NewMemoryLedger()
	if err := ledger.SeedEnergy("trace-1", 3); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWriteTestColony(t, root, map[string]colony.Bee{
		"drone": {Role: "drone"},
	})
	manifest := `defaults:
  prompt_template: default.md
auto_invites:
  - when:
      type: SIGNAL
      kind: energy.paused
    invite:
      bee:
        default: drone
      task:
        default: Review energy pause
`
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	pub := &recordingPublisher{}
	d := runtime.NewDispatcher()
	reg := runtime.NewBeeRegistryFromBees(map[string]colony.Bee{"drone": {Role: "drone"}})
	d.SetBeeRegistry(reg)
	r := runtime.NewTestReactor(runtime.TestReactorOptions{
		ColonyRoot: root,
		Dispatcher: d,
		Registry:   reg,
		Ledger:     ledger,
		Publisher:  pub,
		Replayer:   fakeReplayer{},
	})
	ev := mustEvent(t, "trace-1", "runtime", protocol.EventSignal,
		`{"kind":"energy.paused","episodeId":"pause-1","remaining":0,"pendingCount":1}`)
	if err := r.ProcessEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	for _, published := range pub.events {
		if protocol.PayloadKind(published.Payload) == string(protocol.SignalSessionInvite) {
			t.Fatalf("energy observation produced an invite: %s", published.Payload)
		}
	}
}
