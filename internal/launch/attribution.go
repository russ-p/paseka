package launch

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
)

// Layer identifies the configuration layer that decided a resolved launch
// field. The set is the resolver's ladder, lowest precedence first, and it is
// deliberately closed: a UI that renders an unknown layer would be claiming a
// source the runtime does not have.
type Layer string

const (
	// LayerDefault is an adapter's built-in default — the value in force before
	// any file declares one. It is not a declaration, and saying otherwise is
	// what confused an operator into editing a file that did not control it.
	LayerDefault Layer = "default"
	// LayerBee is the committed bee YAML (with any selected profile already
	// folded in by LoadBee) — the primary description of the work.
	LayerBee Layer = "bee"
	// LayerMachine is the machine-local adapters/<name>.yaml layer.
	LayerMachine Layer = "machine"
	// LayerAlias is model-alias resolution, applied once after the merge.
	LayerAlias Layer = "alias"
)

// Field names the launch settings an operator can inspect. They are the
// semantic keys, so doctor and the Console speak the same words as bee-config.
const (
	FieldBinary    = "binary"
	FieldModel     = "model"
	FieldYolo      = "yolo"
	FieldPlan      = "plan"
	FieldReasoning = "reasoning"
	FieldProvider  = "provider"
	FieldEnv       = "env"
	FieldEnvMode   = "env_mode"
	FieldTimeout   = "timeout"
	FieldAPIKey    = "api_key"
)

// AttributionField is one resolved launch field: the value the resolver chose
// and the layer that chose it. Values are display-safe — a credential is
// reported as set/unset and env values are never printed, only their names.
type AttributionField struct {
	Field string
	Value string
	Layer Layer
}

// Attribution returns the resolved fields with their layer attribution in a
// stable operator-facing order. `paseka doctor --explain` and the Console
// adapter panel both render this, so a UI cannot describe a launch differently
// from the run.
func (p LaunchPlan) Attribution() []AttributionField {
	order := []struct {
		field string
		value string
	}{
		{FieldBinary, p.Binary},
		{FieldModel, p.Params.Model},
		{FieldYolo, yoloValue(p.Params)},
		{FieldPlan, boolValue(p.Params.Plan)},
		{FieldReasoning, dashIfEmpty(p.Params.Reasoning)},
		{FieldProvider, dashIfEmpty(p.Params.Provider)},
		{FieldEnv, envValue(p.Params.Env)},
		{FieldEnvMode, envModeValue(p.Params.EnvMode)},
		{FieldTimeout, timeoutValue(p.Timeout)},
		{FieldAPIKey, setValue(p.Params.APIKey)},
	}
	out := make([]AttributionField, 0, len(order))
	for _, o := range order {
		layer := p.attribution[o.field]
		if layer == "" {
			layer = LayerDefault
		}
		out = append(out, AttributionField{Field: o.field, Value: o.value, Layer: layer})
	}
	return out
}

// attributeFields computes, for each launch field, the layer that supplied the
// value the resolver used. base is the bee layer and over is the machine-local
// layer; the precedence mirrors MergeRunParamsLayers — a key `over` declared
// wins, except `binary`, where a committed value is the fallback's better.
func attributeFields(base, over adapters.RunParamsLayer, aliasApplied bool) map[string]Layer {
	attr := map[string]Layer{
		FieldBinary:    binaryLayer(base, over),
		FieldModel:     modelLayer(base, over, aliasApplied),
		FieldYolo:      declaredLayer(base, over, "trust", "force"),
		FieldPlan:      declaredLayer(base, over, "plan"),
		FieldReasoning: declaredLayer(base, over, "reasoning"),
		FieldProvider:  declaredLayer(base, over, "provider"),
		FieldEnv:       declaredLayer(base, over, "env"),
		FieldEnvMode:   declaredLayer(base, over, "env_mode"),
		FieldTimeout:   declaredLayer(base, over, "timeout"),
		FieldAPIKey:    declaredLayer(base, over, "api_key"),
	}
	return attr
}

// declaredLayer reports which layer declared any of the keys. The machine layer
// is checked first because it overlays the bee; `default` means neither source
// declared the key, so the adapter's built-in value stands.
func declaredLayer(base, over adapters.RunParamsLayer, keys ...string) Layer {
	for _, key := range keys {
		if over.Declares(key) {
			return LayerMachine
		}
	}
	for _, key := range keys {
		if base.Declares(key) {
			return LayerBee
		}
	}
	return LayerDefault
}

// binaryLayer mirrors the merge's binary asymmetry: a committed binary wins, a
// machine declaration is a fallback, and an absent one leaves the adapter
// default.
func binaryLayer(base, over adapters.RunParamsLayer) Layer {
	if base.Declares("binary") && strings.TrimSpace(base.Params.Binary) != "" {
		return LayerBee
	}
	if over.Declares("binary") && strings.TrimSpace(over.Params.Binary) != "" {
		return LayerMachine
	}
	return LayerDefault
}

// modelLayer reports alias resolution above the declaring layer: an alias
// substitutes the value, so the alias is what decided the final model.
func modelLayer(base, over adapters.RunParamsLayer, aliasApplied bool) Layer {
	if aliasApplied {
		return LayerAlias
	}
	return declaredLayer(base, over, "model")
}

// UnmappedReasoning reports an operator-facing notice when a bee sets a
// reasoning value the adapter has no flag for, or "" when the value is empty or
// the adapter maps it. Doctor surfaces it rather than letting the value do
// nothing silently.
func UnmappedReasoning(adapter, reasoning string) string {
	if strings.TrimSpace(reasoning) == "" || colony.AdapterMapsReasoning(adapter) {
		return ""
	}
	return fmt.Sprintf("reasoning %q is unmapped on adapter %s (no flag emitted)",
		strings.TrimSpace(reasoning), adapter)
}

func yoloValue(p adapters.RunParams) string {
	if p.Trust == p.Force {
		return boolValue(p.Trust)
	}
	return fmt.Sprintf("trust=%s force=%s", boolValue(p.Trust), boolValue(p.Force))
}

func boolValue(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func dashIfEmpty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "-"
	}
	return s
}

func setValue(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unset"
	}
	return "set"
}

func envValue(env map[string]string) string {
	if len(env) == 0 {
		return "-"
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func envModeValue(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return "inherit"
	}
	return mode
}

func timeoutValue(d time.Duration) string {
	if d <= 0 {
		return "unbounded"
	}
	return d.String()
}
