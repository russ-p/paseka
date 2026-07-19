package telegram_test

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestCommandMenuScopesDedupesChatIDs(t *testing.T) {
	scopes := tggate.CommandMenuScopes([]int64{10, 10})
	if len(scopes) != 3 {
		t.Fatalf("got %d scopes, want 3 (default, private, chat)", len(scopes))
	}
	if scopes[2].Type != "chat" || scopes[2].ChatID != 10 {
		t.Fatalf("unexpected chat scope: %+v", scopes[2])
	}
}

func TestPresentOnStartupRefreshesCommandMenuAcrossScopes(t *testing.T) {
	bot := &mockBot{}
	cfg := tggate.Config{ChatIDs: []int64{129091866}}
	tggate.PresentOnStartup(
		context.Background(),
		bot,
		colony.Context{Slug: "acme-api"},
		nil,
		cfg,
		nil,
	)

	var deletes, sets int
	for _, req := range bot.requests {
		switch req.(type) {
		case tgbotapi.DeleteMyCommandsConfig:
			deletes++
		case tgbotapi.SetMyCommandsConfig:
			sets++
		}
	}
	if deletes != 3 || sets != 3 {
		t.Fatalf("expected 3 delete + 3 set requests, got deletes=%d sets=%d", deletes, sets)
	}
}

func TestBuildBotCommandsBuiltinOrderAndDescriptions(t *testing.T) {
	commands := tggate.BuildBotCommands()
	want := []struct {
		name string
		desc string
	}{
		{"start", "Colony snapshot"},
		{"status", "Colony snapshot (Refresh button)"},
		{"help", "Command list"},
		{"invites", "Pending session invites"},
	}
	if len(commands) != len(want) {
		t.Fatalf("got %d commands, want %d", len(commands), len(want))
	}
	for i, w := range want {
		if commands[i].Command != w.name {
			t.Fatalf("commands[%d].Command = %q, want %q", i, commands[i].Command, w.name)
		}
		if commands[i].Description != w.desc {
			t.Fatalf("commands[%d].Description = %q, want %q", i, commands[i].Description, w.desc)
		}
	}
}

func TestBuildBotCommandsExcludesParameterizedCommands(t *testing.T) {
	commands := tggate.BuildBotCommands()
	for _, cmd := range commands {
		switch cmd.Command {
		case "energy", "task":
			t.Fatalf("parameterized command %q must not appear in setMyCommands", cmd.Command)
		}
	}
}

func TestBuildReplyKeyboardShape(t *testing.T) {
	kb := tggate.BuildReplyKeyboard()
	if !kb.ResizeKeyboard {
		t.Fatal("expected ResizeKeyboard true")
	}
	if len(kb.Keyboard) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(kb.Keyboard))
	}
	if len(kb.Keyboard[0]) != 2 || len(kb.Keyboard[1]) != 1 {
		t.Fatalf("unexpected row widths: %#v", kb.Keyboard)
	}

	var got []string
	for _, row := range kb.Keyboard {
		for _, btn := range row {
			got = append(got, btn.Text)
		}
	}
	want := tggate.ReplyKeyboardButtonTexts()
	if len(got) != len(want) {
		t.Fatalf("got %d buttons, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("button[%d] = %q, want %q", i, got[i], want[i])
		}
		if !strings.HasPrefix(got[i], "/") {
			t.Fatalf("button[%d] must be slash command, got %q", i, got[i])
		}
	}
}

func TestReplyKeyboardButtonsRecognizedAsCommands(t *testing.T) {
	for _, text := range tggate.ReplyKeyboardButtonTexts() {
		cmd, _, ok := tggate.MessageCommand(&tgbotapi.Message{Text: text})
		if !ok {
			t.Fatalf("%q is not recognized as a command", text)
		}
		if cmd == "" {
			t.Fatalf("empty command for %q", text)
		}
	}
}
