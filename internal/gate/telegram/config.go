package telegram

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"gopkg.in/yaml.v3"
)

var customCommandNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)

var reservedTelegramCommands = map[string]struct{}{
	"start":   {},
	"status":  {},
	"help":    {},
	"invites": {},
	"energy":  {},
	"task":    {},
}

const (
	envBotToken     = "PASEKA_TELEGRAM_BOT_TOKEN"
	modeLongPoll    = "longpoll"
	modeWebhook     = "webhook"
	callbackRefresh = "gate:status:refresh"

	callbackInviteAccept        = "inv:a:"
	callbackInviteReject        = "inv:r:"
	callbackInviteDefer         = "inv:d:"
	callbackInviteConfirmAccept = "inv:ca:"
	callbackInviteConfirmReject = "inv:cr:"
	callbackInviteCancel        = "inv:x:"
	callbackEnergyAdd           = "en:+:"
	callbackTaskConfirm         = "task:c:"
	callbackTaskCancel          = "task:x:"
	callbackSignalConfirm       = "sig:c:"
	callbackSignalCancel        = "sig:x:"

	callbackProposalApprove        = "prop:a:"
	callbackProposalReject         = "prop:r:"
	callbackProposalConfirmApprove = "prop:ca:"
	callbackProposalConfirmReject  = "prop:cr:"
	callbackProposalCancel         = "prop:x:"
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

// NotifyConfig toggles outbound push categories.
type NotifyConfig struct {
	Invites       *bool `yaml:"invites"`
	WaitingReview *bool `yaml:"waiting_review"`
	Blocked       *bool `yaml:"blocked"`
	Failed        *bool `yaml:"failed"`
}

// InvitesEnabled reports whether pending invite push is on (default true).
func (n NotifyConfig) InvitesEnabled() bool {
	if n.Invites == nil {
		return true
	}
	return *n.Invites
}

// WaitingReviewEnabled reports whether waiting_review task push is on (default true).
func (n NotifyConfig) WaitingReviewEnabled() bool {
	if n.WaitingReview == nil {
		return true
	}
	return *n.WaitingReview
}

// BlockedEnabled reports whether blocked task push is on (default true).
func (n NotifyConfig) BlockedEnabled() bool {
	if n.Blocked == nil {
		return true
	}
	return *n.Blocked
}

// FailedEnabled reports whether failed task push is on (default true).
func (n NotifyConfig) FailedEnabled() bool {
	if n.Failed == nil {
		return true
	}
	return *n.Failed
}

// AutorunEnabled reports whether /task should publish task.ready after Confirm (default true).
func (c CommandsConfig) AutorunEnabled() bool {
	if c.TaskAutorun == nil {
		return true
	}
	return *c.TaskAutorun
}

// CommandsConfig holds /task defaults and custom emit commands.
type CommandsConfig struct {
	DefaultBee    string                         `yaml:"default_bee"`
	DefaultIntent string                         `yaml:"default_intent"`
	DefaultReview string                         `yaml:"default_review"`
	TaskAutorun   *bool                          `yaml:"task_autorun"`
	Custom        map[string]CustomCommandConfig `yaml:"custom"`
}

// CustomCommandConfig declares a Telegram slash command that publishes a bus event.
type CustomCommandConfig struct {
	Description string            `yaml:"description"`
	Emit        string            `yaml:"emit"`
	Type        string            `yaml:"type"`
	Kind        string            `yaml:"kind"`
	Static      map[string]string `yaml:"static,omitempty"`
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
	if err := cfg.Commands.validateCustom(); err != nil {
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
	if c.Commands.DefaultIntent == "" {
		c.Commands.DefaultIntent = "general"
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

func (c CommandsConfig) validateCustom() error {
	for name, cmd := range c.Custom {
		if _, reserved := reservedTelegramCommands[name]; reserved {
			return fmt.Errorf("telegram gate: commands.custom.%s: reserved command name", name)
		}
		if !customCommandNamePattern.MatchString(name) {
			return fmt.Errorf("telegram gate: commands.custom.%s: invalid command name (use lowercase letters, digits, underscore; max 32 chars)", name)
		}
		if strings.TrimSpace(cmd.Description) == "" {
			return fmt.Errorf("telegram gate: commands.custom.%s: description is required", name)
		}
		emit := strings.ToLower(strings.TrimSpace(cmd.Emit))
		if emit != "signal" {
			return fmt.Errorf("telegram gate: commands.custom.%s: emit must be %q", name, "signal")
		}
		typ := strings.ToUpper(strings.TrimSpace(cmd.Type))
		if typ != "SIGNAL" {
			return fmt.Errorf("telegram gate: commands.custom.%s: type must be SIGNAL", name)
		}
		if strings.TrimSpace(cmd.Kind) == "" {
			return fmt.Errorf("telegram gate: commands.custom.%s: kind is required", name)
		}
	}
	return nil
}

// CustomCommand returns a configured custom command by Telegram command name.
func (c CommandsConfig) CustomCommand(name string) (CustomCommandConfig, bool) {
	cmd, ok := c.Custom[name]
	return cmd, ok
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
