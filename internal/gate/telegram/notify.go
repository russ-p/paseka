package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

// Notifier pushes invite notifications from the bus and startup reconcile with dedup.
type Notifier struct {
	Colony colony.Context
	Config Config
	Bot    BotAPI
	state  *NotifyState
	ledger taskledger.Ledger
	bus    *bus.Client
}

// NewNotifier prepares notify dedup state and NATS for the gate process.
func NewNotifier(ctx colony.Context, cfg Config, bot BotAPI) (*Notifier, error) {
	state, err := LoadNotifyState(ctx.Slug)
	if err != nil {
		return nil, err
	}
	client, err := bus.ConnectColony(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("telegram gate: nats: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("telegram gate: nats url not configured in home config")
	}
	var ledger taskledger.Ledger
	if kv, err := client.JetStream().KeyValue(bus.TaskLedgerBucket(ctx.Slug)); err == nil {
		ledger = taskledger.NewKVLedger(kv)
	}
	return &Notifier{
		Colony: ctx,
		Config: cfg,
		Bot:    bot,
		state:  state,
		ledger: ledger,
		bus:    client,
	}, nil
}

// Close releases the NATS connection.
func (n *Notifier) Close() {
	if n.bus != nil {
		n.bus.Close()
		n.bus = nil
	}
}

// Run reconciles pending state, then subscribes to live bus events until ctx ends.
func (n *Notifier) Run(ctx context.Context) error {
	log := logging.Component("gate.telegram.notify")
	if n.Config.Notify.Mode(NotifyCategoryInvites).Enabled() {
		if err := n.ReconcilePendingInvites(ctx); err != nil {
			log.Warn("invite reconcile failed", logging.F("error", err.Error()))
		}
	}
	if err := n.ReconcileTaskStatuses(ctx); err != nil {
		log.Warn("task reconcile failed", logging.F("error", err.Error()))
	}

	durable := GateConsumerName(n.Colony.Slug)
	sub, err := n.bus.SubscribeEvents(durable, n.handleEvent)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	log.Info("bus subscription active", logging.F("durable", durable))
	<-ctx.Done()
	return ctx.Err()
}

// ReconcilePendingInvites pushes cards for pending invites not yet deduped.
func (n *Notifier) ReconcilePendingInvites(ctx context.Context) error {
	entries, err := colony.ListInvites(n.Colony.Slug, colony.InviteStatusPending, "")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := n.pushInvite(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileTaskStatuses pushes cards for blocked/failed/review-gated tasks not yet deduped.
// Completed tasks are live-only (not reconciled on gate restart).
func (n *Notifier) ReconcileTaskStatuses(ctx context.Context) error {
	board, err := console.ListTaskBoard(n.Colony)
	if err != nil {
		return err
	}
	for _, group := range board.Groups {
		status := protocol.TaskStatus(group.Status)
		for _, item := range group.Tasks {
			task := n.lookupTask(item.TraceID, item.TaskID)
			task.Status = status
			cat, ok := classifyTaskStatus(task, status)
			if !ok || !n.Config.Notify.Mode(cat).Enabled() {
				continue
			}
			if err := n.pushTaskStatus(ctx, item.TraceID, task, status); err != nil {
				return err
			}
		}
	}
	return nil
}

func (n *Notifier) handleEvent(ev protocol.Event) error {
	kind := protocol.PayloadKind(ev.Payload)
	switch {
	case ev.Type == protocol.EventSignal && kind == string(protocol.SignalSessionInvite):
		return n.handleInviteEvent(ev)
	case ev.Type == protocol.EventSignal && kind == string(protocol.TaskEventStatus):
		return n.handleTaskStatusEvent(ev)
	case ev.Type == protocol.EventVerification && kind == string(protocol.TaskEventCompleted):
		return n.handleTaskCompletedEvent(ev)
	default:
		return nil
	}
}

func (n *Notifier) handleInviteEvent(ev protocol.Event) error {
	if !n.Config.Notify.Mode(NotifyCategoryInvites).Enabled() {
		return nil
	}
	var payload protocol.SessionInvitePayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("telegram gate: parse session.invite: %w", err)
	}
	if payload.Status != protocol.InviteStatusPending {
		return nil
	}
	svc := &invites.Service{Colony: n.Colony}
	if err := svc.ProjectEvent(ev); err != nil {
		return err
	}
	entry := colony.InviteEntry{
		InviteID:    payload.InviteID,
		TraceID:     ev.TraceID,
		Bee:         payload.Bee,
		Intent:      payload.Intent,
		Task:        payload.Task,
		Status:      string(payload.Status),
		ArtifactRef: payload.ArtifactRef,
	}
	return n.pushInvite(context.Background(), entry)
}

func (n *Notifier) handleTaskStatusEvent(ev protocol.Event) error {
	var payload protocol.TaskStatusPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("telegram gate: parse task.status: %w", err)
	}
	task := n.lookupTask(ev.TraceID, payload.TaskID)
	task.Status = payload.Status
	if payload.Summary != "" {
		task.Summary = payload.Summary
	}
	cat, ok := classifyTaskStatus(task, payload.Status)
	if !ok || !n.Config.Notify.Mode(cat).Enabled() {
		return nil
	}
	return n.pushTaskStatus(context.Background(), ev.TraceID, task, payload.Status)
}

func (n *Notifier) handleTaskCompletedEvent(ev protocol.Event) error {
	mode := n.Config.Notify.Mode(NotifyCategoryCompleted)
	if !mode.Enabled() {
		return nil
	}
	var payload protocol.TaskCompletedPayload
	if err := json.Unmarshal(ev.Payload, &payload); err != nil {
		return fmt.Errorf("telegram gate: parse task.completed: %w", err)
	}
	if payload.TaskID == "" {
		return nil
	}
	task := n.lookupTask(ev.TraceID, payload.TaskID)
	task.Status = protocol.TaskStatusCompleted
	if payload.Summary != "" {
		task.Summary = payload.Summary
	}
	return n.pushTaskCompleted(context.Background(), ev.TraceID, task, payload, mode)
}

func (n *Notifier) lookupTask(traceID, taskID string) taskledger.TaskSnapshot {
	task := taskledger.TaskSnapshot{TaskID: taskID}
	if n.ledger == nil {
		return task
	}
	snap, err := n.ledger.Snapshot(traceID)
	if err != nil {
		return task
	}
	if t, ok := snap.Tasks[taskID]; ok {
		return t
	}
	return task
}

func (n *Notifier) pushInvite(ctx context.Context, invite colony.InviteEntry) error {
	if invite.Status != colony.InviteStatusPending {
		return nil
	}
	mode := n.Config.Notify.Mode(NotifyCategoryInvites)
	if !mode.Enabled() {
		return nil
	}
	key := inviteNotifyKey(invite.InviteID, invite.Status)
	if !n.state.ShouldNotify(key) {
		return nil
	}
	text := FormatInviteCard(n.Colony, n.ledger, invite)
	keyboard := inviteActionKeyboard(invite.InviteID)
	return n.broadcast(text, keyboard, key, mode.Silent())
}

func (n *Notifier) pushTaskStatus(ctx context.Context, traceID string, task taskledger.TaskSnapshot, status protocol.TaskStatus) error {
	cat, ok := classifyTaskStatus(task, status)
	if !ok {
		return nil
	}
	mode := n.Config.Notify.Mode(cat)
	if !mode.Enabled() {
		return nil
	}
	key := taskNotifyKey(traceID, task.TaskID, string(status))
	if !n.state.ShouldNotify(key) {
		return nil
	}
	text := FormatTaskStatusCard(n.Colony, n.Config, n.ledger, traceID, task, status)
	var keyboard tgbotapi.InlineKeyboardMarkup
	switch {
	case status == protocol.TaskStatusWaitingReview && taskledger.IsReviewGate(task):
		keyboard = proposalKeyboardForTask(traceID, task)
	case status == protocol.TaskStatusBlocked && taskledger.IsEnergyBlockedTask(task):
		keyboard = energyTopUpKeyboard(traceID)
	}
	return n.broadcast(text, keyboard, key, mode.Silent())
}

func (n *Notifier) pushTaskCompleted(ctx context.Context, traceID string, task taskledger.TaskSnapshot, payload protocol.TaskCompletedPayload, mode NotifyMode) error {
	key := taskCompletedNotifyKey(traceID, task.TaskID)
	if !n.state.ShouldNotify(key) {
		return nil
	}
	text := FormatTaskCompletedCard(traceID, task, payload)
	return n.broadcast(text, tgbotapi.InlineKeyboardMarkup{}, key, mode.Silent())
}

func (n *Notifier) broadcast(text string, keyboard tgbotapi.InlineKeyboardMarkup, dedupKey string, silent bool) error {
	for _, chatID := range n.Config.ChatIDs {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.DisableNotification = silent
		if len(keyboard.InlineKeyboard) > 0 {
			msg.ReplyMarkup = keyboard
		}
		if _, err := n.Bot.Send(msg); err != nil {
			return err
		}
	}
	return n.state.MarkNotified(dedupKey)
}

// FormatTaskStatusCard renders a blocked/failed/waiting_review push card.
func FormatTaskStatusCard(ctx colony.Context, cfg Config, ledger taskledger.Ledger, traceID string, task taskledger.TaskSnapshot, status protocol.TaskStatus) string {
	title := taskTitle(task)
	lines := []string{
		fmt.Sprintf("Task %s", status),
		fmt.Sprintf("Trace: %s", traceID),
		fmt.Sprintf("Task: %s", task.TaskID),
	}
	if task.Bee != "" {
		lines = append(lines, fmt.Sprintf("Bee: %s", task.Bee))
	}
	lines = append(lines, fmt.Sprintf("Title: %s", truncateText(title, maxInviteTaskLen)))
	if summary := strings.TrimSpace(task.Summary); summary != "" && summary != title {
		lines = append(lines, fmt.Sprintf("Summary: %s", truncateText(summary, maxInviteTaskLen)))
	}
	if status == protocol.TaskStatusWaitingReview && taskledger.IsReviewGate(task) {
		lines = append(lines, proposalReviewLines(cfg, task)...)
	}
	if status == protocol.TaskStatusBlocked || taskledger.IsEnergyBlockedTask(task) {
		lines = append(lines, honeyLine(ctx, ledger, traceID))
	}
	return strings.Join(lines, "\n")
}

// FormatTaskCompletedCard renders a task.completed push card.
func FormatTaskCompletedCard(traceID string, task taskledger.TaskSnapshot, payload protocol.TaskCompletedPayload) string {
	title := taskTitle(task)
	lines := []string{
		"Task completed",
		fmt.Sprintf("Trace: %s", traceID),
		fmt.Sprintf("Task: %s", task.TaskID),
	}
	if task.Bee != "" {
		lines = append(lines, fmt.Sprintf("Bee: %s", task.Bee))
	}
	lines = append(lines, fmt.Sprintf("Title: %s", truncateText(title, maxInviteTaskLen)))
	summary := strings.TrimSpace(payload.Summary)
	if summary == "" {
		summary = strings.TrimSpace(task.Summary)
	}
	if summary != "" && summary != title {
		lines = append(lines, fmt.Sprintf("Summary: %s", truncateText(summary, maxInviteTaskLen)))
	}
	if commit := strings.TrimSpace(payload.Commit); commit != "" {
		lines = append(lines, fmt.Sprintf("Commit: %s", truncateText(commit, maxInviteTaskLen)))
	}
	return strings.Join(lines, "\n")
}

func taskTitle(task taskledger.TaskSnapshot) string {
	if t := strings.TrimSpace(task.Title); t != "" {
		return t
	}
	return task.TaskID
}

func GateConsumerName(slug string) string {
	return "paseka-gate-telegram-" + slugSanitize(slug)
}

func slugSanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('_')
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "colony"
	}
	return out
}
