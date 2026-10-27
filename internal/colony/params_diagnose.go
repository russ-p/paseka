package colony

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// ParamDeprecations maps each deprecated bee `params` key to the semantic key
// that replaces it. The legacy keys still work for one release; doctor warns so
// operators migrate before they are removed.
var ParamDeprecations = map[string]string{
	"trust":    "yolo",
	"force":    "yolo",
	"thinking": "reasoning",
}

// adapterYoloSupport lists adapters that map params.yolo onto a CLI flag.
var adapterYoloSupport = map[string]bool{
	"cursor":     true,
	"claude":     true,
	"opencode":   true,
	"opencodev1": true,
	"opencodev2": true,
}

// adapterReasoningSupport lists adapters that map params.reasoning onto a flag.
var adapterReasoningSupport = map[string]bool{
	"pi":         true,
	"opencode":   true,
	"opencodev1": true,
	"opencodev2": true,
}

// AdapterMapsReasoning reports whether the adapter translates params.reasoning
// into a CLI flag. It is the one source `doctor` and `doctor --explain` share,
// so a value cursor/claude would drop is named rather than silently ignored.
func AdapterMapsReasoning(adapter string) bool {
	return adapterReasoningSupport[adapter]
}

// AdapterMapsYolo reports whether the adapter translates params.yolo into a CLI
// flag; pi has no equivalent.
func AdapterMapsYolo(adapter string) bool {
	return adapterYoloSupport[adapter]
}

// reasoningValue returns the declared reasoning, following the one-release
// `thinking` deprecation alias, and whether any was declared.
func reasoningValue(params map[string]any) (string, bool) {
	if v, ok := stringParam(params, "reasoning"); ok {
		return v, true
	}
	return stringParam(params, "thinking")
}

// DiagnoseBeeParams returns operator-facing warnings for bee params that are
// deprecated, unknown, or that the bee's adapter does not map to a flag. Doctor
// loads bees leniently so these leftovers are reported as fixes rather than
// failing the whole check before it can name them. Roles are visited in sorted
// order and keys within a bee are sorted so doctor output is stable.
func DiagnoseBeeParams(bees map[string]Bee) []string {
	roles := make([]string, 0, len(bees))
	for role := range bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	var warnings []string
	for _, role := range roles {
		bee := bees[role]
		name := bee.Role
		if name == "" {
			name = role
		}
		adapter, err := bee.ResolveAdapter()
		if err != nil {
			continue
		}
		warnings = append(warnings, invalidParamWarnings(name, bee.Params)...)
		warnings = append(warnings, deprecatedParamWarnings(name, bee.Params)...)
		if _, ok := bee.Params["provider"]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.provider is deprecated; put the vendor in params.model (e.g. model: <vendor>/<model>)", name))
		}
		if _, ok := bee.Params["output_format"]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.output_format is deprecated; the runtime owns structured output", name))
		}
		if _, ok := reasoningValue(bee.Params); ok && !AdapterMapsReasoning(adapter) {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.reasoning has no mapping on adapter %s (ignored)", name, adapter))
		}
		if _, ok := bee.Params["yolo"]; ok && !AdapterMapsYolo(adapter) {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: adapter %s has no yolo equivalent (params.yolo ignored)", name, adapter))
		}
	}
	return warnings
}

// deprecatedParamWarnings names each legacy param key the bee still carries and
// the semantic key that replaces it, so the migration is one edit.
func deprecatedParamWarnings(name string, params map[string]any) []string {
	var keys []string
	for key := range params {
		if _, ok := ParamDeprecations[key]; ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	warnings := make([]string, 0, len(keys))
	for _, key := range keys {
		warnings = append(warnings, fmt.Sprintf(
			"bee %q: params.%s is deprecated; use params.%s", name, key, ParamDeprecations[key]))
	}
	return warnings
}

// invalidParamWarnings names each param key the bee may no longer declare. A key
// outside RunParamKeys is reported as the typo it is; the RemovedRunParamKeys
// branch names the replacement once the window closes and the key leaves the
// valid set. Stays quiet about keys the schema accepts — the legacy keys
// included, which deprecatedParamWarnings covers while the window is open.
func invalidParamWarnings(name string, params map[string]any) []string {
	var keys []string
	for key := range params {
		if !isRunParamKey(key) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	warnings := make([]string, 0, len(keys))
	for _, key := range keys {
		if repl := RemovedRunParamKeys[key]; repl != "" {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.%s was removed; use params.%s", name, key, repl))
			continue
		}
		if _, ok := RemovedRunParamKeys[key]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.%s was removed (the runtime owns it now)", name, key))
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"bee %q: unknown params key %q", name, key))
	}
	return warnings
}

// DiagnoseAdapterCredentials warns when a machine-local adapter file explicitly
// declares `api_key_env` but the named host variable is unset. A credential that
// resolves to empty is then visible in doctor instead of being mistaken for
// valid auth.
func DiagnoseAdapterCredentials(ctx Context) []string {
	type check struct {
		name   string
		keyEnv string
	}
	checks := []check{
		{"cursor", ctx.Cursor.APIKeyEnv},
		{"pi", ctx.Pi.APIKeyEnv},
		{"claude", ctx.Claude.APIKeyEnv},
	}
	var warnings []string
	for _, c := range checks {
		keyEnv := strings.TrimSpace(c.keyEnv)
		if keyEnv == "" {
			continue
		}
		exists, keys := AdapterConfigDeclared(ctx.Slug, c.name)
		if !exists || !keys["api_key_env"] {
			continue
		}
		if strings.TrimSpace(os.Getenv(keyEnv)) == "" {
			warnings = append(warnings, fmt.Sprintf(
				"adapter %s: api_key_env %q is not set in the environment (credential resolves empty)",
				c.name, keyEnv))
		}
	}
	return warnings
}
