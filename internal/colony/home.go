package colony

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/paseka/paseka/internal/gitroot"
	"gopkg.in/yaml.v3"
)

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

// CursorAdapterConfig is ~/.config/paseka/<slug>/adapters/cursor.yaml.
type CursorAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`
}

// Context binds project-local colony config with machine-local home config.
type Context struct {
	ColonyRoot string
	Slug       string
	Home       HomeConfig
	Cursor     CursorAdapterConfig
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

	return Context{
		ColonyRoot: colonyRoot,
		Slug:       slug,
		Home:       home,
		Cursor:     cursor,
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
