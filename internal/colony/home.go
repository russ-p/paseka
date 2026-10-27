package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/gitroot"
	"gopkg.in/yaml.v3"
)

// EnvNATSURL overrides HomeConfig.NATS.URL when set.
const EnvNATSURL = "PASEKA_NATS_URL"

// HomeConfig is machine-local colony state under ~/.config/paseka/<slug>/.
type HomeConfig struct {
	ColonyRoot   string            `yaml:"colony_root"`
	Slug         string            `yaml:"slug"`
	Profile      string            `yaml:"profile,omitempty"`
	ModelAliases map[string]string `yaml:"model_aliases,omitempty"`
	NATS         NATSConfig        `yaml:"nats"`
	Adapters     map[string]any    `yaml:"adapters"`
	Forge        ForgeConfig       `yaml:"forge,omitempty"`
}

// ForgeConfig is the apiary-local pull-request hosting driver (not a bee adapter).
type ForgeConfig struct {
	Command []string `yaml:"command,omitempty"`
}

// NATSConfig holds NATS connection settings.
type NATSConfig struct {
	URL string `yaml:"url"`
}

// EffectiveURL returns the NATS server URL, preferring PASEKA_NATS_URL over config.
func (c NATSConfig) EffectiveURL() string {
	if v := strings.TrimSpace(os.Getenv(EnvNATSURL)); v != "" {
		return v
	}
	return strings.TrimSpace(c.URL)
}

// CursorAdapterConfig is ~/.config/paseka/<slug>/adapters/cursor.yaml.
type CursorAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`

	binaryDeclared bool
}

// BinaryDeclared reports whether the effective config set `binary`, as opposed
// to inheriting the loader default. It keeps a default from masquerading as a
// declaration: the merge treats a declared machine binary as a fallback and an
// undeclared one as absent, leaving the adapter's own default in place.
func (c CursorAdapterConfig) BinaryDeclared() bool { return c.binaryDeclared }

// PiAdapterConfig is ~/.config/paseka/<slug>/adapters/pi.yaml.
type PiAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`

	binaryDeclared bool
}

// BinaryDeclared reports whether the effective config set `binary`.
func (c PiAdapterConfig) BinaryDeclared() bool { return c.binaryDeclared }

// ClaudeAdapterConfig is ~/.config/paseka/<slug>/adapters/claude.yaml.
type ClaudeAdapterConfig struct {
	Binary    string `yaml:"binary"`
	APIKeyEnv string `yaml:"api_key_env"`

	binaryDeclared bool
}

// BinaryDeclared reports whether the effective config set `binary`.
func (c ClaudeAdapterConfig) BinaryDeclared() bool { return c.binaryDeclared }

// OpenCodeAdapterConfig is ~/.config/paseka/<slug>/adapters/opencode.yaml.
//
// The OpenCode family is one machine-local file: `opencode` is the version
// proxy, and `opencodev1` / `opencodev2` are explicit pins. version, v1_binary,
// v2_binary, and detect_timeout are optional; existing files with only binary
// stay valid.
type OpenCodeAdapterConfig struct {
	Binary        string `yaml:"binary"`
	Version       string `yaml:"version"`
	V1Binary      string `yaml:"v1_binary"`
	V2Binary      string `yaml:"v2_binary"`
	DetectTimeout string `yaml:"detect_timeout"`

	binaryDeclared        bool
	versionDeclared       bool
	v1BinaryDeclared      bool
	v2BinaryDeclared      bool
	detectTimeoutDeclared bool
}

// BinaryDeclared reports whether the effective config set `binary`.
func (c OpenCodeAdapterConfig) BinaryDeclared() bool { return c.binaryDeclared }

