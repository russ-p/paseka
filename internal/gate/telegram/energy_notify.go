package telegram

import (
	"encoding/json"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// handleEnergyObservationEvent turns one energy.paused / energy.resumed
// observation into a single card per episode. The observation is the trail's
// admission truth: no task status is read, and nothing is synthesized from a
// legacy task.blocked.
func (n *Notifier) handleEnergyObservationEvent(ev protocol.Event) error {
	kind := protocol.PayloadKind(ev.Payload)
	switch protocol.EnergyEventKind(kind) {
	case protocol.SignalEnergyPaused:
		var payload protocol.EnergyPausedPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return fmt.Errorf("telegram gate: parse energy.paused: %w", err)
		}
		return n.pushEnergyPause(ev.TraceID, payload)
	case protocol.SignalEnergyResumed:
		var payload protocol.EnergyResumedPayload
		if err := json.Unmarshal(ev.Payload, &payload); err != nil {
			return fmt.Errorf("telegram gate: parse energy.resumed: %w", err)
		}
		return n.pushEnergyResume(ev.TraceID, payload)
	default:
		return nil
	}
}

// pushEnergyPause sends the one pause card of an episode, with the same top-up
// buttons the gate offers elsewhere. An episode is deduped by identity, so a
// redelivery or a gate restart does not repeat the card.
func (n *Notifier) pushEnergyPause(traceID string, payload protocol.EnergyPausedPayload) error {
	episodeID := strings.TrimSpace(payload.EpisodeID)
	if episodeID == "" {
		return nil
	}
	mode := n.Config.Notify.Mode(NotifyCategoryEnergy)
	if !mode.Enabled() {
		return nil
	}
	key := energyPauseNotifyKey(traceID, episodeID)
	if !n.state.ShouldNotify(key) {
		return nil
	}
	if _, ok := n.trailScope(traceID); !ok {
		return nil
	}
	text := FormatEnergyPauseCard(traceID, payload)
	return n.broadcast(text, energyTopUpKeyboard(traceID), key, mode.Silent())
}

// pushEnergyResume sends the closing card of an episode: the increase that ended
// the pause, plus the counts that say whether deferred work ran or was
// superseded by a new allocation.
func (n *Notifier) pushEnergyResume(traceID string, payload protocol.EnergyResumedPayload) error {
	episodeID := strings.TrimSpace(payload.EpisodeID)
	if episodeID == "" {
		return nil
	}
	mode := n.Config.Notify.Mode(NotifyCategoryEnergy)
	if !mode.Enabled() {
		return nil
	}
	key := energyResumeNotifyKey(traceID, episodeID)
	if !n.state.ShouldNotify(key) {
		return nil
	}
	if _, ok := n.trailScope(traceID); !ok {
		return nil
	}
	text := FormatEnergyResumeCard(traceID, payload)
	return n.broadcast(text, tgbotapi.InlineKeyboardMarkup{}, key, mode.Silent())
}

// FormatEnergyPauseCard renders the card an energy.paused observation produces.
func FormatEnergyPauseCard(traceID string, payload protocol.EnergyPausedPayload) string {
	lines := []string{
		"Energy paused",
		fmt.Sprintf("Trace: %s", traceID),
		fmt.Sprintf("Episode: %s", payload.EpisodeID),
	}
	if reason := strings.TrimSpace(payload.Reason); reason != "" {
		lines = append(lines, fmt.Sprintf("Reason: %s", reason))
	}
	lines = append(lines,
		"honey: "+taskledger.FormatHoneyCompact(payload.Remaining, payload.Budget, 0),
		fmt.Sprintf("Pending: %d", payload.PendingCount),
	)
	// The pause is a trail-wide condition, so the card names the action that
	// changes it rather than pointing at one task.
	lines = append(lines, "Top up to admit deferred work.")
	return strings.Join(lines, "\n")
}

// FormatEnergyResumeCard renders the card an energy.resumed observation produces.
// A stipend that superseded the previous allocation says so through its source
// and abandoned count rather than pretending the old work ran.
func FormatEnergyResumeCard(traceID string, payload protocol.EnergyResumedPayload) string {
	lines := []string{
		"Energy resumed",
		fmt.Sprintf("Trace: %s", traceID),
		fmt.Sprintf("Episode: %s", payload.EpisodeID),
		fmt.Sprintf("Source: %s", payload.Source),
		"honey: " + taskledger.FormatHoneyCompact(payload.Remaining, payload.Budget, 0),
		fmt.Sprintf("Pending: %d → %d", payload.PendingBefore, payload.PendingAfter),
		fmt.Sprintf("Dispatched: %d", payload.Dispatched),
	}
	if payload.Abandoned > 0 {
		lines = append(lines, fmt.Sprintf("Abandoned: %d", payload.Abandoned))
	}
	return strings.Join(lines, "\n")
}
