package telegram

import (
	"context"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
)

// BotAPI is the subset of Telegram Bot API used by the gate (mockable in tests).
type BotAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// Handler dispatches allowlisted Telegram updates to gate commands.
type Handler struct {
	Colony     colony.Context
	Config     Config
	Supervisor *runtime.Supervisor
	Bot        BotAPI
	Invites    *InviteActions
}

// HandleUpdate processes one Telegram update. Non-allowlisted traffic is silently ignored.
func (h *Handler) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		h.handleCallback(ctx, update.CallbackQuery)
		return
	}
	if update.Message == nil || !update.Message.IsCommand() {
		return
	}
	userID := int64(0)
	if update.Message.From != nil {
		userID = update.Message.From.ID
	}
	chatID := update.Message.Chat.ID
	if !Allowed(h.Config, userID, chatID) {
		return
	}

	switch update.Message.Command() {
	case "start", "status":
		h.sendStatus(chatID, 0)
	case "help":
		h.sendHelp(chatID)
	case "invites":
		h.inviteActions().SendInvitesList(chatID)
	}
}

func (h *Handler) handleCallback(ctx context.Context, q *tgbotapi.CallbackQuery) {
	if q == nil || q.Message == nil {
		return
	}
	userID := int64(0)
	if q.From != nil {
		userID = q.From.ID
	}
	chatID := q.Message.Chat.ID
	messageID := q.Message.MessageID
	if !Allowed(h.Config, userID, chatID) {
		return
	}

	data := strings.TrimSpace(q.Data)
	h.ackCallback(q.ID, "")

	switch {
	case data == callbackRefresh:
		h.sendStatus(chatID, messageID)
	case strings.HasPrefix(data, callbackInviteAccept):
		h.inviteActions().showInviteConfirm(chatID, messageID, strings.TrimPrefix(data, callbackInviteAccept), "accept")
	case strings.HasPrefix(data, callbackInviteReject):
		h.inviteActions().showInviteConfirm(chatID, messageID, strings.TrimPrefix(data, callbackInviteReject), "reject")
	case strings.HasPrefix(data, callbackInviteDefer):
		h.inviteActions().executeDefer(ctx, chatID, messageID, strings.TrimPrefix(data, callbackInviteDefer))
	case strings.HasPrefix(data, callbackInviteConfirmAccept):
		h.inviteActions().executeAccept(ctx, chatID, messageID, strings.TrimPrefix(data, callbackInviteConfirmAccept))
	case strings.HasPrefix(data, callbackInviteConfirmReject):
		h.inviteActions().executeReject(ctx, chatID, messageID, strings.TrimPrefix(data, callbackInviteConfirmReject))
	case strings.HasPrefix(data, callbackInviteCancel):
		h.inviteActions().cancelInviteConfirm(chatID, messageID, strings.TrimPrefix(data, callbackInviteCancel))
	case strings.HasPrefix(data, callbackEnergyAdd):
		traceID, amount, ok := ParseEnergyCallback(strings.TrimPrefix(data, callbackEnergyAdd))
		if ok {
			h.inviteActions().addEnergy(ctx, chatID, messageID, traceID, amount)
		}
	}
}

func (h *Handler) inviteActions() *InviteActions {
	if h.Invites != nil {
		return h.Invites
	}
	return &InviteActions{
		Colony: h.Colony,
		Config: h.Config,
		Bot:    h.Bot,
	}
}

// ParseEnergyCallback parses en:+:<traceId>:<amount> callback suffix.
func ParseEnergyCallback(rest string) (traceID string, amount int, ok bool) {
	parts := strings.Split(rest, ":")
	if len(parts) < 2 {
		return "", 0, false
	}
	amount, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil || amount <= 0 {
		return "", 0, false
	}
	traceID = strings.Join(parts[:len(parts)-1], ":")
	if traceID == "" {
		return "", 0, false
	}
	return traceID, amount, true
}

func (h *Handler) sendStatus(chatID int64, editMessageID int) {
	snap, err := BuildSnapshot(h.Colony, h.Supervisor)
	if err != nil {
		h.sendPlain(chatID, editMessageID, "status unavailable: "+err.Error())
		return
	}
	text := FormatStatus(snap)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Refresh", callbackRefresh),
		),
	)
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		edit.ReplyMarkup = &keyboard
		_, _ = h.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	_, _ = h.Bot.Send(msg)
}

func (h *Handler) sendHelp(chatID int64) {
	h.sendPlain(chatID, 0, HelpText)
}

func (h *Handler) sendPlain(chatID int64, editMessageID int, text string) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, _ = h.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = h.Bot.Send(msg)
}

func (h *Handler) ackCallback(callbackID, text string) {
	cb := tgbotapi.NewCallback(callbackID, text)
	_, _ = h.Bot.Request(cb)
}
