package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/review"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

// ProposalActions handles proposal approve/reject callbacks for the Telegram gate.
type ProposalActions struct {
	Colony colony.Context
	Config Config
	Bot    BotAPI
}

// CanApproveFromTelegram reports whether approve may be offered for a review-gated task.
// Final-merge gates (review: final) are Console/CLI only per spec 010.
func CanApproveFromTelegram(task taskledger.TaskSnapshot) bool {
	if task.Status != protocol.TaskStatusWaitingReview {
		return false
	}
	if !taskledger.IsReviewGate(task) {
		return false
	}
	return !taskledger.IsFinalReviewTask(task)
}

func proposalCallbackKey(traceID, taskID string) string {
	return traceID + "/" + taskID
}

// ParseProposalCallback splits traceId/taskId from a proposal callback suffix.
func ParseProposalCallback(rest string) (traceID, taskID string, ok bool) {
	traceID, taskID, ok = strings.Cut(rest, "/")
	if !ok || traceID == "" || taskID == "" {
		return "", "", false
	}
	return traceID, taskID, true
}

func proposalActionKeyboard(traceID, taskID string, canApprove bool) tgbotapi.InlineKeyboardMarkup {
	var buttons []tgbotapi.InlineKeyboardButton
	key := proposalCallbackKey(traceID, taskID)
	if canApprove {
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Approve", callbackProposalApprove+key))
	}
	buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData("Reject", callbackProposalReject+key))
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...))
}

func proposalConfirmKeyboard(traceID, taskID, action string) tgbotapi.InlineKeyboardMarkup {
	key := proposalCallbackKey(traceID, taskID)
	var confirmData string
	switch action {
	case "approve":
		confirmData = callbackProposalConfirmApprove + key
	case "reject":
		confirmData = callbackProposalConfirmReject + key
	default:
		confirmData = callbackProposalCancel + key
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Confirm", confirmData),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", callbackProposalCancel+key),
		),
	)
}

func proposalConsoleHint(cfg Config) string {
	lines := []string{"Approve in Console or CLI only (paseka proposal approve)."}
	if url := strings.TrimSpace(cfg.ConsoleBaseURL); url != "" {
		lines = append(lines, fmt.Sprintf("Console: %s", strings.TrimRight(url, "/")))
	}
	return strings.Join(lines, "\n")
}

func (a *ProposalActions) showProposalConfirm(chatID int64, messageID int, traceID, taskID, action string) {
	label := "approve"
	if action == "reject" {
		label = "reject"
	}
	text := fmt.Sprintf("Confirm %s proposal %s/%s?", label, traceID, taskID)
	keyboard := proposalConfirmKeyboard(traceID, taskID, action)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ReplyMarkup = &keyboard
	_, _ = a.Bot.Send(edit)
}

func (a *ProposalActions) cancelProposalConfirm(chatID int64, messageID int, traceID, taskID string) {
	task, status, ok := a.loadReviewTask(traceID, taskID)
	if !ok {
		a.sendText(chatID, messageID, fmt.Sprintf("Task %s/%s not found or not awaiting review.", traceID, taskID))
		return
	}
	ledger, closeLedger := a.ledgerSession()
	defer closeLedger()
	text := FormatTaskStatusCard(a.Colony, a.Config, ledger, traceID, task, status)
	keyboard := proposalKeyboardForTask(traceID, task)
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	if len(keyboard.InlineKeyboard) > 0 {
		edit.ReplyMarkup = &keyboard
	}
	_, _ = a.Bot.Send(edit)
}

