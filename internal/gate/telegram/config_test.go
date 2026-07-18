package telegram_test

import (
	"os"
	"path/filepath"
	"testing"

	tggate "github.com/paseka/paseka/internal/gate/telegram"
)

func writeTelegramYAML(t *testing.T, slug, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "paseka", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "telegram.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsMissingConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	_, err := tggate.Load("missing-slug")
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoadRejectsDisabled(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: false
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
`)
	_, err := tggate.Load("tg-test")
	if err == nil {
		t.Fatal("expected error for disabled gate")
	}
}

func TestLoadRejectsEmptyAllowFrom(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "tok"
allow_from: []
chat_ids: [-1]
`)
	_, err := tggate.Load("tg-test")
	if err == nil {
		t.Fatal("expected error for empty allow_from")
	}
}

func TestLoadRejectsEmptyChatIDs(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: []
`)
	_, err := tggate.Load("tg-test")
	if err == nil {
		t.Fatal("expected error for empty chat_ids")
	}
}

func TestLoadAppliesCommandDefaults(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
`)
	cfg, err := tggate.Load("tg-test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Commands.DefaultBee != "builder" {
		t.Fatalf("default_bee = %q", cfg.Commands.DefaultBee)
	}
	if cfg.Commands.DefaultIntent != "general" {
		t.Fatalf("default_intent = %q", cfg.Commands.DefaultIntent)
	}
	if cfg.Commands.DefaultReview != "none" {
		t.Fatalf("default_review = %q", cfg.Commands.DefaultReview)
	}
	if !cfg.Commands.AutorunEnabled() {
		t.Fatal("expected task_autorun default true")
	}
}

func TestLoadAcceptsCustomDefaultIntent(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "tok"
allow_from: [1]
chat_ids: [-1]
commands:
  default_intent: feature
`)
	cfg, err := tggate.Load("tg-test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Commands.DefaultIntent != "feature" {
		t.Fatalf("default_intent = %q", cfg.Commands.DefaultIntent)
	}
}

func TestLoadAcceptsValidConfig(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "yaml-token"
mode: longpoll
allow_from: [123]
chat_ids: [-1001]
`)
	cfg, err := tggate.Load("tg-test")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled {
		t.Fatal("expected enabled")
	}
	if cfg.BotToken() != "yaml-token" {
		t.Fatalf("token = %q", cfg.BotToken())
	}
	if !cfg.LongPoll() {
		t.Fatal("expected longpoll mode")
	}
}

func TestBotTokenEnvOverride(t *testing.T) {
	writeTelegramYAML(t, "tg-test", `enabled: true
bot_token: "yaml-token"
allow_from: [1]
chat_ids: [-1]
`)
	t.Setenv("PASEKA_TELEGRAM_BOT_TOKEN", "env-token")
	cfg, err := tggate.Load("tg-test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BotToken() != "env-token" {
		t.Fatalf("token = %q, want env-token", cfg.BotToken())
	}
}
