package telegram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

const (
	envBotToken     = "PASEKA_TELEGRAM_BOT_TOKEN"
	modeLongPoll    = "longpoll"
	modeWebhook     = "webhook"
	callbackRefresh = "gate:status:refresh"
)

// Config is machine-local Telegram Human Gateway settings (~/.config/paseka/<slug>/telegram.yaml).
type Config struct {
	Enabled        bool           `yaml:"enabled"`
	Token          string         `yaml:"bot_token"`
	Mode           string         `yaml:"mode"`
	AllowFrom      []int64        `yaml:"allow_from"`
	ChatIDs        []int64        `yaml:"chat_ids"`
	ConsoleBaseURL string         `yaml:"console_base_url"`
	Notify         NotifyConfig   `yaml:"notify"`
	Commands       CommandsConfig `yaml:"commands"`
	Webhook        WebhookConfig  `yaml:"webhook"`
}

// NotifyConfig toggles outbound push categories (used by later slices).
type NotifyConfig struct {
	Invites       bool `yaml:"invites"`
	WaitingReview bool `yaml:"waiting_review"`
	Blocked       bool `yaml:"blocked"`
	Failed        bool `yaml:"failed"`
}

// CommandsConfig holds /task defaults for later slices.
type CommandsConfig struct {
	DefaultBee    string `yaml:"default_bee"`
	DefaultReview string `yaml:"default_review"`
	TaskAutorun   *bool  `yaml:"task_autorun"`
}

// WebhookConfig is optional webhook transport settings.
type WebhookConfig struct {
	Listen string `yaml:"listen"`
	URL    string `yaml:"url"`
}

// ConfigPath returns ~/.config/paseka/<slug>/telegram.yaml.
func ConfigPath(slug string) (string, error) {
	homeDir, err := colony.HomeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, "telegram.yaml"), nil
}

// Load reads and validates telegram.yaml for the colony slug.
func Load(slug string) (Config, error) {
	path, err := ConfigPath(slug)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, fmt.Errorf("telegram gate: missing %s (create it to enable paseka gate telegram)", path)
		}
		return Config{}, fmt.Errorf("telegram gate: read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("telegram gate: parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Mode == "" {
		c.Mode = modeLongPoll
	}
	if c.Commands.DefaultBee == "" {
		c.Commands.DefaultBee = "builder"
	}
	if c.Commands.DefaultReview == "" {
		c.Commands.DefaultReview = "none"
	}
	if c.Commands.TaskAutorun == nil {
		v := true
		c.Commands.TaskAutorun = &v
	}
}

func (c *Config) validate() error {
	if !c.Enabled {
		return fmt.Errorf("telegram gate: disabled in telegram.yaml (set enabled: true)")
	}
	if strings.TrimSpace(c.BotToken()) == "" {
		return fmt.Errorf("telegram gate: bot_token is required (or set %s)", envBotToken)
	}
	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	if mode != modeLongPoll && mode != modeWebhook {
		return fmt.Errorf("telegram gate: mode must be %q or %q, got %q", modeLongPoll, modeWebhook, c.Mode)
	}
	if len(c.AllowFrom) == 0 {
		return fmt.Errorf("telegram gate: allow_from must be non-empty")
	}
	if len(c.ChatIDs) == 0 {
		return fmt.Errorf("telegram gate: chat_ids must be non-empty")
	}
	return nil
}

// BotToken returns the configured token, preferring PASEKA_TELEGRAM_BOT_TOKEN.
func (c Config) BotToken() string {
	if v := strings.TrimSpace(os.Getenv(envBotToken)); v != "" {
		return v
	}
	return strings.TrimSpace(c.Token)
}

// LongPoll reports whether the gate should use getUpdates long-polling.
func (c Config) LongPoll() bool {
	return strings.EqualFold(strings.TrimSpace(c.Mode), modeLongPoll)
}