func (a *ProposalActions) executeApprove(ctx context.Context, chatID int64, messageID int, traceID, taskID string) {
	if _, _, ok := a.loadReviewTask(traceID, taskID); !ok {
		a.sendText(chatID, messageID, fmt.Sprintf("Task %s/%s not found or not awaiting review.", traceID, taskID))
		return
	}
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, messageID, "approve failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Client == nil || session.Ledger == nil {
		a.sendText(chatID, messageID, "approve failed: nats url not configured")
		return
	}

	snap, err := session.Ledger.Snapshot(traceID)
	if err != nil {
		a.sendText(chatID, messageID, "approve failed: "+err.Error())
		return
	}
	task := snap.Tasks[taskID]
	if !CanApproveFromTelegram(task) {
		a.sendText(chatID, messageID, "This proposal must be approved in Console or CLI.")
		return
	}

	approveRes, err := review.Approve(ctx, a.Colony, session.Client, session.Ledger, review.ApproveInput{
		TraceID: traceID,
		TaskID:  taskID,
		AgentID: "telegram",
	})
	if err != nil {
		a.sendText(chatID, messageID, "approve failed: "+err.Error())
		return
	}
	msg := review.ApproveMessage(review.ApproveMessageOptions{
		ProposalWorkspace: task.ProposalWorkspace,
		CommitSHA:         approveRes.CommitSHA,
		StashOutcome:      approveRes.StashOutcome,
	})
	a.sendText(chatID, messageID, fmt.Sprintf("Proposal %s/%s approved.\n%s", traceID, taskID, msg))
}

func (a *ProposalActions) executeReject(ctx context.Context, chatID int64, messageID int, traceID, taskID string) {
	if _, _, ok := a.loadReviewTask(traceID, taskID); !ok {
		a.sendText(chatID, messageID, fmt.Sprintf("Task %s/%s not found or not awaiting review.", traceID, taskID))
		return
	}
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, messageID, "reject failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Client == nil || session.Ledger == nil {
		a.sendText(chatID, messageID, "reject failed: nats url not configured")
		return
	}

	if err := review.Reject(ctx, session.Client, session.Ledger, review.RejectInput{
		TraceID:  traceID,
		TaskID:   taskID,
		AgentID:  "telegram",
		Feedback: "Rejected from Telegram.",
	}); err != nil {
		a.sendText(chatID, messageID, "reject failed: "+err.Error())
		return
	}
	a.sendText(chatID, messageID, fmt.Sprintf("Proposal %s/%s rejected. Feedback published.", traceID, taskID))
}

func (a *ProposalActions) loadReviewTask(traceID, taskID string) (taskledger.TaskSnapshot, protocol.TaskStatus, bool) {
	ledger, closeLedger := a.ledgerSession()
	defer closeLedger()
	if ledger == nil {
		return taskledger.TaskSnapshot{}, "", false
	}
	snap, err := ledger.Snapshot(traceID)
	if err != nil {
		return taskledger.TaskSnapshot{}, "", false
	}
	task, ok := snap.Tasks[taskID]
	if !ok || task.Status != protocol.TaskStatusWaitingReview || !taskledger.IsReviewGate(task) {
		return taskledger.TaskSnapshot{}, "", false
	}
	return task, task.Status, true
}

func (a *ProposalActions) ledgerSession() (taskledger.Ledger, func()) {
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil || session == nil || session.Ledger == nil {
		return nil, func() {}
	}
	return session.Ledger, session.Close
}

func (a *ProposalActions) sendText(chatID int64, editMessageID int, text string) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = a.Bot.Send(msg)
}

func proposalKeyboardForTask(traceID string, task taskledger.TaskSnapshot) tgbotapi.InlineKeyboardMarkup {
	if !taskledger.IsReviewGate(task) || task.Status != protocol.TaskStatusWaitingReview {
		return tgbotapi.InlineKeyboardMarkup{}
	}
	return proposalActionKeyboard(traceID, task.TaskID, CanApproveFromTelegram(task))
}

// proposalReviewLines adds review-policy context for waiting_review proposal cards.
func proposalReviewLines(cfg Config, task taskledger.TaskSnapshot) []string {
	if !taskledger.IsReviewGate(task) {
		return nil
	}
	policy := string(protocol.NormalizeTaskReviewPolicy(task.Review))
	lines := []string{fmt.Sprintf("Review: %s", policy)}
	if taskledger.IsFinalReviewTask(task) {
		lines = append(lines, proposalConsoleHint(cfg))
	}
	return lines
}
