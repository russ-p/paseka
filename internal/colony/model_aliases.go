package colony

import (
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
)

// MergedModelAliases overlays home aliases onto colony aliases (home wins on key collision).
func MergedModelAliases(colonyMap, homeMap map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range colonyMap {
		out[k] = v
	}
	for k, v := range homeMap {
		out[k] = v
	}
	return out
}

// ValidateModelID accepts a vendor-qualified id (`vendor/model[:suffix]`) or a
// bare id without a vendor. It rejects only structurally empty forms, so a typo
// like a leading slash fails at load rather than becoming a silent no-op.
func ValidateModelID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("empty model id")
	}
	vendor, rest, hasSlash := strings.Cut(id, "/")
	if !hasSlash {
		return nil
	}
	if vendor == "" {
		return fmt.Errorf("model id %q: empty vendor before %q", id, "/")
	}
	if rest == "" {
		return fmt.Errorf("model id %q: empty model after %q", id, "/")
	}
	return nil
}

// ValidateModelAliases rejects empty keys/values and alias values that match another key.
func ValidateModelAliases(aliases map[string]string) error {
	if len(aliases) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(aliases))
	for k, v := range aliases {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" {
			return fmt.Errorf("colony: model_aliases: empty alias key")
		}
		if val == "" {
			return fmt.Errorf("colony: model_aliases: empty value for alias %q", key)
		}
		if err := ValidateModelID(val); err != nil {
			return fmt.Errorf("colony: model_aliases: alias %q: %w", key, err)
		}
		normalized[key] = val
	}
	for key, val := range normalized {
		if _, isAlias := normalized[val]; isAlias {
			return fmt.Errorf("colony: model_aliases: value %q for alias %q must be a vendor id, not another alias", val, key)
		}
	}
	return nil
}

// NormalizeModelAliases trims keys and values in place into a new map.
func NormalizeModelAliases(raw map[string]string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" && val == "" {
			continue
		}
		out[key] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResolveModel maps a bee params.model through the merged alias table.
// ok is true when name was an alias key and was substituted.
func ResolveModel(name string, aliases map[string]string) (resolved string, ok bool) {
	name = strings.TrimSpace(name)
	if name == "" || len(aliases) == 0 {
		return name, false
	}
	if v, found := aliases[name]; found {
		return v, true
	}
	return name, false
}

// ApplyModelAliases substitutes params.Model when it matches a merged alias key.
func ApplyModelAliases(params *adapters.RunParams, aliases map[string]string) {
	if params == nil || params.Model == "" || len(aliases) == 0 {
		return
	}
	if resolved, ok := ResolveModel(params.Model, aliases); ok {
		params.Model = resolved
	}
}
