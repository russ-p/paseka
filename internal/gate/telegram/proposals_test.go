package telegram

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

func TestCanApproveFromTelegram(t *testing.T) {
	required := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Status: protocol.TaskStatusWaitingReview,
		Review: protocol.TaskReviewRequired,
	}
	if !CanApproveFromTelegram(required) {
		t.Fatal("review: required should allow telegram approve")
	}

	final := taskledger.TaskSnapshot{
		TaskID: "_review",
		Status: protocol.TaskStatusWaitingReview,
		Review: protocol.TaskReviewFinal,
	}
	if CanApproveFromTelegram(final) {
		t.Fatal("review: final must not allow telegram approve")
	}

	plain := taskledger.TaskSnapshot{
		TaskID: "task-2",
		Status: protocol.TaskStatusWaitingReview,
		Review: protocol.TaskReviewNone,
	}
	if CanApproveFromTelegram(plain) {
		t.Fatal("non-review-gate task must not allow approve")
	}
}

func TestProposalActionKeyboardFinalMergeRejectOnly(t *testing.T) {
	kb := proposalActionKeyboard("trace-1", "_review", false)
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("expected single Reject row, got %#v", kb.InlineKeyboard)
	}
	if kb.InlineKeyboard[0][0].Text != "Reject" {
		t.Fatalf("button = %q, want Reject", kb.InlineKeyboard[0][0].Text)
	}
}

func TestProposalActionKeyboardMidApproveAndReject(t *testing.T) {
	kb := proposalActionKeyboard("trace-1", "task-1", true)
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 2 {
		t.Fatalf("expected Approve+Reject row, got %#v", kb.InlineKeyboard)
	}
	if kb.InlineKeyboard[0][0].Text != "Approve" || kb.InlineKeyboard[0][1].Text != "Reject" {
		t.Fatalf("buttons = %#v", kb.InlineKeyboard[0])
	}
}

func TestParseProposalCallback(t *testing.T) {
	traceID, taskID, ok := ParseProposalCallback("trace-abc/task-1")
	if !ok || traceID != "trace-abc" || taskID != "task-1" {
		t.Fatalf("got trace=%q task=%q ok=%v", traceID, taskID, ok)
	}
	_, _, ok = ParseProposalCallback("missing-separator")
	if ok {
		t.Fatal("expected invalid callback")
	}
}

func TestFormatTaskStatusCardFinalReviewConsoleHint(t *testing.T) {
	task := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "Merge feature",
		Review: protocol.TaskReviewFinal,
	}
	cfg := Config{ConsoleBaseURL: "https://console.example"}
	text := FormatTaskStatusCard(colonyContextForTest(), cfg, nil, "trace-1", task, protocol.TaskStatusWaitingReview)
	if !strings.Contains(text, "Review: final") {
		t.Fatalf("missing review policy:\n%s", text)
	}
	if !strings.Contains(text, "Console or CLI only") {
		t.Fatalf("missing console-only hint:\n%s", text)
	}
	if !strings.Contains(text, "https://console.example") {
		t.Fatalf("missing console url:\n%s", text)
	}
}

func TestNotifierPushWaitingReviewProposalKeyboard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-review"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBotWithKeyboard{}
	n := &Notifier{
		Colony: colonyContextForTest(),
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	task := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "Add OAuth",
		Bee:    "builder",
		Review: protocol.TaskReviewRequired,
		Status: protocol.TaskStatusWaitingReview,
	}
	if err := n.pushTaskStatus(t.Context(), "trace-1", task, protocol.TaskStatusWaitingReview); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d", len(bot.messages))
	}
	if bot.lastKeyboard == nil || len(bot.lastKeyboard.InlineKeyboard) == 0 {
		t.Fatal("expected proposal keyboard")
	}
	row := bot.lastKeyboard.InlineKeyboard[0]
	if len(row) != 2 || row[0].Text != "Approve" || row[1].Text != "Reject" {
		t.Fatalf("keyboard = %#v", row)
	}
}

func TestNotifierPushFinalReviewRejectOnlyKeyboard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-final"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBotWithKeyboard{}
	n := &Notifier{
		Colony: colonyContextForTest(),
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	task := taskledger.TaskSnapshot{
		TaskID: "_review",
		Title:  "Final merge",
		Review: protocol.TaskReviewFinal,
		Status: protocol.TaskStatusWaitingReview,
	}
	if err := n.pushTaskStatus(t.Context(), "trace-1", task, protocol.TaskStatusWaitingReview); err != nil {
		t.Fatal(err)
	}
	row := bot.lastKeyboard.InlineKeyboard[0]
	if len(row) != 1 || row[0].Text != "Reject" {
		t.Fatalf("final review keyboard = %#v", row)
	}
}

func colonyContextForTest() colony.Context {
	return colony.Context{Slug: "test"}
}

type recordingBotWithKeyboard struct {
	messages     []string
	lastKeyboard *tgbotapi.InlineKeyboardMarkup
}

func (b *recordingBotWithKeyboard) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	switch v := c.(type) {
	case tgbotapi.MessageConfig:
		b.messages = append(b.messages, v.Text)
		if markup, ok := v.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup); ok {
			b.lastKeyboard = &markup
		}
	case tgbotapi.EditMessageTextConfig:
		b.messages = append(b.messages, v.Text)
		if v.ReplyMarkup != nil {
			b.lastKeyboard = v.ReplyMarkup
		}
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

func (b *recordingBotWithKeyboard) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}
