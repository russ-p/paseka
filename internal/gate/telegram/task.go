package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/tasks"
)

const maxTaskPreviewLen = 300

// TaskActions handles /task preview and confirm flow.
type TaskActions struct {
	Colony  colony.Context
	Config  Config
	Bot     BotAPI
	Pending *PendingTasks
}

func (a *TaskActions) pending() *PendingTasks {
	if a.Pending != nil {
		return a.Pending
	}
	return NewPendingTasks()
}

// HandleCommand shows a preview card for /task <text>.
func (a *TaskActions) HandleCommand(ctx context.Context, chatID int64, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		a.sendText(chatID, 0, "Usage: /task <description>")
		return
	}
	pendingID, err := a.pending().Put(PendingTask{
		Text:    text,
		Bee:     a.Config.Commands.DefaultBee,
		Intent:  a.Config.Commands.DefaultIntent,
		Review:  a.Config.Commands.DefaultReview,
		Autorun: a.Config.Commands.AutorunEnabled(),
	})
	if err != nil {
		a.sendText(chatID, 0, "task preview failed: "+err.Error())
		return
	}
	body := FormatTaskPreview(
		a.Config.Commands.DefaultBee,
		a.Config.Commands.DefaultIntent,
		a.Config.Commands.DefaultReview,
		text,
		a.Config.Commands.AutorunEnabled(),
	)
	keyboard := taskPreviewKeyboard(pendingID)
	msg := tgbotapi.NewMessage(chatID, body)
	msg.ReplyMarkup = keyboard
	_, _ = a.Bot.Send(msg)
}

// FormatTaskPreview renders the /task confirm card body.
func FormatTaskPreview(bee, intent, review, text string, autorun bool) string {
	intent = strings.TrimSpace(intent)
	review = strings.TrimSpace(review)
	if review == "" {
		review = string(protocol.TaskReviewNone)
	}
	autorunLabel := "no"
	if autorun {
		autorunLabel = "yes"
	}
	lines := []string{
		"Task preview",
		fmt.Sprintf("Bee: %s", bee),
	}
	if intent != "" {
		lines = append(lines, fmt.Sprintf("Intent: %s", intent))
	}
	lines = append(lines,
		fmt.Sprintf("Review: %s", review),
		fmt.Sprintf("Autorun: %s", autorunLabel),
		fmt.Sprintf("Text: %s", truncateText(text, maxTaskPreviewLen)),
		"",
		"Confirm to publish task.plan"+autorunSuffix(autorun),
	)
	return strings.Join(lines, "\n")
}

func autorunSuffix(autorun bool) string {
	if autorun {
		return " and task.ready."
	}
	return "."
}

func taskPreviewKeyboard(pendingID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Confirm", callbackTaskConfirm+pendingID),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", callbackTaskCancel+pendingID),
		),
	)
}

// Confirm executes task.plan (+ optional task.ready) for a pending preview.
func (a *TaskActions) Confirm(ctx context.Context, chatID int64, messageID int, pendingID string) {
	task, ok := a.pending().Take(pendingID)
	if !ok {
		a.sendText(chatID, messageID, "Task preview expired. Send /task again.")
		return
	}
	session, err := tasks.OpenLedger(a.Colony)
	if err != nil {
		a.sendText(chatID, messageID, "task create failed: "+err.Error())
		return
	}
	defer session.Close()
	if session.Client == nil {
		a.sendText(chatID, messageID, "task create failed: nats url not configured")
		return
	}
	res, err := tasks.Create(ctx, session, tasks.CreateInput{
		Body:    task.Text,
		Bee:     task.Bee,
		Intent:  task.Intent,
		Review:  task.Review,
		Autorun: task.Autorun,
		AgentID: "telegram",
	})
	if err != nil {
		a.sendText(chatID, messageID, "task create failed: "+err.Error())
		return
	}
	msg := fmt.Sprintf("Task created\nTrace: %s\nTask: %s\nBee: %s", res.TraceID, res.TaskID, res.Bee)
	if res.Autorun {
		msg += "\nPublished task.ready."
	}
	a.sendText(chatID, messageID, msg)
}

// Cancel discards a pending preview.
func (a *TaskActions) Cancel(chatID int64, messageID int, pendingID string) {
	a.pending().Take(pendingID)
	a.sendText(chatID, messageID, "Task cancelled.")
}

func (a *TaskActions) sendText(chatID int64, editMessageID int, text string) {
	if editMessageID > 0 {
		edit := tgbotapi.NewEditMessageText(chatID, editMessageID, text)
		_, _ = a.Bot.Send(edit)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	_, _ = a.Bot.Send(msg)
}
