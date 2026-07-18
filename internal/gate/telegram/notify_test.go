package telegram

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
)

type recordingBot struct {
	messages []string
}

func (b *recordingBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	switch v := c.(type) {
	case tgbotapi.MessageConfig:
		b.messages = append(b.messages, v.Text)
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
