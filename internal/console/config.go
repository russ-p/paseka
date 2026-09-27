package console

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/gate/telegram"
)

// Sources a ConfigValue can name. They are display-ready strings because the
// only question a view asks of one is "where would I have to look to change
// this", and an operator reads `env:PASEKA_NATS_URL` faster than a bare `env`.
const (
	configSourceUnset   = "unset"
	configSourceEnv     = "env"
	configSourceHome    = "config.yaml"
	configSourceColony  = "colony.yaml"
	configSourceDefault = "default"
)

// ConfigValue is one setting's effective value together with the source that
// decides it.
//
// The source is not decoration. NATSConfig.EffectiveURL prefers
// PASEKA_NATS_URL over the home config, so a console that showed only the value
// would report one URL while the process used another, and a settings form that
// wrote the file would look successful while changing nothing. This is the same
// rule the bees roster and the worktrees inventory applied: the client is never
// left to infer a state the server did not send.
type ConfigValue struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// ConfigAdapterView describes one adapter's machine-local settings. The API key
// itself is never sent — only the variable name and whether that variable
// resolves, because the credential model stores a reference and reads the value
// from the environment at call time (colony.CursorAdapterConfig.APIKey).
type ConfigAdapterView struct {
	Name string `json:"name"`
	// Binary is the executable the adapter launches.
	Binary ConfigValue `json:"binary"`
	// APIKeyEnv is the variable the adapter reads its key from, empty when the
	// adapter has none.
	APIKeyEnv ConfigValue `json:"apiKeyEnv"`
	// APIKeySet reports whether APIKeyEnv resolves to a value right now.
	APIKeySet bool `json:"apiKeySet"`
	// Configured reports whether adapters/<name>.yaml exists, as opposed to
	// every value here having been inferred from the loader's defaults.
	Configured bool `json:"configured"`
}

// ConfigNotifyView is one outbound push category and the mode it delivers in.
type ConfigNotifyView struct {
	Category string `json:"category"`
	Mode     string `json:"mode"`
}

// ConfigTelegramView describes the human gateway's push preferences.
//
// It reports presence separately from enabled because Load treats a missing file
// and `enabled: false` as errors: a settings page has to be able to say the gate
// is off, and reading it through Load would render that as a failed read.
type ConfigTelegramView struct {
	Present        bool               `json:"present"`
	Enabled        bool               `json:"enabled"`
	Mode           ConfigValue        `json:"mode"`
	BotTokenSet    bool               `json:"botTokenSet"`
	BotTokenEnv    string             `json:"botTokenEnv"`
	AllowFrom      []int64            `json:"allowFrom"`
	ChatIDs        []int64            `json:"chatIds"`
	ConsoleBaseURL string             `json:"consoleBaseUrl,omitempty"`
	Notify         []ConfigNotifyView `json:"notify"`
}

// ConfigTerminalView describes how an interactive session is attached.
type ConfigTerminalView struct {
	Terminal      string `json:"terminal"`
	GhosttyBinary string `json:"ghosttyBinary"`
	// Configured reports whether terminal.yaml exists; LoadTerminalConfig answers
	// with defaults for every failure, so this is the only way to tell a chosen
	// terminal from the fallback.
	Configured bool `json:"configured"`
}

// ConfigProfileLayersView names which profile files actually loaded for the
// process. It is its own type rather than a colony.ProfileLayers, because that
// struct carries no JSON tags and would marshal as `Colony`/`Home` — the only
// place in this view that would not be camelCase like the rest of the API.
type ConfigProfileLayersView struct {
	Colony bool `json:"colony"`
	Home   bool `json:"home"`
}

// ConfigProfileView describes the selected profile and the alternatives on disk.
type ConfigProfileView struct {
	Selected ConfigValue             `json:"selected"`
	Colony   []string                `json:"colony"`
	Home     []string                `json:"home"`
	Layers   ConfigProfileLayersView `json:"layers"`
}

