package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/gitroot"
	"gopkg.in/yaml.v3"
)

const envNATSURL = "PASEKA_NATS_URL"

// HomeConfig is machine-local colony state under ~/.config/paseka/<slug>/.
type HomeConfig struct {
	ColonyRoot string         `yaml:"colony_root"`
	Slug       string         `yaml:"slug"`
	NATS       NATSConfig     `yaml:"nats"`
	Adapters   map[string]any `yaml:"adapters"`
}

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	URL string `yaml:"url"`
}

// EffectiveURL returns the NATS server URL, preferring PASEKA_NATS_URL over config.
func (c NATSConfig) EffectiveURL() string {
	if v := strings.TrimSpace(os.Getenv(envNATSURL)); v != "" {
		return v
	}
	return strings.TrimSpace(c.URL)
}

// CursorAdapterConfig is ~/.config/paseka/<slug>/adapters/cursor.yaml.
type CursorAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// PiAdapterConfig is ~/.config/paseka/<slug>/adapters/pi.yaml.
type PiAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// ClaudeAdapterConfig is ~/.config/paseka/<slug>/adapters/claude.yaml.
type ClaudeAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// Context binds project-local colony config with machine-local home config.
type Context struct {
	ColonyRoot string
	Slug       string
	Home       HomeConfig
	Cursor     CursorAdapterConfig
	Pi         PiAdapterConfig
	Claude     ClaudeAdapterConfig
}

// ResolveContext finds the git repo, loads colony + home config.
func ResolveContext(startDir string) (Context, error) {
	if startDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Context{}, err
		}
		startDir = wd
	}

	colonyRoot, err := gitroot.Find(startDir)
	if err != nil {
		return Context{}, err
	}

	manifest, err := LoadColony(colonyRoot)
	if err != nil {
		return Context{}, err
	}
	if _, err := os.Stat(filepath.Join(colonyRoot, pasekaDir, "colony.yaml")); err != nil {
		return Context{}, fmt.Errorf("colony: not initialized at %s (run paseka init)", colonyRoot)
	}

	origin, _ := gitroot.OriginURL(colonyRoot)
	slug := ResolveSlug(colonyRoot, manifest, origin)
	if slug == "" {
		return Context{}, fmt.Errorf("colony: missing slug (run paseka init)")
	}

	home, err := LoadHomeConfig(slug)
	if err != nil {
		return Context{}, fmt.Errorf("colony: home config: %w (run paseka init)", err)
	}
	if home.ColonyRoot != "" {
		homeRoot, err := filepath.Abs(home.ColonyRoot)
		if err != nil {
			return Context{}, err
		}
		if homeRoot != colonyRoot {
			return Context{}, fmt.Errorf("colony: home config points to %q, but repo is %q", homeRoot, colonyRoot)
		}
	}

	cursor, err := LoadCursorAdapter(slug)
	if err != nil {
		return Context{}, err
	}

	pi, err := LoadPiAdapter(slug)
	if err != nil {
		return Context{}, err
	}

	claude, err := LoadClaudeAdapter(slug)
	if err != nil {
		return Context{}, err
	}

	return Context{
		ColonyRoot: colonyRoot,
		Slug:       slug,
		Home:       home,
		Cursor:     cursor,
		Pi:         pi,
		Claude:     claude,
	}, nil
}

// LoadHomeConfig reads ~/.config/paseka/<slug>/config.yaml.
func LoadHomeConfig(slug string) (HomeConfig, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return HomeConfig{}, err
	}
	path := filepath.Join(homeDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HomeConfig{}, fmt.Errorf("missing %s", path)
		}
		return HomeConfig{}, err
	}
	var cfg HomeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return HomeConfig{}, fmt.Errorf("parse home config: %w", err)
	}
	if cfg.Slug == "" {
		cfg.Slug = slug
	}
	return cfg, nil
}

// LoadCursorAdapter reads ~/.config/paseka/<slug>/adapters/cursor.yaml.
func LoadCursorAdapter(slug string) (CursorAdapterConfig, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return CursorAdapterConfig{}, err
	}
	path := filepath.Join(homeDir, "adapters", "cursor.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return CursorAdapterConfig{Binary: "agent", APIKeyEnv: "CURSOR_API_KEY"}, nil
		}
		return CursorAdapterConfig{}, err
	}
	var cfg CursorAdapterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return CursorAdapterConfig{}, fmt.Errorf("parse cursor adapter config: %w", err)
	}
	if cfg.Binary == "" {
		cfg.Binary = "agent"
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "CURSOR_API_KEY"
	}
	return cfg, nil
}

// APIKey returns the Cursor API key from the configured environment variable.
func (c CursorAdapterConfig) APIKey() string {
	return os.Getenv(c.APIKeyEnv)
}

// LoadPiAdapter reads ~/.config/paseka/<slug>/adapters/pi.yaml.
func LoadPiAdapter(slug string) (PiAdapterConfig, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return PiAdapterConfig{}, err
	}
	path := filepath.Join(homeDir, "adapters", "pi.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return PiAdapterConfig{Binary: "pi"}, nil
		}
		return PiAdapterConfig{}, err
	}
	var cfg PiAdapterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return PiAdapterConfig{}, fmt.Errorf("parse pi adapter config: %w", err)
	}
	if cfg.Binary == "" {
		cfg.Binary = "pi"
	}
	return cfg, nil
}

// APIKey returns the Pi API key when api_key_env is configured and set in the environment.
func (c PiAdapterConfig) APIKey() string {
	if c.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.APIKeyEnv)
}

// LoadClaudeAdapter reads ~/.config/paseka/<slug>/adapters/claude.yaml.
func LoadClaudeAdapter(slug string) (ClaudeAdapterConfig, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return ClaudeAdapterConfig{}, err
	}
	path := filepath.Join(homeDir, "adapters", "claude.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ClaudeAdapterConfig{Binary: "claude", APIKeyEnv: "ANTHROPIC_API_KEY"}, nil
		}
		return ClaudeAdapterConfig{}, err
	}
	var cfg ClaudeAdapterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ClaudeAdapterConfig{}, fmt.Errorf("parse claude adapter config: %w", err)
	}
	if cfg.Binary == "" {
		cfg.Binary = "claude"
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	return cfg, nil
}

// APIKey returns the Claude API key from the configured environment variable.
// When empty, Claude Code falls back to its own stored subscription auth
// (claude login), so no key is forwarded.
func (c ClaudeAdapterConfig) APIKey() string {
	if c.APIKeyEnv == "" {
		return ""
	}
	return os.Getenv(c.APIKeyEnv)
}

// TerminalConfig is ~/.config/paseka/<slug>/terminal.yaml.
type TerminalConfig struct {
	Terminal      string `yaml:"terminal"`       // default | ghostty
	GhosttyBinary string `yaml:"ghostty_binary"` // default: ghostty
}

// LoadTerminalConfig reads terminal preferences for session attach.
func LoadTerminalConfig(slug string) TerminalConfig {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return TerminalConfig{Terminal: "default", GhosttyBinary: "ghostty"}
	}
	path := filepath.Join(homeDir, "terminal.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return TerminalConfig{Terminal: "default", GhosttyBinary: "ghostty"}
	}
	var cfg TerminalConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return TerminalConfig{Terminal: "default", GhosttyBinary: "ghostty"}
	}
	if cfg.Terminal == "" {
		cfg.Terminal = "default"
	}
	if cfg.GhosttyBinary == "" {
		cfg.GhosttyBinary = "ghostty"
	}
	return cfg
}
