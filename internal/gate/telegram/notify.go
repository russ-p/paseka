package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
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

// Run reconciles pending invites, then subscribes to live bus events until ctx ends.
func (n *Notifier) Run(ctx context.Context) error {
	log := logging.Component("gate.telegram.notify")
	if n.Config.Notify.InvitesEnabled() {
		if err := n.ReconcilePendingInvites(ctx); err != nil {
			log.Warn("invite reconcile failed", logging.F("error", err.Error()))
		}
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

func (n *Notifier) handleEvent(ev protocol.Event) error {
	if !n.Config.Notify.InvitesEnabled() {
		return nil
	}
	if ev.Type != protocol.EventSignal || protocol.PayloadKind(ev.Payload) != string(protocol.SignalSessionInvite) {
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

func (n *Notifier) pushInvite(ctx context.Context, invite colony.InviteEntry) error {
	if invite.Status != colony.InviteStatusPending {
		return nil
	}
	key := inviteNotifyKey(invite.InviteID, invite.Status)
	if !n.state.ShouldNotify(key) {
		return nil
	}
	text := FormatInviteCard(n.Colony, n.ledger, invite)
	keyboard := inviteActionKeyboard(invite.InviteID)
	for _, chatID := range n.Config.ChatIDs {
		msg := tgbotapi.NewMessage(chatID, text)
		msg.ReplyMarkup = keyboard
		if _, err := n.Bot.Send(msg); err != nil {
			return err
		}
	}
	return n.state.MarkNotified(key)
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