// ConfigNATSView describes the transport the console and every bee connect with.
type ConfigNATSView struct {
	URL ConfigValue `json:"url"`
	// SubjectPrefix is committed config, so it is read-only in the console even
	// though the URL beside it is not.
	SubjectPrefix ConfigValue `json:"subjectPrefix"`
}

// ConfigView is the effective colony configuration behind /next/settings.
type ConfigView struct {
	Slug       string              `json:"slug"`
	ColonyRoot string              `json:"colonyRoot"`
	Profile    ConfigProfileView   `json:"profile"`
	NATS       ConfigNATSView      `json:"nats"`
	Adapters   []ConfigAdapterView `json:"adapters"`
	Telegram   ConfigTelegramView  `json:"telegram"`
	Terminal   ConfigTerminalView  `json:"terminal"`
}

// GetConfig reports the settings a console operator can see, each with the
// source that decides it.
//
// It reports configured values and their provenance, not liveness. Whether NATS
// is currently connected belongs to the topbar's chrome stream and the dashboard
// poll, and a second reader of it here would be the same duplicate poller the
// store contract forbids.
func GetConfig(ctx colony.Context) (ConfigView, error) {
	slug := ctx.Slug
	view := ConfigView{
		Slug:       slug,
		ColonyRoot: ctx.ColonyRoot,
		Adapters:   adapterViews(ctx),
	}

	view.NATS = natsView(ctx)
	view.Profile = profileView(ctx, slug)

	view.Terminal = terminalView(slug)

	telegramView, err := telegramConfigView(slug)
	if err != nil {
		return ConfigView{}, err
	}
	view.Telegram = telegramView

	return view, nil
}

func natsView(ctx colony.Context) ConfigNATSView {
	homeURL := strings.TrimSpace(ctx.Home.NATS.URL)
	url := ConfigValue{Value: homeURL, Source: configSourceHome}
	if env := strings.TrimSpace(os.Getenv(colony.EnvNATSURL)); env != "" {
		url = ConfigValue{Value: env, Source: configSourceEnv + ":" + colony.EnvNATSURL}
	} else if homeURL == "" {
		url.Source = configSourceUnset
	}

	prefix := ConfigValue{Source: configSourceDefault}
	if manifest, err := colony.LoadColony(ctx.ColonyRoot); err == nil {
		if declared := strings.TrimSpace(manifest.NATS.SubjectPrefix); declared != "" {
			prefix = ConfigValue{Value: declared, Source: configSourceColony}
		}
	}
	if prefix.Source == configSourceDefault {
		prefix.Value = "paseka." + ctx.Slug
	}
	return ConfigNATSView{URL: url, SubjectPrefix: prefix}
}

func profileView(ctx colony.Context, slug string) ConfigProfileView {
	selected := ConfigValue{Value: strings.TrimSpace(ctx.Profile), Source: configSourceUnset}
	if selected.Value != "" {
		selected.Source = configSourceHome
		// A --profile flag outranks the environment, and the environment outranks
		// the home file's sticky `profile:`, so the two overrides are named
		// separately — otherwise the page would attribute the profile to the file
		// while the process was actually started with something else.
		if sel := colony.ProcessProfile(); sel.FlagSet && sel.Name != "" {
			selected.Source = "flag"
		} else if strings.TrimSpace(os.Getenv(colony.EnvProfile)) != "" {
			selected.Source = configSourceEnv + ":" + colony.EnvProfile
		}
	}

	// A colony with no profiles is an empty list, never a null: a `null` here is
	// what a client has to special-case, and the absence of a profile directory
	// is a fact rather than a missing value.
	colonyNames, homeNames, err := colony.ListProfileNames(ctx.ColonyRoot, slug)
	if err != nil {
		// A profile directory that cannot be listed is not a reason to fail the
		// whole read: the selected profile is still reported, and the page says
		// the alternatives are unavailable rather than showing none exist.
		colonyNames, homeNames = nil, nil
	}
	if colonyNames == nil {
		colonyNames = []string{}
	}
	if homeNames == nil {
		homeNames = []string{}
	}
	return ConfigProfileView{
		Selected: selected,
		Colony:   colonyNames,
		Home:     homeNames,
		Layers: ConfigProfileLayersView{
			Colony: ctx.ProfileLayers.Colony,
			Home:   ctx.ProfileLayers.Home,
		},
	}
}

