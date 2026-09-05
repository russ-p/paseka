package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/cues"
	"github.com/paseka/paseka/internal/tasks"
)

const maxSignalTitleLen = 200

// PendingSignal is a custom emit preview awaiting Confirm.
type PendingSignal struct {
	Command string
	Text    string
}

// PendingSignals stores in-flight custom signal previews keyed by short callback ids.
type PendingSignals struct {
	store map[string]PendingSignal
}

// NewPendingSignals creates an empty pending-signal store.
func NewPendingSignals() *PendingSignals {
	return &PendingSignals{
		store: make(map[string]PendingSignal),
	}
}

// Put stores a pending signal and returns a short id for callback data.
func (p *PendingSignals) Put(signal PendingSignal) (string, error) {
	id, err := randomPendingID()
	if err != nil {
		return "", err
	}
	if p.store == nil {
		p.store = make(map[string]PendingSignal)
	}
	p.store[id] = signal
	return id, nil
}

// Take removes and returns a pending signal by id.
func (p *PendingSignals) Take(id string) (PendingSignal, bool) {
	signal, ok := p.store[id]
	if ok {
		delete(p.store, id)
	}
	return signal, ok
}

// SignalActions handles custom emit:signal commands.
type SignalActions struct {
	Colony  colony.Context
	Config  Config
	Bot     BotAPI
	Pending *PendingSignals
}

func (a *SignalActions) pending() *PendingSignals {
	if a.Pending != nil {
		return a.Pending
	}
	return NewPendingSignals()
}

// HandleCommand shows a preview card for a configured custom command.
func (a *SignalActions) HandleCommand(ctx context.Context, chatID int64, command, text string) {
	text = strings.TrimSpace(text)
	cmd, ok := a.Config.Commands.CustomCommand(command)
	if !ok {
		return
	}
	if text == "" {
		a.sendText(chatID, 0, fmt.Sprintf("Usage: /%s <text>", command))
		return
	}
	pendingID, err := a.pending().Put(PendingSignal{
		Command: command,
		Text:    text,
	})
	if err != nil {
		a.sendText(chatID, 0, "signal preview failed: "+err.Error())
		return
	}
	body, err := a.formatPreview(cmd, text)
	if err != nil {
		a.sendText(chatID, 0, "signal preview failed: "+err.Error())
		return
	}
	keyboard := signalPreviewKeyboard(pendingID)
	msg := tgbotapi.NewMessage(chatID, body)
	msg.ReplyMarkup = keyboard
	_, _ = a.Bot.Send(msg)
}

func (a *SignalActions) formatPreview(cmd CustomCommandConfig, text string) (string, error) {
	if cmd.UsesCue() {
		cue, err := cues.Load(a.Colony.ColonyRoot, cmd.CueID())
		if err != nil {
			return "", err
		}
		return FormatCuePreview(cue, text), nil
	}
	return FormatSignalPreview(cmd, text), nil
}

// FormatCuePreview renders the cue-based confirm card body.
func FormatCuePreview(cue cues.Cue, text string) string {
	lines := []string{
		"Cue preview",
		fmt.Sprintf("Cue: %s", cue.ID),
	}
	if desc := strings.TrimSpace(cue.Description); desc != "" {
		lines = append(lines, fmt.Sprintf("Description: %s", desc))
	}
	switch cue.Emit {
	case cues.EmitSignal:
		lines = append(lines,
			fmt.Sprintf("Type: %s", cue.SignalType),
			fmt.Sprintf("Kind: %s", cue.SignalKind),
		)
	case cues.EmitTask:
		lines = append(lines,
			fmt.Sprintf("Bee: %s", cue.Bee),
			fmt.Sprintf("Intent: %s", cue.Intent),
		)
		if review := strings.TrimSpace(cue.Review); review != "" {
			lines = append(lines, fmt.Sprintf("Review: %s", review))
		}
		autorunLabel := "no"
		if cue.Autorun {
			autorunLabel = "yes"
		}
		lines = append(lines, fmt.Sprintf("Autorun: %s", autorunLabel))
	}
	if cue.IsStanding() {
		lines = append(lines, fmt.Sprintf("Standing: %s", cue.StandingTrace))
	}
	confirm := "Confirm to publish on a new trace."
	if cue.IsStanding() {
		confirm = fmt.Sprintf("Confirm to publish on standing trail %s.", cue.StandingTrace)
	}
	lines = append(lines,
		fmt.Sprintf("Text: %s", truncateText(text, maxTaskPreviewLen)),
		"",
		confirm,
	)
	return strings.Join(lines, "\n")
}

