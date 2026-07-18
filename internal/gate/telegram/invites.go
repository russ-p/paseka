package telegram

import (
	"context"
	"fmt"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/invites"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/sessions"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

const maxInviteTaskLen = 200

// InviteActions handles invite list/decision commands for the Telegram gate.
type InviteActions struct {
	Colony   colony.Context
	Config   Config
	Bot      BotAPI
	Sessions *sessions.Manager
}

func (a *InviteActions) sessions() *sessions.Manager {
	if a.Sessions != nil {
		return a.Sessions
	}
	return sessions.DefaultManager
}

// SendInvitesList posts one card per pending invite.
func (a *InviteActions) SendInvitesList(chatID int64) {
	list, err := console.ListInvites(a.Colony, colony.InviteStatusPending)
	if err != nil {
		a.sendText(chatID, 0, "invites unavailable: "+err.Error())
		return
	}
	if len(list) == 0 {
		a.sendText(chatID, 0, "No pending invites.")
		return
	}
	ledger, closeLedger := a.ledgerSession()
	defer closeLedger()
	for _, view := range list {
		entry := colony.InviteEntry{
			InviteID:    view.InviteID,
			TraceID:     view.TraceID,
			Bee:         view.Bee,
			Intent:      view.Intent,
			Task:        view.Task,
			Status:      view.Status,
			ArtifactRef: view.ArtifactRef,
		}
		a.sendInviteCard(chatID, 0, entry, ledger)
	}
}

func (a *InviteActions) sendInviteCard(chatID int64, editMessageID int, invite colony.InviteEntry, ledger taskledger.Ledger) {
	text := FormatInviteCard(a.Colony, ledger, invite)
	keyboard := inviteActionKeyboard(invite.InviteID)
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		edit.ReplyMarkup = &keyboard
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	_, _ = a.Bot.Send(msg)
}

// FormatInviteCard renders a pending invite push/list card.
func FormatInviteCard(ctx colony.Context, ledger taskledger.Ledger, invite colony.InviteEntry) string {
	task := truncateText(invite.Task, maxInviteTaskLen)
	lines := []string{
		"Session invite",
		fmt.Sprintf("Invite: %s", invite.InviteID),
		fmt.Sprintf("Trace: %s", invite.TraceID),
		fmt.Sprintf("Bee: %s", invite.Bee),
	}
	if invite.Intent != "" {
		lines = append(lines, fmt.Sprintf("Intent: %s", invite.Intent))
	}
	lines = append(lines,
		fmt.Sprintf("Task: %s", task),
		honeyLine(ctx, ledger, invite.TraceID),
	)
	return strings.Join(lines, "\n")
}

func honeyLine(ctx colony.Context, ledger taskledger.Ledger, traceID string) string {
	remaining, budget := traceHoney(ctx, ledger, traceID)
	return fmt.Sprintf("honey: %d/%d", remaining, budget)
}

func traceHoney(ctx colony.Context, ledger taskledger.Ledger, traceID string) (remaining, budget int) {
	budget = protocol.DefaultEnergyBudget
	if ctx.ColonyRoot != "" {
		if manifest, err := colony.LoadColony(ctx.ColonyRoot); err == nil {
			budget = manifest.ResolvedEnergyBudget()
		}
	}
	if ledger == nil {
		return 0, budget
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return 0, budget
	}
	remaining = snap.EnergyRemaining
	if snap.EnergyBudget > 0 {
		budget = snap.EnergyBudget
	}
	return remaining, budget
}

func inviteActionKeyboard(inviteID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Accept", callbackInviteAccept+inviteID),
			tgbotapi.NewInlineKeyboardButtonData("Reject", callbackInviteReject+inviteID),
			tgbotapi.NewInlineKeyboardButtonData("Defer", callbackInviteDefer+inviteID),
		),
	)
}

func inviteConfirmKeyboard(inviteID, action string) tgbotapi.InlineKeyboardMarkup {
	var confirmData string
	switch action {
	case "accept":
		confirmData = callbackInviteConfirmAccept + inviteID
	case "reject":
		confirmData = callbackInviteConfirmReject + inviteID
	default:
		confirmData = callbackInviteCancel + inviteID
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Confirm", confirmData),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", callbackInviteCancel+inviteID),
		),
	)
}

func energyTopUpKeyboard(traceID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("+1", fmt.Sprintf("%s%s:1", callbackEnergyAdd, traceID)),
			tgbotapi.NewInlineKeyboardButtonData("+5", fmt.Sprintf("%s%s:5", callbackEnergyAdd, traceID)),
			tgbotapi.NewInlineKeyboardButtonData("+12", fmt.Sprintf("%s%s:12", callbackEnergyAdd, traceID)),
		),
	)
}

func (a *InviteActions) showInviteConfirm(chatID int64, messageID int, inviteID, action string) {
	label := "accept"
	if action == "reject" {
		label = "reject"
	}
	text := fmt.Sprintf("Confirm %s invite %s?", label, inviteID)
	keyboard := inviteConfirmKeyboard(inviteID, action)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ReplyMarkup = &keyboard
	_, _ = a.Bot.Send(edit)
}