func terminalView(slug string) ConfigTerminalView {
	cfg := colony.LoadTerminalConfig(slug)
	view := ConfigTerminalView{
		Terminal:      cfg.Terminal,
		GhosttyBinary: cfg.GhosttyBinary,
	}
	// LoadTerminalConfig answers with defaults for every failure, so the file
	// having to exist is the only thing that separates a chosen terminal from
	// the fallback.
	if homeDir, err := colony.HomeDir(slug); err == nil {
		if _, statErr := os.Stat(filepath.Join(homeDir, "terminal.yaml")); statErr == nil {
			view.Configured = true
		}
	}
	return view
}

func telegramConfigView(slug string) (ConfigTelegramView, error) {
	// Every list is non-nil from the start, so the absent-gate path below cannot
	// hand a client three nulls. "None configured" is an empty list, not a shape
	// to special-case, and the absent path returns before anything fills them.
	view := ConfigTelegramView{
		BotTokenEnv: telegram.EnvBotToken,
		Mode:        ConfigValue{Source: configSourceUnset},
		AllowFrom:   []int64{},
		ChatIDs:     []int64{},
		Notify:      []ConfigNotifyView{},
	}
	cfg, present, err := telegram.Inspect(slug)
	if err != nil {
		return ConfigTelegramView{}, err
	}
	view.Present = present
	if !present {
		return view, nil
	}

	mode := strings.TrimSpace(cfg.Mode)
	view.Enabled = cfg.Enabled
	view.Mode = ConfigValue{Value: mode, Source: configSourceHome}
	// A token is environment-first but the file counts too — the env var is an
	// override, not the only way to be configured. Either way the token itself
	// stays on the server and only its presence is reported.
	view.BotTokenSet = strings.TrimSpace(cfg.BotToken()) != ""
	view.AllowFrom = append(view.AllowFrom, cfg.AllowFrom...)
	view.ChatIDs = append(view.ChatIDs, cfg.ChatIDs...)
	view.ConsoleBaseURL = strings.TrimSpace(cfg.ConsoleBaseURL)

	for _, cat := range telegram.OrderedNotifyCategories {
		view.Notify = append(view.Notify, ConfigNotifyView{
			Category: telegram.CategoryKey(cat),
			Mode:     string(cfg.Notify.Mode(cat)),
		})
	}
	return view, nil
}

func adapterViews(ctx colony.Context) []ConfigAdapterView {
	views := []ConfigAdapterView{
		adapterView(ctx.Slug, "cursor", ctx.Cursor.Binary, ctx.Cursor.APIKeyEnv),
		adapterView(ctx.Slug, "pi", ctx.Pi.Binary, ctx.Pi.APIKeyEnv),
		adapterView(ctx.Slug, "claude", ctx.Claude.Binary, ctx.Claude.APIKeyEnv),
		adapterView(ctx.Slug, "opencode", ctx.OpenCode.Binary, ""),
	}
	return views
}

func adapterView(slug, name, binary, apiKeyEnv string) ConfigAdapterView {
	present, declared := colony.AdapterConfigDeclared(slug, name)

	env := ConfigValue{Value: strings.TrimSpace(apiKeyEnv), Source: configSourceDefault}
	switch {
	case env.Value == "":
		env.Source = configSourceUnset
	case declared["api_key_env"]:
		env.Source = configSourceHome
	}

	binarySource := configSourceDefault
	if declared["binary"] {
		binarySource = configSourceHome
	}

	return ConfigAdapterView{
		Name: name,
		Binary: ConfigValue{
			Value:  binary,
			Source: binarySource,
		},
		APIKeyEnv: env,
		APIKeySet: env.Value != "" && strings.TrimSpace(os.Getenv(env.Value)) != "",
		// Present is reported beside the sources rather than replacing them: it
		// answers whether the operator has a file to edit at all.
		Configured: present,
	}
}
