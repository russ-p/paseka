package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func energyObservationEvent(t *testing.T, traceID string, payload any) protocol.Event {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return protocol.Event{TraceID: traceID, Type: protocol.EventSignal, Payload: raw}
}

func newEnergyNotifier(t *testing.T, slug string, cfg NotifyConfig, bot BotAPI) *Notifier {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	return &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}, Notify: cfg},
		Bot:    bot,
		state:  state,
	}
}

func TestNotifierPushEnergyPauseDedup(t *testing.T) {
	bot := &recordingBot{}
	n := newEnergyNotifier(t, "energy-pause", NotifyConfig{}, bot)

	payload := protocol.EnergyPausedPayload{
		Kind:         protocol.SignalEnergyPaused,
		EpisodeID:    "pause-1",
		Reason:       protocol.HoneyReserveExhaustedSummary,
		Remaining:    0,
		Budget:       12,
		PendingCount: 2,
	}
	ev := energyObservationEvent(t, "trace-1", payload)
	if err := n.handleEvent(ev); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d, want one pause card", len(bot.messages))
	}
	text := bot.messages[0]
	for _, want := range []string{"Energy paused", "Trace: trace-1", "Episode: pause-1", "honey: 0/12", "Pending: 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("pause card %q missing %q", text, want)
		}
	}
	if len(bot.keyboards) != 1 || len(bot.keyboards[0].InlineKeyboard) != 1 {
		t.Fatalf("keyboards = %+v, want one top-up row", bot.keyboards)
	}
	buttons := bot.keyboards[0].InlineKeyboard[0]
	if len(buttons) != 3 {
		t.Fatalf("top-up buttons = %d, want three", len(buttons))
	}
	for _, button := range buttons {
		if button.CallbackData == nil || !strings.HasPrefix(*button.CallbackData, callbackEnergyAdd+"trace-1:") {
			t.Fatalf("button %+v does not top up trace-1", button)
		}
	}
	if len(bot.disableNotification) != 1 || bot.disableNotification[0] {
		t.Fatal("energy default should be sound (DisableNotification=false)")
	}

	// A redelivered observation for the same episode must not repeat the card.
	if err := n.handleEvent(ev); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("dedup failed: messages = %d", len(bot.messages))
	}
}

func TestNotifierPushEnergyResume(t *testing.T) {
	bot := &recordingBot{}
	n := newEnergyNotifier(t, "energy-resume", NotifyConfig{}, bot)

	payload := protocol.EnergyResumedPayload{
		Kind:          protocol.SignalEnergyResumed,
		EpisodeID:     "pause-1",
		Source:        protocol.EnergyResumeSourceAdd,
		Remaining:     3,
		Budget:        12,
		PendingBefore: 2,
		PendingAfter:  0,
		Dispatched:    2,
	}
	if err := n.handleEvent(energyObservationEvent(t, "trace-1", payload)); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d, want one resume card", len(bot.messages))
	}
	text := bot.messages[0]
	for _, want := range []string{"Energy resumed", "Episode: pause-1", "Source: energy.add", "honey: 3/12", "Pending: 2 → 0", "Dispatched: 2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("resume card %q missing %q", text, want)
		}
	}
	if strings.Contains(text, "Abandoned:") {
		t.Fatalf("resume card %q should omit a zero abandoned count", text)
	}
	if len(bot.keyboards) != 1 || len(bot.keyboards[0].InlineKeyboard) != 0 {
		t.Fatalf("keyboards = %+v, want none on the closing card", bot.keyboards)
	}

	if err := n.handleEvent(energyObservationEvent(t, "trace-1", payload)); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("dedup failed: messages = %d", len(bot.messages))
	}
}

func TestEnergyResumeCardReportsStipendSupersession(t *testing.T) {
	text := FormatEnergyResumeCard("trace-1", protocol.EnergyResumedPayload{
		EpisodeID:     "pause-1",
		Source:        protocol.EnergyResumeSourceStipend,
		Remaining:     12,
		Budget:        12,
		PendingBefore: 3,
		PendingAfter:  0,
		Abandoned:     3,
	})
	for _, want := range []string{"Source: energy.stipend", "Pending: 3 → 0", "Abandoned: 3"} {
		if !strings.Contains(text, want) {
			t.Fatalf("resume card %q missing %q", text, want)
		}
	}
}

