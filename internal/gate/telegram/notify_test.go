package telegram

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

type recordingBot struct {
	messages            []string
	disableNotification []bool
}

func (b *recordingBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	switch v := c.(type) {
	case tgbotapi.MessageConfig:
		b.messages = append(b.messages, v.Text)
		b.disableNotification = append(b.disableNotification, v.DisableNotification)
	case tgbotapi.EditMessageTextConfig:
		b.messages = append(b.messages, v.Text)
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

func (b *recordingBot) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestNotifierPushBlockedTaskDedup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-blocked"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	task := taskledger.TaskSnapshot{
		TaskID:  "task-1",
		Title:   "Build feature",
		Bee:     "builder",
		Status:  protocol.TaskStatusBlocked,
		Summary: protocol.HoneyReserveExhaustedSummary,
	}
	if err := n.pushTaskStatus(context.Background(), "trace-1", task, protocol.TaskStatusBlocked); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d", len(bot.messages))
	}
	if !strings.Contains(bot.messages[0], "honey:") {
		t.Fatalf("missing honey line: %q", bot.messages[0])
	}
	if len(bot.disableNotification) != 1 || bot.disableNotification[0] {
		t.Fatal("blocked default should be sound (DisableNotification=false)")
	}
	if err := n.pushTaskStatus(context.Background(), "trace-1", task, protocol.TaskStatusBlocked); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("dedup failed: messages = %d", len(bot.messages))
	}
}

func TestNotifierPushInviteDedup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-push"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	invite := colony.InviteEntry{
		InviteID: "inv-dedup",
		TraceID:  "trace-1",
		Bee:      "drone",
		Task:     "Task",
		Status:   colony.InviteStatusPending,
	}
	if err := n.pushInvite(context.Background(), invite); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d", len(bot.messages))
	}
	if !strings.Contains(bot.messages[0], "honey:") {
		t.Fatalf("missing honey line: %q", bot.messages[0])
	}
	if err := n.pushInvite(context.Background(), invite); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("dedup failed: messages = %d", len(bot.messages))
	}
}

func TestNotifierPushSilentMode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	silent := NotifySilent
	state, err := LoadNotifyState("notify-silent")
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: "notify-silent"},
		Config: Config{
			ChatIDs: []int64{-100},
			Notify:  NotifyConfig{Blocked: &silent},
		},
		Bot:   bot,
		state: state,
	}
	task := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "Build",
		Status: protocol.TaskStatusBlocked,
	}
	if err := n.pushTaskStatus(context.Background(), "trace-1", task, protocol.TaskStatusBlocked); err != nil {
		t.Fatal(err)
	}
	if len(bot.disableNotification) != 1 || !bot.disableNotification[0] {
		t.Fatal("silent blocked push should set DisableNotification=true")
	}
}

func TestNotifierPushCompletedLive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-completed"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	task := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "Done task",
		Bee:    "builder",
	}
	payload := protocol.TaskCompletedPayload{
		Kind:        protocol.TaskEventCompleted,
		TaskID:      "task-1",
		Status:      protocol.TaskStatusCompleted,
		Summary:     "Shipped",
		Commit:      "abc123",
		CompletedAt: time.Now(),
	}
	mode := n.Config.Notify.Mode(NotifyCategoryCompleted)
	if err := n.pushTaskCompleted(context.Background(), "trace-1", task, payload, mode); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d", len(bot.messages))
	}
	if !strings.Contains(bot.messages[0], "Task completed") {
		t.Fatalf("card = %q", bot.messages[0])
	}
	if !strings.Contains(bot.messages[0], "abc123") {
		t.Fatalf("missing commit: %q", bot.messages[0])
	}
	if len(bot.disableNotification) != 1 || !bot.disableNotification[0] {
		t.Fatal("completed default should be silent")
	}
}

func TestHandleTaskCompletedEvent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-completed-ev"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	payload, _ := json.Marshal(protocol.TaskCompletedPayload{
		Kind:   protocol.TaskEventCompleted,
		TaskID: "task-1",
		Status: protocol.TaskStatusCompleted,
		Summary: "Done",
	})
	ev := protocol.Event{
		TraceID: "trace-1",
		Type:    protocol.EventVerification,
		Payload: payload,
	}
	if err := n.handleTaskCompletedEvent(ev); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 1 {
		t.Fatalf("messages = %d", len(bot.messages))
	}
}

func TestCommitGateNotifyOffByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)

	slug := "notify-commit-gate"
	state, err := LoadNotifyState(slug)
	if err != nil {
		t.Fatal(err)
	}
	bot := &recordingBot{}
	n := &Notifier{
		Colony: colony.Context{Slug: slug},
		Config: Config{ChatIDs: []int64{-100}},
		Bot:    bot,
		state:  state,
	}
	task := taskledger.TaskSnapshot{
		TaskID: "task-1",
		Title:  "AFK defer",
		Review: protocol.TaskReviewNone,
		Status: protocol.TaskStatusWaitingReview,
	}
	if err := n.pushTaskStatus(context.Background(), "trace-1", task, protocol.TaskStatusWaitingReview); err != nil {
		t.Fatal(err)
	}
	if len(bot.messages) != 0 {
		t.Fatalf("commit_gate should be off by default, got %d messages", len(bot.messages))
	}
}

func TestFormatTaskCompletedCard(t *testing.T) {
	task := taskledger.TaskSnapshot{TaskID: "task-1", Title: "Feature", Bee: "builder"}
	payload := protocol.TaskCompletedPayload{
		Summary: "All good",
		Commit:  "deadbeef",
	}
	text := FormatTaskCompletedCard("trace-1", task, payload)
	if !strings.Contains(text, "Task completed") {
		t.Fatalf("missing header: %q", text)
	}
	if !strings.Contains(text, "deadbeef") {
		t.Fatalf("missing commit: %q", text)
	}
}