// FormatSignalPreview renders the custom signal confirm card body.
func FormatSignalPreview(cmd CustomCommandConfig, text string) string {
	lines := []string{
		"Signal preview",
		fmt.Sprintf("Type: %s", strings.ToUpper(strings.TrimSpace(cmd.Type))),
		fmt.Sprintf("Kind: %s", strings.TrimSpace(cmd.Kind)),
		fmt.Sprintf("Text: %s", truncateText(text, maxTaskPreviewLen)),
		"",
		"Confirm to publish on a new trace.",
	}
	return strings.Join(lines, "\n")
}

func signalPreviewKeyboard(pendingID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Confirm", callbackSignalConfirm+pendingID),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", callbackSignalCancel+pendingID),
		),
	)
}

// Confirm publishes the configured SIGNAL or cue for a pending preview.
func (a *SignalActions) Confirm(ctx context.Context, chatID int64, messageID int, pendingID string) {
	pending, ok := a.pending().Take(pendingID)
	if !ok {
		a.sendText(chatID, messageID, "Signal preview expired. Send the command again.")
		return
	}
	cmd, ok := a.Config.Commands.CustomCommand(pending.Command)
	if !ok {
		a.sendText(chatID, messageID, "signal publish failed: command config missing")
		return
	}
	if cmd.UsesCue() {
		a.confirmCue(ctx, chatID, messageID, cmd, pending.Text)
		return
	}
	traceID, err := colony.NewTraceID()
	if err != nil {
		a.sendText(chatID, messageID, "signal publish failed: "+err.Error())
		return
	}
	payloadJSON, err := BuildCustomSignalPayload(cmd, pending.Text)
	if err != nil {
		a.sendText(chatID, messageID, "signal publish failed: "+err.Error())
		return
	}
	client, err := bus.ConnectColony(a.Colony, false)
	if err != nil {
		a.sendText(chatID, messageID, "signal publish failed: "+err.Error())
		return
	}
	if client == nil {
		a.sendText(chatID, messageID, "signal publish failed: nats url not configured")
		return
	}
	defer client.Close()

	ev, err := bus.NewEventFromCLI(traceID, "telegram", "SIGNAL", string(payloadJSON))
	if err != nil {
		a.sendText(chatID, messageID, "signal publish failed: "+err.Error())
		return
	}
	if err := client.PublishEvent(ctx, ev); err != nil {
		a.sendText(chatID, messageID, "signal publish failed: "+err.Error())
		return
	}
	msg := fmt.Sprintf("Published SIGNAL/%s\nTrace: %s", strings.TrimSpace(cmd.Kind), traceID)
	a.sendText(chatID, messageID, msg)
}

func (a *SignalActions) confirmCue(ctx context.Context, chatID int64, messageID int, cmd CustomCommandConfig, text string) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, messageID, "cue publish failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Publisher == nil {
		a.sendText(chatID, messageID, "cue publish failed: nats url not configured")
		return
	}
	res, err := cues.Run(ctx, session.Publisher, session.Ledger, cues.RunInput{
		ColonyRoot: a.Colony.ColonyRoot,
		CueID:      cmd.CueID(),
		Text:       text,
		Source:     "telegram",
		AgentID:    "telegram",
	})
	if err != nil {
		a.sendText(chatID, messageID, "cue publish failed: "+err.Error())
		return
	}
	msg := fmt.Sprintf("Published %s/%s\nTrace: %s", res.EventType, res.Kind, res.TraceID)
	if res.TaskID != "" {
		msg += fmt.Sprintf("\nTask: %s", res.TaskID)
	}
	a.sendText(chatID, messageID, msg)
}

// Cancel discards a pending signal preview.
func (a *SignalActions) Cancel(chatID int64, messageID int, pendingID string) {
	a.pending().Take(pendingID)
	a.sendText(chatID, messageID, "Signal cancelled.")
}

// BuildCustomSignalPayload builds the JSON payload for a custom emit:signal command.
func BuildCustomSignalPayload(cmd CustomCommandConfig, text string) (json.RawMessage, error) {
	text = strings.TrimSpace(text)
	title := signalTitleFromText(text)
	payload := map[string]string{
		"kind":   strings.TrimSpace(cmd.Kind),
		"title":  title,
		"body":   text,
		"source": "telegram",
	}
	for k, v := range cmd.Static {
		if strings.TrimSpace(k) == "" {
			continue
		}
		payload[k] = v
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func signalTitleFromText(text string) string {
	line := text
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		line = text[:idx]
	}
	return truncateText(strings.TrimSpace(line), maxSignalTitleLen)
}

func (a *SignalActions) sendText(chatID int64, editMessageID int, text string) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = a.Bot.Send(msg)
}