func TestEnergyCardsSurviveRestartWithoutDuplicates(t *testing.T) {
	bot := &recordingBot{}
	n := newEnergyNotifier(t, "energy-restart", NotifyConfig{}, bot)

	pause := energyObservationEvent(t, "trace-1", protocol.EnergyPausedPayload{
		Kind:         protocol.SignalEnergyPaused,
		EpisodeID:    "pause-1",
		Remaining:    0,
		Budget:       12,
		PendingCount: 1,
	})
	resume := energyObservationEvent(t, "trace-1", protocol.EnergyResumedPayload{
		Kind:          protocol.SignalEnergyResumed,
		EpisodeID:     "pause-1",
		Source:        protocol.EnergyResumeSourceAdd,
		Remaining:     3,
		Budget:        12,
		PendingBefore: 1,
		Dispatched:    1,
	})
	if err := n.handleEvent(pause); err != nil {
		t.Fatal(err)
	}
	if err := n.handleEvent(resume); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 2 {
		t.Fatalf("messages = %d, want a pause and a resume card", len(bot.messages))
	}

	// A restarted gate reloads the same dedup state from disk. Redelivered
	// observations for the episode must not produce a second pair of cards.
	restarted := &Notifier{
		Colony: colony.Context{Slug: "energy-restart"},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
	}
	state, err := LoadNotifyState("energy-restart")
	if err != nil {
		t.Fatal(err)
	}
	restarted.state = state
	if err := restarted.handleEvent(pause); err != nil {
		t.Fatal(err)
	}
	if err := restarted.handleEvent(resume); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 2 {
		t.Fatalf("restart duplicated cards: messages = %d", len(bot.messages))
	}
}

func TestBlockedTaskCardDoesNotSynthesizeEnergyAlert(t *testing.T) {
	bot := &recordingBot{}
	n := newEnergyNotifier(t, "energy-blocked", NotifyConfig{}, bot)

	// A legacy honey-blocked task still renders as a task card.
	task := taskledger.TaskSnapshot{
		TaskID:  "task-1",
		Title:   "Build feature",
		Bee:     "builder",
		Status:  protocol.TaskStatusBlocked,
		Summary: protocol.HoneyReserveExhaustedSummary,
	}
	if err := n.pushTaskStatus(context.Background(), "trace-1", task, protocol.TaskStatusBlocked); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d, want the legacy blocked card", len(bot.messages))
	}
	if !strings.Contains(bot.messages[0], "Task blocked") {
		t.Fatalf("legacy card = %q", bot.messages[0])
	}
	if !strings.Contains(bot.messages[0], "honey:") {
		t.Fatalf("legacy card missing honey line: %q", bot.messages[0])
	}
	if len(bot.keyboards) != 1 || len(bot.keyboards[0].InlineKeyboard) != 0 {
		t.Fatalf("keyboards = %+v, want no top-up buttons from a task status", bot.keyboards)
	}
}

func TestEnergyCategoryOffSuppressesCards(t *testing.T) {
	off := NotifyOff
	bot := &recordingBot{}
	n := newEnergyNotifier(t, "energy-off", NotifyConfig{Energy: &off}, bot)

	if err := n.handleEvent(energyObservationEvent(t, "trace-1", protocol.EnergyPausedPayload{
		Kind:         protocol.SignalEnergyPaused,
		EpisodeID:    "pause-1",
		Remaining:    0,
		PendingCount: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if err := n.handleEvent(energyObservationEvent(t, "trace-1", protocol.EnergyResumedPayload{
		Kind:       protocol.SignalEnergyResumed,
		EpisodeID:  "pause-1",
		Source:     protocol.EnergyResumeSourceAdd,
		Remaining:  3,
		Dispatched: 1,
	})); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 0 {
		t.Fatalf("energy: off should send nothing, got %d messages", len(bot.messages))
	}
}