// Options returns the adapter-specific settings the file actually declared.
// Only declared keys travel, so a machine file can act as a default without
// clobbering a bee's params.options.
func (c OpenCodeAdapterConfig) Options() map[string]string {
	opts := map[string]string{}
	if c.versionDeclared {
		opts["version"] = c.Version
	}
	if c.v1BinaryDeclared {
		opts["v1_binary"] = c.V1Binary
	}
	if c.v2BinaryDeclared {
		opts["v2_binary"] = c.V2Binary
	}
	if c.detectTimeoutDeclared {
		opts["detect_timeout"] = c.DetectTimeout
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

// Context binds project-local colony config with machine-local home config.
type Context struct {
	ColonyRoot    string
	Slug          string
	Home          HomeConfig
	ModelAliases  map[string]string
	Cursor        CursorAdapterConfig
	Pi            PiAdapterConfig
	Claude        ClaudeAdapterConfig
	OpenCode      OpenCodeAdapterConfig
	Profile       string
	ProfileLayers ProfileLayers
	colonyProfile ColonyProfile
	// ConfigRoot, when set, is the materialized configuration tree an execution
	// is bound to. It replaces ColonyRoot for configuration-file reads only.
	ConfigRoot     string
	ConfigRef      string
	ConfigRevision int
	WorkspaceRoot  string
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

	opencode, err := LoadOpenCodeAdapter(slug)
	if err != nil {
		return Context{}, err
	}

	merged := MergedModelAliases(manifest.ModelAliases, home.ModelAliases)
	if err := ValidateModelAliases(merged); err != nil {
		return Context{}, err
	}

	ctx := Context{
		ColonyRoot:   colonyRoot,
		Slug:         slug,
		Home:         home,
		ModelAliases: merged,
		Cursor:       cursor,
		Pi:           pi,
		Claude:       claude,
		OpenCode:     opencode,
	}

	name, err := selectedProfileName(ProcessProfile(), home.Profile)
	if err != nil {
		return Context{}, err
	}
	if name != "" {
		if err := applySelectedProfile(&ctx, manifest, name); err != nil {
			return Context{}, err
		}
	}

	return ctx, nil
}

// EnrichFromHome fills machine-local Home (including NATS URL) when callers
// passed only ColonyRoot/Slug. If home config cannot be loaded, ctx is returned unchanged.
func EnrichFromHome(ctx Context) Context {
	if ctx.ColonyRoot == "" || ctx.Home.NATS.EffectiveURL() != "" {
		return ctx
	}
	resolved, err := ResolveContext(ctx.ColonyRoot)
	if err != nil {
		return ctx
	}
	return resolved
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
	cfg.ModelAliases = NormalizeModelAliases(cfg.ModelAliases)
	if err := ValidateModelAliases(cfg.ModelAliases); err != nil {
		return HomeConfig{}, err
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
	cfg.binaryDeclared = strings.TrimSpace(cfg.Binary) != ""
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
	cfg.binaryDeclared = strings.TrimSpace(cfg.Binary) != ""
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
	cfg.binaryDeclared = strings.TrimSpace(cfg.Binary) != ""
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

// LoadOpenCodeAdapter reads ~/.config/paseka/<slug>/adapters/opencode.yaml.
func LoadOpenCodeAdapter(slug string) (OpenCodeAdapterConfig, error) {
	homeDir, err := HomeDir(slug)
	if err != nil {
		return OpenCodeAdapterConfig{}, err
	}
	path := filepath.Join(homeDir, "adapters", "opencode.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return OpenCodeAdapterConfig{Binary: "opencode"}, nil
		}
		return OpenCodeAdapterConfig{}, err
	}
	var cfg OpenCodeAdapterConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return OpenCodeAdapterConfig{}, fmt.Errorf("parse opencode adapter config: %w", err)
	}
	cfg.binaryDeclared = strings.TrimSpace(cfg.Binary) != ""
	cfg.versionDeclared = strings.TrimSpace(cfg.Version) != ""
	cfg.v1BinaryDeclared = strings.TrimSpace(cfg.V1Binary) != ""
	cfg.v2BinaryDeclared = strings.TrimSpace(cfg.V2Binary) != ""
	cfg.detectTimeoutDeclared = strings.TrimSpace(cfg.DetectTimeout) != ""
	if cfg.Binary == "" {
		cfg.Binary = "opencode"
	}
	if cfg.Version == "" {
		cfg.Version = "auto"
	}
	return cfg, nil
}

// AdapterConfigDeclared reports whether ~/.config/paseka/<slug>/adapters/<name>.yaml
// exists and which top-level keys it actually sets.
//
// Every adapter loader fills a default for an absent key, so a loaded struct
// cannot say where a value came from. A view that needs to attribute one — to
// tell an operator that the api_key_env they see is the loader's default rather
// than something they wrote — has to ask the file. A file that exists but does
// not parse is reported as absent: the loader will have failed loudly on its
// own, and this must not become a second, quieter error.
func AdapterConfigDeclared(slug, name string) (exists bool, keys map[string]bool) {
	keys = map[string]bool{}
	homeDir, err := HomeDir(slug)
	if err != nil {
		return false, keys
	}
	data, err := os.ReadFile(filepath.Join(homeDir, "adapters", name+".yaml"))
	if err != nil {
		return false, keys
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, keys
	}
	for key := range raw {
		keys[key] = true
	}
	return true, keys
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
