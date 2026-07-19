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
	Energy     *EnergyActions
	Tasks      *TaskActions
	Signals    *SignalActions
	Proposals  *ProposalActions
}

// HandleUpdate processes one Telegram update. Non-allowlisted traffic is silently ignored.
func (h *Handler) HandleUpdate(ctx context.Context, update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		h.handleCallback(ctx, update.CallbackQuery)
		return
	}
	if update.Message == nil {
		return
	}
	command, args, ok := MessageCommand(update.Message)
	if !ok {
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

	switch command {
	case "start", "status":
		h.sendStatus(chatID, 0)
	case "help":
		h.sendHelp(chatID)
	case "invites":
		h.inviteActions().SendInvitesList(chatID)
	case "traces":
		h.sendTraces(chatID)
	case "energy":
		h.energyActions().HandleCommand(ctx, chatID, args)
	case "task":
		h.taskActions().HandleCommand(ctx, chatID, args)
	default:
		if h.Config.Commands.Custom != nil {
			if _, ok := h.Config.Commands.Custom[command]; ok {
				h.signalActions().HandleCommand(ctx, chatID, command, args)
			}
		}
	}
}

// MessageCommand returns the slash command and arguments from a Telegram message.
// Reply keyboard buttons send plain "/command" text without bot_command entities, so
// both entity-based and plain-text slash commands are accepted.
func MessageCommand(msg *tgbotapi.Message) (command string, args string, ok bool) {
	if msg.IsCommand() {
		return msg.Command(), msg.CommandArguments(), true
	}
	text := strings.TrimSpace(msg.Text)
	if !strings.HasPrefix(text, "/") {
		return "", "", false
	}
	parts := strings.SplitN(text, " ", 2)
	command = strings.TrimPrefix(parts[0], "/")
	if i := strings.Index(command, "@"); i != -1 {
		command = command[:i]
	}
	if command == "" {
		return "", "", false
	}
	if len(parts) > 1 {
		args = parts[1]
	}
	return command, args, true
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
			h.energyActions().Add(ctx, chatID, messageID, traceID, amount)
		}
	case strings.HasPrefix(data, callbackTaskConfirm):
		h.taskActions().Confirm(ctx, chatID, messageID, strings.TrimPrefix(data, callbackTaskConfirm))
	case strings.HasPrefix(data, callbackTaskCancel):
		h.taskActions().Cancel(chatID, messageID, strings.TrimPrefix(data, callbackTaskCancel))
	case strings.HasPrefix(data, callbackSignalConfirm):
		h.signalActions().Confirm(ctx, chatID, messageID, strings.TrimPrefix(data, callbackSignalConfirm))
	case strings.HasPrefix(data, callbackSignalCancel):
		h.signalActions().Cancel(chatID, messageID, strings.TrimPrefix(data, callbackSignalCancel))
	case strings.HasPrefix(data, callbackProposalApprove):
		traceID, taskID, ok := ParseProposalCallback(strings.TrimPrefix(data, callbackProposalApprove))
		if ok {
			h.proposalActions().showProposalConfirm(chatID, messageID, traceID, taskID, "approve")
		}
	case strings.HasPrefix(data, callbackProposalReject):
		traceID, taskID, ok := ParseProposalCallback(strings.TrimPrefix(data, callbackProposalReject))
		if ok {
			h.proposalActions().showProposalConfirm(chatID, messageID, traceID, taskID, "reject")
		}
	case strings.HasPrefix(data, callbackProposalConfirmApprove):
		traceID, taskID, ok := ParseProposalCallback(strings.TrimPrefix(data, callbackProposalConfirmApprove))
		if ok {
			h.proposalActions().executeApprove(ctx, chatID, messageID, traceID, taskID)
		}
	case strings.HasPrefix(data, callbackProposalConfirmReject):
		traceID, taskID, ok := ParseProposalCallback(strings.TrimPrefix(data, callbackProposalConfirmReject))
		if ok {
			h.proposalActions().executeReject(ctx, chatID, messageID, traceID, taskID)
		}
	case strings.HasPrefix(data, callbackProposalCancel):
		traceID, taskID, ok := ParseProposalCallback(strings.TrimPrefix(data, callbackProposalCancel))
		if ok {
			h.proposalActions().cancelProposalConfirm(chatID, messageID, traceID, taskID)
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

func (h *Handler) energyActions() *EnergyActions {
	if h.Energy != nil {
		return h.Energy
	}
	return &EnergyActions{
		Colony: h.Colony,
		Bot:    h.Bot,
	}
}

func (h *Handler) taskActions() *TaskActions {
	if h.Tasks != nil {
		return h.Tasks
	}
	return &TaskActions{
		Colony:  h.Colony,
		Config:  h.Config,
		Bot:     h.Bot,
		Pending: NewPendingTasks(),
	}
}

func (h *Handler) signalActions() *SignalActions {
	if h.Signals != nil {
		return h.Signals
	}
	return &SignalActions{
		Colony:  h.Colony,
		Config:  h.Config,
		Bot:     h.Bot,
		Pending: NewPendingSignals(),
	}
}

func (h *Handler) proposalActions() *ProposalActions {
	if h.Proposals != nil {
		return h.Proposals
	}
	return &ProposalActions{
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
	h.sendPlain(chatID, 0, FormatHelpText(h.Config.Commands))
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
