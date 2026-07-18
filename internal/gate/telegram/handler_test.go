package telegram_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

type mockBot struct {
	sent     []tgbotapi.Chattable
	requests []tgbotapi.Chattable
}

func (m *mockBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.sent = append(m.sent, c)
	return tgbotapi.Message{MessageID: 99}, nil
}

func (m *mockBot) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	m.requests = append(m.requests, c)
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func TestHandlerIgnoresNonAllowlistedCommand(t *testing.T) {
	bot := &mockBot{}
	h := &tggate.Handler{
		Config: tggate.Config{
			AllowFrom: []int64{1},
			ChatIDs:   []int64{-100},
		},
		Bot: bot,
	}
	update := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
			Text:     "/status",
			Chat:     &tgbotapi.Chat{ID: -100},
			From:     &tgbotapi.User{ID: 999},
		},
	}
	h.HandleUpdate(context.Background(), update)
	if len(bot.sent) != 0 {
		t.Fatalf("expected no replies, got %d", len(bot.sent))
	}
}

func TestHandlerStatusAndHelp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	repo := initTestRepo(t)
	ctxColony, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	bot := &mockBot{}
	h := &tggate.Handler{
		Colony: ctxColony,
		Config: tggate.Config{
			AllowFrom: []int64{1},
			ChatIDs:   []int64{-100},
		},
		Bot: bot,
	}

	statusUpdate := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 7}},
			Text:     "/status",
			Chat:     &tgbotapi.Chat{ID: -100},
			From:     &tgbotapi.User{ID: 1},
		},
	}
	h.HandleUpdate(context.Background(), statusUpdate)
	if len(bot.sent) != 1 {
		t.Fatalf("status: expected 1 send, got %d", len(bot.sent))
	}

	helpUpdate := tgbotapi.Update{
		Message: &tgbotapi.Message{
			Entities: []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: 5}},
			Text:     "/help",
			Chat:     &tgbotapi.Chat{ID: -100},
			From:     &tgbotapi.User{ID: 1},
		},
	}
	h.HandleUpdate(context.Background(), helpUpdate)
	if len(bot.sent) != 2 {
		t.Fatalf("help: expected 2 sends, got %d", len(bot.sent))
	}
}

func TestHandlerRefreshCallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	repo := initTestRepo(t)
	ctxColony, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}

	bot := &mockBot{}
	h := &tggate.Handler{
		Colony: ctxColony,
		Config: tggate.Config{
			AllowFrom: []int64{1},
			ChatIDs:   []int64{-100},
		},
		Bot: bot,
	}
	update := tgbotapi.Update{
		CallbackQuery: &tgbotapi.CallbackQuery{
			ID:   "cb1",
			Data: "gate:status:refresh",
			From: &tgbotapi.User{ID: 1},
			Message: &tgbotapi.Message{
				MessageID: 7,
				Chat:      &tgbotapi.Chat{ID: -100},
			},
		},
	}
	h.HandleUpdate(context.Background(), update)
	if len(bot.requests) != 1 {
		t.Fatalf("expected callback ack, got %d requests", len(bot.requests))
	}
	if len(bot.sent) != 1 {
		t.Fatalf("expected edited status, got %d sends", len(bot.sent))
	}
}

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "init")
	res, err := colony.Init(colony.InitOptions{StartDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return res.ColonyRoot
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
