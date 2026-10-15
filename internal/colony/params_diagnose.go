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
	"cursor":   true,
	"claude":   true,
	"opencode": true,
}

// adapterReasoningSupport lists adapters that map params.reasoning onto a flag.
var adapterReasoningSupport = map[string]bool{
	"pi":       true,
	"opencode": true,
}

// DiagnoseBeeParams returns operator-facing warnings for bee params that are
// deprecated or that the bee's adapter does not map to a flag. Roles are
// visited in sorted order and keys within a bee are sorted so doctor output is
// stable.
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
		warnings = append(warnings, deprecatedParamWarnings(name, bee.Params)...)
		if _, ok := bee.Params["provider"]; ok {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.provider is deprecated; put the vendor in params.model (e.g. model: <vendor>/<model>)", name))
		}
		if _, ok := bee.Params["reasoning"]; ok && !adapterReasoningSupport[adapter] {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: params.reasoning has no mapping on adapter %s (ignored)", name, adapter))
		}
		if _, ok := bee.Params["yolo"]; ok && !adapterYoloSupport[adapter] {
			warnings = append(warnings, fmt.Sprintf(
				"bee %q: adapter %s has no yolo equivalent (params.yolo ignored)", name, adapter))
		}
	}
	return warnings
}

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
