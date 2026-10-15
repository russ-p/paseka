package colony

import (
	"fmt"
	"sort"
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
