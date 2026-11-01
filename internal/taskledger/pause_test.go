package taskledger_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func exhaustedTrace() taskledger.TraceSnapshot {
	return taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    4,
		EnergyRemaining: 0,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
}

func deferDirect(t *testing.T, trace taskledger.TraceSnapshot, id, taskID, reason string) taskledger.TraceSnapshot {
	t.Helper()
	updated, added := taskledger.DeferPendingDispatch(trace, taskledger.PendingDispatch{
		ID:     id,
		Kind:   taskledger.PendingDispatchDirect,
		TaskID: taskID,
		Reason: reason,
	})
	if !added {
		t.Fatalf("deferred dispatch %q was not recorded", id)
	}
	return updated
}

// The first energy-caused deferral opens one episode in the same mutation that
// records the work; later deferrals keep that identity instead of opening a
// second pause.
func TestFirstEnergyDeferralOpensOnePauseEpisode(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	pause := trace.EnergyPause
	if pause == nil {
		t.Fatal("expected a pause episode")
	}
	if pause.EpisodeID == "" {
		t.Fatal("episode id is empty")
	}
	if pause.Published {
		t.Fatal("a fresh episode must be unpublished until the runtime claims it")
	}
	if pause.Reason != protocol.HoneyReserveExhaustedSummary {
		t.Fatalf("reason = %q", pause.Reason)
	}
	if pause.Remaining != 0 {
		t.Fatalf("remaining = %d, want the exhausted reserve", pause.Remaining)
	}
	if pause.Budget != 4 {
		t.Fatalf("budget = %d, want 4", pause.Budget)
	}
	if pause.Pending != 1 {
		t.Fatalf("pending = %d, want the deferral that opened it", pause.Pending)
	}
	if pause.StartedAt.IsZero() {
		t.Fatal("startedAt is empty")
	}

	trace = deferDirect(t, trace, "direct-b", "task-b", protocol.HoneyReserveExhaustedSummary)
	if trace.EnergyPause == nil || trace.EnergyPause.EpisodeID != pause.EpisodeID {
		t.Fatalf("episode = %+v, want the first episode kept", trace.EnergyPause)
	}
	if trace.EnergyPause.Pending != 1 {
		t.Fatalf("pending = %d, want the open-time count until a claim refreshes it", trace.EnergyPause.Pending)
	}
}

// A configuration-barrier hold is not energy waiting, so it neither opens a
// pause episode nor counts as one.
func TestConfigurationBarrierHoldDoesNotOpenPauseEpisode(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-held", "task-a", protocol.ConfigBarrierHoldReason)
	if trace.EnergyPause != nil {
		t.Fatalf("pause = %+v, want none for a configuration hold", trace.EnergyPause)
	}
}

// Deferring while the reserve still admits work is not a pause.
func TestDeferralWithReserveDoesNotOpenPauseEpisode(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    4,
		EnergyRemaining: 2,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	trace = deferDirect(t, trace, "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	if trace.EnergyPause != nil {
		t.Fatalf("pause = %+v, want none while the reserve can admit work", trace.EnergyPause)
	}
}

// Publication is a compare-and-set claim: the episode is returned exactly once,
// so replay or restart cannot send a second paused observation.
func TestClaimEnergyPausePublishedIsExactlyOnce(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	episodeID := trace.EnergyPause.EpisodeID

	trace, claimed, ok := taskledger.ClaimEnergyPausePublished(trace)
	if !ok {
		t.Fatal("expected the first claim to win")
	}
	if !claimed.Published {
		t.Fatal("the claimed episode must read as published")
	}
	if claimed.EpisodeID != episodeID {
		t.Fatalf("episode = %q, want %q", claimed.EpisodeID, episodeID)
	}
	if trace.EnergyPause == nil || !trace.EnergyPause.Published {
		t.Fatal("the persisted episode must record the publication")
	}

	if _, _, ok := taskledger.ClaimEnergyPausePublished(trace); ok {
		t.Fatal("a second claim must not win")
	}
}

// Closing clears the field and returns the episode once; a second close finds
// nothing, so a resumed observation has exactly one source.
func TestCloseEnergyPauseIsExactlyOnce(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	episodeID := trace.EnergyPause.EpisodeID

	trace, closed, ok := taskledger.CloseEnergyPause(trace)
	if !ok {
		t.Fatal("expected the first close to find the episode")
	}
	if closed.EpisodeID != episodeID {
		t.Fatalf("episode = %q, want %q", closed.EpisodeID, episodeID)
	}
	if trace.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the field cleared", trace.EnergyPause)
	}
	if _, _, ok := taskledger.CloseEnergyPause(trace); ok {
		t.Fatal("a second close must not find an episode")
	}
}

