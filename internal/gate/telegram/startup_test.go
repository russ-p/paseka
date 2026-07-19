package telegram_test

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func TestBuildBotCommandsBuiltinOrderAndDescriptions(t *testing.T) {
	commands := tggate.BuildBotCommands(tggate.CommandsConfig{})
	want := []struct {
		name string
		desc string
	}{
		{"start", "Colony snapshot"},
		{"status", "Colony snapshot (Refresh button)"},
		{"help", "Command list"},
		{"invites", "Pending session invites"},
		{"energy", "Honey reserve (remaining/budget)"},
		{"task", "Inject task (preview + Confirm)"},
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

func TestBuildBotCommandsCustomSortedAfterBuiltins(t *testing.T) {
	commands := tggate.BuildBotCommands(tggate.CommandsConfig{
		Custom: map[string]tggate.CustomCommandConfig{
			"zebra":   {Description: "Z command", Kind: "z.kind"},
			"feature": {Description: "Intake idea/bug via Scout", Kind: "feature.requested"},
		},
	})
	if len(commands) != 8 {
		t.Fatalf("got %d commands, want 8", len(commands))
	}
	if commands[6].Command != "feature" || commands[6].Description != "Intake idea/bug via Scout" {
		t.Fatalf("custom feature: got %+v", commands[6])
	}
	if commands[7].Command != "zebra" || commands[7].Description != "Z command" {
		t.Fatalf("custom zebra: got %+v", commands[7])
	}
}

func TestBuildBotCommandsCustomDescriptionFallback(t *testing.T) {
	commands := tggate.BuildBotCommands(tggate.CommandsConfig{
		Custom: map[string]tggate.CustomCommandConfig{
			"ping": {Kind: "ping.requested"},
		},
	})
	if len(commands) != 7 {
		t.Fatalf("got %d commands, want 7", len(commands))
	}
	last := commands[len(commands)-1]
	if last.Command != "ping" {
		t.Fatalf("last command = %q, want ping", last.Command)
	}
	if last.Description != "publish SIGNAL/ping.requested" {
		t.Fatalf("description = %q", last.Description)
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
	if len(kb.Keyboard[0]) != 2 || len(kb.Keyboard[1]) != 3 {
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
