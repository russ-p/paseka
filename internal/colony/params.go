package colony

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
)

// RunParamKeys is the closed set of keys a bee or profile `params` block may
// declare. A key outside this set is treated as a typo rather than a silently
// ignored extra: it fails at load so the operator sees the mistake instead of a
// run whose flags quietly did not change.
var RunParamKeys = []string{
	"binary",
	"force",
	"model",
	"output_format",
	"plan",
	"provider",
	"reasoning",
	"thinking",
	"trust",
	"yolo",
}

// ValidateRunParamKeys rejects any key outside RunParamKeys and names the valid
// set. It is applied to bee `params` and to profile `params` at load time.
func ValidateRunParamKeys(params map[string]any) error {
	if len(params) == 0 {
		return nil
	}
	var unknown []string
	for key := range params {
		if !isRunParamKey(key) {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	noun := "key"
	if len(unknown) > 1 {
		noun = "keys"
	}
	return fmt.Errorf("unknown params %s %s (valid keys: %s)",
		noun, quoteKeys(unknown), strings.Join(RunParamKeys, ", "))
}

func isRunParamKey(key string) bool {
	for _, valid := range RunParamKeys {
		if key == valid {
			return true
		}
	}
	return false
}

func quoteKeys(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = fmt.Sprintf("%q", key)
	}
	return strings.Join(quoted, ", ")
}

// RunParamsFromBee maps bee YAML params to adapter RunParams.
func RunParamsFromBee(b Bee) adapters.RunParams {
	return LayerFromBee(b).Params
}

// LayerFromBee maps bee YAML params and records which keys the bee declared.
// The declared set lets a higher layer merge tri-state: a bee that never set
// `trust` still floats the default true, while a profile that sets `trust:
// false` can carry the explicit false through.
func LayerFromBee(b Bee) adapters.RunParamsLayer {
	p := adapters.RunParams{Trust: true, Force: true}
	layer := adapters.RunParamsLayer{Declared: map[string]bool{}}
	if b.Params == nil {
		layer.Params = p
		return layer
	}
	declared := layer.Declared
	if v, ok := stringParam(b.Params, "model"); ok {
		p.Model = v
		declared["model"] = true
	}
	if v, ok := stringParam(b.Params, "output_format"); ok {
		p.OutputFormat = v
		declared["output_format"] = true
	}
	if v, ok := boolParam(b.Params, "trust"); ok {
		p.Trust = v
		declared["trust"] = true
	}
	if v, ok := boolParam(b.Params, "force"); ok {
		p.Force = v
		declared["force"] = true
	}
	// yolo is the semantic collapse of trust/force and wins when both appear:
	// the legacy keys are the deprecation fallback, not the other way around.
	if v, ok := boolParam(b.Params, "yolo"); ok {
		p.Trust = v
		p.Force = v
		declared["trust"] = true
		declared["force"] = true
	}
	if v, ok := boolParam(b.Params, "plan"); ok {
		p.Plan = v
		declared["plan"] = true
	}
	if v, ok := stringParam(b.Params, "binary"); ok {
		p.Binary = v
		declared["binary"] = true
	}
	if v, ok := stringParam(b.Params, "provider"); ok {
		p.Provider = v
		declared["provider"] = true
	}
	// reasoning is the semantic key; thinking is a one-release deprecation
	// alias for it. reasoning wins when both appear.
	if v, ok := stringParam(b.Params, "reasoning"); ok {
		p.Reasoning = v
		declared["reasoning"] = true
	}
	if v, ok := stringParam(b.Params, "thinking"); ok {
		if !declared["reasoning"] {
			p.Reasoning = v
			declared["reasoning"] = true
		}
	}
	layer.Params = p
	return layer
}

// MergeRunParamsLayers overlays the keys `over` declared onto `base` with
// presence-aware, tri-state semantics. A declared boolean is applied even when
// false, so a higher layer can turn a lower layer's true off; an undeclared key
// leaves the base value untouched.
//
// `binary` is the one asymmetry: `over` is the machine-local layer, which is a
// *fallback* for the executable. A committed `params.binary` therefore always
// wins, and a machine file that never set `binary` leaves the base — and behind
// it the adapter's own default binary — in place. `api_key` is a machine fact
// and always comes from `over` when declared.
func MergeRunParamsLayers(base, over adapters.RunParamsLayer) adapters.RunParams {
	out := base.Params
	declared := over.Declared
	if declared["model"] {
		out.Model = over.Params.Model
	}
	if declared["output_format"] {
		out.OutputFormat = over.Params.OutputFormat
	}
	if declared["provider"] {
		out.Provider = over.Params.Provider
	}
	if declared["reasoning"] {
		out.Reasoning = over.Params.Reasoning
	}
	if declared["trust"] {
		out.Trust = over.Params.Trust
	}
	if declared["force"] {
		out.Force = over.Params.Force
	}
	if declared["plan"] {
		out.Plan = over.Params.Plan
	}
	if declared["api_key"] {
		out.APIKey = over.Params.APIKey
	}
	if declared["binary"] && out.Binary == "" {
		out.Binary = over.Params.Binary
	}
	return out
}

// AdapterExtra returns machine-local binary and API key for the resolved adapter.
func AdapterExtra(ctx Context, adapterName string) adapters.RunParams {
	switch adapterName {
	case "pi":
		return adapters.RunParams{
			Binary: ctx.Pi.Binary,
			APIKey: ctx.Pi.APIKey(),
		}
	case "claude":
		return adapters.RunParams{
			Binary: ctx.Claude.Binary,
			APIKey: ctx.Claude.APIKey(),
		}
	case "opencode":
		return adapters.RunParams{
			Binary: ctx.OpenCode.Binary,
		}
	default:
		return adapters.RunParams{
			Binary: ctx.Cursor.Binary,
			APIKey: ctx.Cursor.APIKey(),
		}
	}
}

// AdapterExtraLayer is the machine-local layer as declared keys: the resolved
// credential and a binary only when the machine file actually set one. An
// absent (or binary-free) file declares no binary, so the merge leaves the
// adapter's built-in default as the true fallback.
func AdapterExtraLayer(ctx Context, adapterName string) adapters.RunParamsLayer {
	layer := adapters.RunParamsLayer{Declared: map[string]bool{"api_key": true}}
	switch adapterName {
	case "pi":
		layer.Params.APIKey = ctx.Pi.APIKey()
		if ctx.Pi.BinaryDeclared() {
			layer.Params.Binary = ctx.Pi.Binary
			layer.Declared["binary"] = true
		}
	case "claude":
		layer.Params.APIKey = ctx.Claude.APIKey()
		if ctx.Claude.BinaryDeclared() {
			layer.Params.Binary = ctx.Claude.Binary
			layer.Declared["binary"] = true
		}
	case "opencode":
		if ctx.OpenCode.BinaryDeclared() {
			layer.Params.Binary = ctx.OpenCode.Binary
			layer.Declared["binary"] = true
		}
	default:
		layer.Params.APIKey = ctx.Cursor.APIKey()
		if ctx.Cursor.BinaryDeclared() {
			layer.Params.Binary = ctx.Cursor.Binary
			layer.Declared["binary"] = true
		}
	}
	return layer
}

func stringParam(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func boolParam(m map[string]any, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

// ResolveAdapter returns the adapter name, defaulting to cursor.
func (b Bee) ResolveAdapter() (string, error) {
	name := b.Adapter
	if name == "" {
		name = "cursor"
	}
	switch name {
	case "cursor", "pi", "claude", "opencode", "script":
		return name, nil
	default:
		return "", fmt.Errorf("colony: unknown adapter %q for bee %q", name, b.Role)
	}
}