// The stipend reducer closes the episode in its own mutation and reports the
// abandoned count the resumed observation must carry.
func TestStipendReducerClosesPauseAndReportsAbandonedWork(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	trace = deferDirect(t, trace, "direct-b", "task-b", protocol.HoneyReserveExhaustedSummary)
	episodeID := trace.EnergyPause.EpisodeID

	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyPause != nil {
		t.Fatalf("pause = %+v, want the stipend to clear it", res.Trace.EnergyPause)
	}
	if res.PauseClose == nil {
		t.Fatal("expected the stipend reduction to report a pause close")
	}
	closeReport := res.PauseClose
	if closeReport.Episode.EpisodeID != episodeID {
		t.Fatalf("episode = %q, want %q", closeReport.Episode.EpisodeID, episodeID)
	}
	if closeReport.Source != protocol.EnergyResumeSourceStipend {
		t.Fatalf("source = %q, want energy.stipend", closeReport.Source)
	}
	if closeReport.PendingBefore != 2 || closeReport.PendingAfter != 0 {
		t.Fatalf("pending = %d -> %d, want 2 -> 0", closeReport.PendingBefore, closeReport.PendingAfter)
	}
	if closeReport.Abandoned != 2 {
		t.Fatalf("abandoned = %d, want both superseded entries", closeReport.Abandoned)
	}
	if closeReport.Remaining != 4 {
		t.Fatalf("remaining = %d, want the stipend amount", closeReport.Remaining)
	}
}

// energy.add raises the reserve but never closes the episode: the runtime closes
// it only after the drain, so a partial drain cannot pretend to have resumed.
func TestEnergyAddReducerKeepsPauseEpisodeOpen(t *testing.T) {
	trace := deferDirect(t, exhaustedTrace(), "direct-a", "task-a", protocol.HoneyReserveExhaustedSummary)
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyAddPayload{
		Kind:   protocol.SignalEnergyAdd,
		Amount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.Trace.EnergyPause == nil {
		t.Fatal("energy.add must leave the episode for the runtime to close after the drain")
	}
	if res.PauseClose != nil {
		t.Fatal("energy.add must not report a close")
	}
	if res.Trace.EnergyRemaining != 1 {
		t.Fatalf("remaining = %d, want 1", res.Trace.EnergyRemaining)
	}
}

// Observations are not ledger-owned energy events: receiving one changes no
// reserve, no pending queue, and no episode.
func TestEnergyObservationsDoNotChangeLedgerState(t *testing.T) {
	base := exhaustedTrace()
	base.EnergyRemaining = 2
	paused, err := protocol.NewEvent("trace-1", protocol.RuntimeAgentID, 1, protocol.EventSignal, protocol.EnergyPausedPayload{
		Kind:         protocol.SignalEnergyPaused,
		EpisodeID:    "pause-1",
		Remaining:    0,
		PendingCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := protocol.NewEvent("trace-1", protocol.RuntimeAgentID, 2, protocol.EventSignal, protocol.EnergyResumedPayload{
		Kind:       protocol.SignalEnergyResumed,
		EpisodeID:  "pause-1",
		Source:     protocol.EnergyResumeSourceAdd,
		Remaining:  3,
		Dispatched: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []protocol.Event{paused, resumed} {
		res, err := taskledger.ApplyEvent(base, ev)
		if err != nil {
			t.Fatal(err)
		}
		if res.Changed {
			t.Fatalf("%s changed the ledger", protocol.PayloadKind(ev.Payload))
		}
		if res.Trace.EnergyRemaining != 2 || res.Trace.EnergyBudget != 4 {
			t.Fatalf("%s changed the reserve: %+v", protocol.PayloadKind(ev.Payload), res.Trace)
		}
		if res.Trace.EnergyPause != nil {
			t.Fatalf("%s opened a pause episode", protocol.PayloadKind(ev.Payload))
		}
		if len(res.Trace.PendingDispatches) != 0 {
			t.Fatalf("%s recorded pending work: %+v", protocol.PayloadKind(ev.Payload), res.Trace.PendingDispatches)
		}
	}
}

// A stipend on a trail without an episode reports no close, so the runtime has
// nothing to announce.
func TestStipendWithoutPauseEpisodeReportsNoClose(t *testing.T) {
	trace := taskledger.TraceSnapshot{
		TraceID:         "trace-1",
		EnergyBudget:    4,
		EnergyRemaining: 1,
		Tasks:           map[string]taskledger.TaskSnapshot{},
	}
	ev, err := protocol.NewEvent("trace-1", "cli", 1, protocol.EventSignal, protocol.EnergyStipendPayload{
		Kind:   protocol.SignalEnergyStipend,
		Amount: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := taskledger.ApplyEvent(trace, ev)
	if err != nil {
		t.Fatal(err)
	}
	if res.PauseClose != nil {
		t.Fatalf("pause close = %+v, want none", res.PauseClose)
	}
	if res.Trace.EnergyRemaining != 4 {
		t.Fatalf("remaining = %d", res.Trace.EnergyRemaining)
	}
}