func (a *InviteActions) cancelInviteConfirm(chatID int64, messageID int, inviteID string) {
	invite, err := colony.FindInvite(a.Colony.Slug, inviteID)
	if err != nil {
		a.sendText(chatID, messageID, "Invite not found.")
		return
	}
	if invite.Status != colony.InviteStatusPending {
		a.sendText(chatID, messageID, fmt.Sprintf("Invite %s is %s.", inviteID, invite.Status))
		return
	}
	ledger, closeLedger := a.ledgerSession()
	defer closeLedger()
	a.sendInviteCard(chatID, messageID, invite, ledger)
}

func (a *InviteActions) executeDefer(ctx context.Context, chatID int64, messageID int, inviteID string) {
	svc, closeFn, err := a.inviteService()
	if err != nil {
		a.sendText(chatID, messageID, "defer failed: "+err.Error())
		return
	}
	defer closeFn()

	invite, err := svc.Reject(ctx, inviteID, true)
	if err != nil {
		a.sendText(chatID, messageID, "defer failed: "+err.Error())
		return
	}
	a.sendText(chatID, messageID, fmt.Sprintf("Invite %s deferred.", invite.InviteID))
}

func (a *InviteActions) executeAccept(ctx context.Context, chatID int64, messageID int, inviteID string) {
	svc, closeFn, err := a.inviteService()
	if err != nil {
		a.sendText(chatID, messageID, "accept failed: "+err.Error())
		return
	}
	defer closeFn()

	res, err := svc.Accept(ctx, inviteID, false)
	if err != nil {
		if isHoneyExhausted(err) {
			invite, findErr := colony.FindInvite(a.Colony.Slug, inviteID)
			if findErr != nil {
				a.sendText(chatID, messageID, "accept failed: "+err.Error())
				return
			}
			text := fmt.Sprintf("Insufficient honey to accept invite %s.\n%s", inviteID, err.Error())
			a.sendTextWithKeyboard(chatID, messageID, text, energyTopUpKeyboard(invite.TraceID))
			return
		}
		a.sendText(chatID, messageID, "accept failed: "+err.Error())
		return
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "gate host"
	}
	a.sendText(chatID, messageID, formatAcceptSuccess(a.Config, res, hostname))
}

func (a *InviteActions) executeReject(ctx context.Context, chatID int64, messageID int, inviteID string) {
	svc, closeFn, err := a.inviteService()
	if err != nil {
		a.sendText(chatID, messageID, "reject failed: "+err.Error())
		return
	}
	defer closeFn()

	invite, err := svc.Reject(ctx, inviteID, false)
	if err != nil {
		a.sendText(chatID, messageID, "reject failed: "+err.Error())
		return
	}
	a.sendText(chatID, messageID, fmt.Sprintf("Invite %s rejected (%s).", invite.InviteID, invite.Status))
}

func (a *InviteActions) addEnergy(ctx context.Context, chatID int64, messageID int, traceID string, amount int) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, messageID, "energy add failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Client == nil || session.Ledger == nil {
		a.sendText(chatID, messageID, "energy add failed: nats url not configured")
		return
	}
	snap, err := tasks.AddEnergy(ctx, session, tasks.AddEnergyInput{
		TraceID: traceID,
		Amount:  amount,
		AgentID: "telegram",
	})
	if err != nil {
		a.sendText(chatID, messageID, "energy add failed: "+err.Error())
		return
	}
	text := fmt.Sprintf("Added %d honey to %s\nhoney: %d/%d", amount, traceID, snap.EnergyRemaining, snap.EnergyBudget)
	a.sendText(chatID, messageID, text)
}

func formatAcceptSuccess(cfg Config, res *invites.AcceptResult, hostname string) string {
	lines := []string{
		"Session started",
		fmt.Sprintf("Invite %s accepted", res.Invite.InviteID),
		fmt.Sprintf("Session: %s", res.SessionID),
		fmt.Sprintf("PTY is on this machine (%s) — not in Telegram chat.", hostname),
		fmt.Sprintf("Attach: paseka session attach %s", res.SessionID),
	}
	if url := strings.TrimSpace(cfg.ConsoleBaseURL); url != "" {
		lines = append(lines, fmt.Sprintf("Console: %s", strings.TrimRight(url, "/")))
	}
	return strings.Join(lines, "\n")
}

func (a *InviteActions) inviteService() (*invites.Service, func(), error) {
	client, err := bus.ConnectColony(a.Colony, false)
	if err != nil {
		return nil, nil, err
	}
	if client == nil {
		return nil, nil, fmt.Errorf("nats url not configured")
	}
	svc := &invites.Service{
		Colony:   a.Colony,
		Bus:      client,
		Sessions: a.sessions(),
	}
	return svc, func() { client.Close() }, nil
}

func (a *InviteActions) ledgerSession() (taskledger.Ledger, func()) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil || session == nil || session.Ledger == nil {
		return nil, func() {}
	}
	return session.Ledger, session.Close
}

func (a *InviteActions) sendText(chatID int64, editMessageID int, text string) {
	a.sendTextWithKeyboard(chatID, editMessageID, text, tgbotapi.InlineKeyboardMarkup{})
}

func (a *InviteActions) sendTextWithKeyboard(chatID int64, editMessageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		if len(keyboard.InlineKeyboard) > 0 {
			edit.ReplyMarkup = &keyboard
		}
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if len(keyboard.InlineKeyboard) > 0 {
		msg.ReplyMarkup = keyboard
	}
	_, _ = a.Bot.Send(msg)
}

func isHoneyExhausted(err error) bool {
	return err != nil && strings.Contains(err.Error(), "honey reserve exhausted")
}

func truncateText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
