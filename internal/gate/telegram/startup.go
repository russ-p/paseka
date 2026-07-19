package telegram

import (
	"context"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/logging"
	"github.com/paseka/paseka/internal/runtime"
)

// builtinMenuCommands is the fixed setMyCommands order for built-in slash commands
// that work without arguments (parameterized commands stay in /help only).
var builtinMenuCommands = []struct {
	name        string
	description string
}{
	{name: "start", description: "Colony snapshot"},
	{name: "status", description: "Colony snapshot (Refresh button)"},
	{name: "help", description: "Command list"},
	{name: "invites", description: "Pending session invites"},
	{name: "traces", description: "Recent colony traces"},
}

// replyKeyboardCommands are slash commands shown on the reply keyboard (no-arg only).
var replyKeyboardCommands = []string{
	"/status",
	"/help",
	"/invites",
}

// BuildBotCommands assembles setMyCommands entries for no-arg built-ins only.
func BuildBotCommands() []tgbotapi.BotCommand {
	out := make([]tgbotapi.BotCommand, 0, len(builtinMenuCommands))
	for _, cmd := range builtinMenuCommands {
		out = append(out, tgbotapi.BotCommand{
			Command:     cmd.name,
			Description: cmd.description,
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
		),
	)
}

// ReplyKeyboardButtonTexts returns the slash-command text of each reply keyboard button (row-major).
func ReplyKeyboardButtonTexts() []string {
	return append([]string(nil), replyKeyboardCommands...)
}

// CommandMenuScopes returns Telegram command scopes to clear and refresh on startup.
func CommandMenuScopes(chatIDs []int64) []tgbotapi.BotCommandScope {
	scopes := []tgbotapi.BotCommandScope{
		tgbotapi.NewBotCommandScopeDefault(),
		tgbotapi.NewBotCommandScopeAllPrivateChats(),
	}
	seen := make(map[int64]struct{}, len(chatIDs))
	for _, chatID := range chatIDs {
		if _, ok := seen[chatID]; ok {
			continue
		}
		seen[chatID] = struct{}{}
		scopes = append(scopes, tgbotapi.NewBotCommandScopeChat(chatID))
	}
	return scopes
}

func syncBotCommandMenu(bot BotAPI, chatIDs []int64, commands []tgbotapi.BotCommand, log *logging.Logger) {
	scopes := CommandMenuScopes(chatIDs)
	for _, scope := range scopes {
		del := tgbotapi.NewDeleteMyCommandsWithScope(scope)
		if _, err := bot.Request(del); err != nil {
			log.Warn("telegram deleteMyCommands failed",
				logging.F("scope_type", scope.Type),
				logging.F("error", err.Error()),
			)
		}
		set := tgbotapi.NewSetMyCommandsWithScope(scope, commands...)
		if _, err := bot.Request(set); err != nil {
			log.Error("telegram setMyCommands failed",
				logging.F("scope_type", scope.Type),
				logging.F("error", err.Error()),
			)
		}
	}
	log.Info("telegram command menu registered",
		logging.F("count", strconv.Itoa(len(commands))),
		logging.F("scopes", strconv.Itoa(len(scopes))),
	)
}

// PresentOnStartup registers the bot command menu and installs a fresh reply keyboard in allowlisted chats.
// Failures are logged and do not return an error.
func PresentOnStartup(ctx context.Context, bot BotAPI, col colony.Context, sup *runtime.Supervisor, cfg Config, log *logging.Logger) {
	if bot == nil {
		return
	}
	if log == nil {
		log = logging.Component("gate.telegram")
	}

	commands := BuildBotCommands()
	syncBotCommandMenu(bot, cfg.ChatIDs, commands, log)

	keyboard := BuildReplyKeyboard()
	welcomeText := FormatWelcomeFallback(col.Slug)
	if snap, err := BuildSnapshot(col, sup); err == nil {
		welcomeText = FormatWelcome(snap)
	} else {
		log.Warn("telegram welcome status unavailable", logging.F("error", err.Error()))
	}
	for _, chatID := range cfg.ChatIDs {
		if ctx.Err() != nil {
			return
		}
		install := tgbotapi.NewMessage(chatID, welcomeText)
		install.ReplyMarkup = keyboard
		if _, err := bot.Send(install); err != nil {
			log.Error("telegram reply keyboard install failed",
				logging.F("chat_id", strconv.FormatInt(chatID, 10)),
				logging.F("error", err.Error()),
			)
		}
	}
}
