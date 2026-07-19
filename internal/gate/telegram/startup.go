package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/logging"
)

// builtinMenuCommands is the fixed setMyCommands order for built-in slash commands.
var builtinMenuCommands = []struct {
	name        string
	description string
}{
	{name: "start", description: "Colony snapshot"},
	{name: "status", description: "Colony snapshot (Refresh button)"},
	{name: "help", description: "Command list"},
	{name: "invites", description: "Pending session invites"},
	{name: "energy", description: "Honey reserve (remaining/budget)"},
	{name: "task", description: "Inject task (preview + Confirm)"},
}

// replyKeyboardCommands are high-frequency slash commands shown on the reply keyboard.
var replyKeyboardCommands = []string{
	"/status",
	"/help",
	"/invites",
	"/energy",
	"/task",
}

// BuildBotCommands assembles setMyCommands entries: built-ins in fixed order, then custom (sorted).
func BuildBotCommands(commands CommandsConfig) []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, 0, len(builtinMenuCommands)+len(commands.Custom))
	for _, cmd := range builtinMenuCommands {
		out = append(out, tgbotapi.BotCommand{
			Command:     cmd.name,
			Description: cmd.description,
		})
	}
	names := make([]string, 0, len(commands.Custom))
	for name := range commands.Custom {
		names = append(names, name)
	}
	sortCustomCommandNames(names)
	for _, name := range names {
		cfg := commands.Custom[name]
		desc := strings.TrimSpace(cfg.Description)
		if desc == "" {
			desc = fmt.Sprintf("publish SIGNAL/%s", strings.TrimSpace(cfg.Kind))
		}
		out = append(out, tgbotapi.BotCommand{
			Command:     name,
			Description: desc,
		})
	}
	return out
}

// BuildReplyKeyboard returns a compact reply keyboard whose buttons send slash commands.
func BuildReplyKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/status"),
			tgbotapi.NewKeyboardButton("/help"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("/invites"),
			tgbotapi.NewKeyboardButton("/energy"),
			tgbotapi.NewKeyboardButton("/task"),
		),
	)
}

// ReplyKeyboardButtonTexts returns the slash-command text of each reply keyboard button (row-major).
func ReplyKeyboardButtonTexts() []string {
	return append([]string(nil), replyKeyboardCommands...)
}

// PresentOnStartup registers the bot command menu and installs a fresh reply keyboard in allowlisted chats.
// Failures are logged and do not return an error.
func PresentOnStartup(ctx context.Context, bot BotAPI, cfg Config, log *logging.Logger) {
	if bot == nil {
		return
	}
	if log == nil {
		log = logging.Component("gate.telegram")
	}

	commands := BuildBotCommands(cfg.Commands)
	setCmds := tgbotapi.NewSetMyCommands(commands...)
	if _, err := bot.Request(setCmds); err != nil {
		log.Error("telegram setMyCommands failed", logging.F("error", err.Error()))
	} else {
		log.Info("telegram command menu registered", logging.F("count", strconv.Itoa(len(commands))))
	}

	keyboard := BuildReplyKeyboard()
	for _, chatID := range cfg.ChatIDs {
		if ctx.Err() != nil {
			return
		}
		remove := tgbotapi.NewMessage(chatID, " ")
		remove.ReplyMarkup = tgbotapi.NewRemoveKeyboard(false)
		if _, err := bot.Send(remove); err != nil {
			log.Error("telegram reply keyboard remove failed",
				logging.F("chat_id", strconv.FormatInt(chatID, 10)),
				logging.F("error", err.Error()),
			)
			continue
		}
		install := tgbotapi.NewMessage(chatID, "Quick commands ready.")
		install.ReplyMarkup = keyboard
		if _, err := bot.Send(install); err != nil {
			log.Error("telegram reply keyboard install failed",
				logging.F("chat_id", strconv.FormatInt(chatID, 10)),
				logging.F("error", err.Error()),
			)
		}
	}
}
