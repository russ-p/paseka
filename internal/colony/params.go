package colony

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

// ParamsAreDefault reports whether the bee's params resolve to the adapter
// defaults, which makes them a no-op and therefore compatible with a custom
// `command:`. A command-mode bee may only carry params that leave these values
// untouched: a custom command ignores params entirely, so any deviation is a
// contradiction rather than a flag that quietly did not apply.
func (b Bee) ParamsAreDefault() bool {
	return len(b.NonDefaultParamKeys()) == 0
}

// NonDefaultParamKeys returns the declared params whose value differs from the
// adapter default, sorted. It names the offending keys in the load-time
// exclusivity error so the operator knows what to remove or reset.
//
// `timeout` is process control applied by the runtime, not an adapter flag, so
// it is exempt: a command-mode bee may still bound its run.
func (b Bee) NonDefaultParamKeys() []string {
	if len(b.Params) == 0 {
		return nil
	}
	var keys []string
	for key, value := range b.Params {
		if key == "timeout" {
			continue
		}
		if !isDefaultParamValue(key, value) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func isDefaultParamValue(key string, value any) bool {
	switch key {
	case "trust", "force", "yolo":
		b, ok := value.(bool)
		return ok && b
	case "plan":
		b, ok := value.(bool)
		return ok && !b
	case "env":
		switch v := value.(type) {
		case nil:
			return true
		case map[string]any:
			return len(v) == 0
		case map[string]string:
			return len(v) == 0
		default:
			return false
		}
	case "options":
		switch v := value.(type) {
		case nil:
			return true
		case map[string]any:
			return len(v) == 0
		case map[string]string:
			return len(v) == 0
		default:
			return false
		}
	default:
		s, ok := value.(string)
		return ok && s == ""
	}
}

// RunParamKeys is the closed set of keys a bee or profile `params` block may
// declare. A key outside this set is treated as a typo rather than a silently
// ignored extra: it fails at load so the operator sees the mistake instead of a
// run whose flags quietly did not change.
var RunParamKeys = []string{
	"binary",
	"env",
	"env_mode",
	"force",
	"model",
	"options",
	"output_format",
	"plan",
	"provider",
	"reasoning",
	"thinking",
	"timeout",
	"trust",
	"yolo",
}

// RemovedRunParamKeys is the list the *next* release removes: the legacy keys
// the deprecation window still carries, with the semantic replacement for each.
// The window is open for one full release, so these keys remain in
// RunParamKeys and load without error; doctor warns so operators migrate before
// removal. An empty replacement means the key is dropped outright (the runtime
// owns the behavior now).
var RemovedRunParamKeys = map[string]string{
	"trust":         "yolo",
	"force":         "yolo",
	"thinking":      "reasoning",
	"provider":      "model",
	"output_format": "",
}

// ValidateRunParams rejects unknown keys and invalid values. Unknown keys fail
// with the valid-key list; env must be a name/value map and env_mode, when set,
// must be the strict opt-in.
func ValidateRunParams(params map[string]any) error {
	if err := ValidateRunParamKeys(params); err != nil {
		return err
	}
	return ValidateRunParamValues(params)
}

// ValidateRunParamValues checks the value of every accepted params key. It is
// the half of ValidateRunParams the diagnosis loader keeps: doctor relaxes only
// the key gate so it can name a leftover key, but still fails on a value the
// runtime would reject, so a colony that cannot load is not silently doctor-clean.
func ValidateRunParamValues(params map[string]any) error {
	if len(params) == 0 {
		return nil
	}
	if v, ok := params["env_mode"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("params.env_mode must be a string")
		}
		switch strings.TrimSpace(s) {
		case "", adapters.EnvModeStrict:
		default:
			return fmt.Errorf("params.env_mode must be %q (got %q)", adapters.EnvModeStrict, s)
		}
	}
	if v, ok := params["env"]; ok && v != nil {
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("params.env must be a map of name: value")
		}
	}
	if v, ok := params["options"]; ok && v != nil {
		if _, ok := v.(map[string]any); !ok {
			return fmt.Errorf("params.options must be a map of name: value")
		}
	}
	if v, ok := params["timeout"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("params.timeout must be a duration string (e.g. 90s, 5m)")
		}
		d, err := time.ParseDuration(strings.TrimSpace(s))
		if err != nil {
			return fmt.Errorf("params.timeout: %w", err)
		}
		if d <= 0 {
			return fmt.Errorf("params.timeout must be a positive duration (got %q)", s)
		}
	}
	return nil
}

// ValidateRunParamKeys rejects any key outside RunParamKeys and names the valid
// set. It is applied to bee `params` and to profile `params` at load time.
//
// While the deprecation window is open the legacy keys are members of
// RunParamKeys, so they load without error; the RemovedRunParamKeys branch below
// is inert until the next release moves them out of the valid set, at which
// point a leftover key is reported with its replacement rather than as a bare
// typo.
func ValidateRunParamKeys(params map[string]any) error {
	if len(params) == 0 {
		return nil
	}
	var removed, unknown []string
	for key := range params {
		switch {
		case isRunParamKey(key):
		case isRemovedRunParamKey(key):
			removed = append(removed, key)
		default:
			unknown = append(unknown, key)
		}
	}
	if len(removed) == 0 && len(unknown) == 0 {
		return nil
	}
	sort.Strings(removed)
	sort.Strings(unknown)
	var parts []string
	if len(removed) > 0 {
		parts = append(parts, removedParamMessage(removed))
	}
	if len(unknown) > 0 {
		noun := "key"
		if len(unknown) > 1 {
			noun = "keys"
		}
		parts = append(parts, fmt.Sprintf("unknown params %s %s", noun, quoteKeys(unknown)))
	}
	return fmt.Errorf("%s (valid keys: %s)",
		strings.Join(parts, "; "), strings.Join(RunParamKeys, ", "))
}

// removedParamMessage renders one migration sentence per retired key. The
// replacement is named when there is one; a key the runtime absorbed says so
// instead of pointing at a semantic key that does not exist.
func removedParamMessage(keys []string) string {
	msgs := make([]string, 0, len(keys))
	for _, key := range keys {
		if repl := RemovedRunParamKeys[key]; repl != "" {
			msgs = append(msgs, fmt.Sprintf("params.%s was removed; use params.%s", key, repl))
			continue
		}
		msgs = append(msgs, fmt.Sprintf("params.%s was removed (the runtime owns it now)", key))
	}
	return strings.Join(msgs, "; ")
}

func isRemovedRunParamKey(key string) bool {
	_, ok := RemovedRunParamKeys[key]
	return ok
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
// `yolo` still floats the autonomous default, while a profile that sets
// `yolo: false` can carry the explicit false through.
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
	// trust/force are the legacy one-release aliases for yolo.
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
	// It drives both the trust and force flags the adapters still take, and
	// the declared set records trust/force, which is the pair the merge and
	// attribution work in.
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
	// output_format is accepted and ignored: the runtime owns structured output,
	// so the legacy key never reaches RunParams and declares nothing.
	if v, ok := envParam(b.Params, "env"); ok {
		p.Env = v
		declared["env"] = true
	}
	if v, ok := optionsParam(b.Params, "options"); ok {
		p.Options = v
		declared["options"] = true
	}
	if v, ok := stringParam(b.Params, "env_mode"); ok {
		p.EnvMode = strings.TrimSpace(v)
		declared["env_mode"] = true
	}
	if v, ok := durationParam(b.Params, "timeout"); ok {
		p.Timeout = v
		declared["timeout"] = true
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
	if declared["env"] {
		merged := make(map[string]string, len(out.Env)+len(over.Params.Env))
		for k, v := range out.Env {
			merged[k] = v
		}
		for k, v := range over.Params.Env {
			merged[k] = v
		}
		out.Env = merged
	}
	if declared["options"] {
		// Like binary, the committed bee's options win over the machine-local
		// fallback. The machine file supplies defaults; a bee may override any
		// key it names.
		merged := make(map[string]string, len(out.Options)+len(over.Params.Options))
		for k, v := range over.Params.Options {
			merged[k] = v
		}
		for k, v := range out.Options {
			merged[k] = v
		}
		out.Options = merged
	}
	if declared["env_mode"] {
		out.EnvMode = over.Params.EnvMode
	}
	if declared["timeout"] {
		out.Timeout = over.Params.Timeout
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
	case "opencode", "opencodev1", "opencodev2":
		return adapters.RunParams{
			Binary:  openCodeBinary(ctx.OpenCode, adapterName),
			Options: openCodeOptions(ctx.OpenCode, adapterName),
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
	case "opencode", "opencodev1", "opencodev2":
		if openCodeBinaryDeclared(ctx.OpenCode, adapterName) {
			layer.Params.Binary = openCodeBinary(ctx.OpenCode, adapterName)
			layer.Declared["binary"] = true
		}
		if options := openCodeOptions(ctx.OpenCode, adapterName); len(options) > 0 {
			layer.Params.Options = options
			layer.Declared["options"] = true
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

// openCodeBinary is the effective binary for one OpenCode name. A pinned name
// prefers its own path and never falls through to another adapter's defaults.
func openCodeBinary(cfg OpenCodeAdapterConfig, name string) string {
	switch name {
	case "opencodev1":
		if cfg.v1BinaryDeclared {
			return cfg.V1Binary
		}
	case "opencodev2":
		if cfg.v2BinaryDeclared {
			return cfg.V2Binary
		}
	}
	return cfg.Binary
}

// openCodeBinaryDeclared reports whether the file declares a binary relevant to
// this OpenCode name. An absent file leaves the adapter's built-in default.
func openCodeBinaryDeclared(cfg OpenCodeAdapterConfig, name string) bool {
	switch name {
	case "opencodev1":
		return cfg.v1BinaryDeclared || cfg.binaryDeclared
	case "opencodev2":
		return cfg.v2BinaryDeclared || cfg.binaryDeclared
	default:
		return cfg.binaryDeclared
	}
}

// openCodeOptions combines the file's options with the pin a named adapter
// implies. A pinned bee skips detection, so its version option is authoritative.
func openCodeOptions(cfg OpenCodeAdapterConfig, name string) map[string]string {
	opts := cfg.Options()
	switch name {
	case "opencodev1":
		if opts == nil {
			opts = map[string]string{}
		}
		opts["version"] = "1"
	case "opencodev2":
		if opts == nil {
			opts = map[string]string{}
		}
		opts["version"] = "2"
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func envParam(m map[string]any, key string) (map[string]string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for name, value := range raw {
		out[name] = fmt.Sprint(value)
	}
	return out, true
}

// optionsParam maps params.options to adapter-specific strings. YAML turns an
// unquoted `version: 2` into an integer, so scalar values are stringified rather
// than rejected; nested maps and lists are not a valid option value.
func optionsParam(m map[string]any, key string) (map[string]string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return nil, false
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for name, value := range raw {
		switch value.(type) {
		case map[string]any, []any:
			continue
		}
		out[name] = fmt.Sprint(value)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
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

// durationParam parses a params key as a Go duration string. An absent key, a
// non-string value, or an unparseable duration yields ok=false; strict load-time
// validation reports those, so the mapping stays permissive and total.
func durationParam(m map[string]any, key string) (time.Duration, bool) {
	s, ok := stringParam(m, key)
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return d, true
}

// ResolveAdapter returns the adapter name, defaulting to cursor.
func (b Bee) ResolveAdapter() (string, error) {
	name := b.Adapter
	if name == "" {
		name = "cursor"
	}
	switch name {
	case "cursor", "pi", "claude", "opencode", "opencodev1", "opencodev2", "script":
		return name, nil
	default:
		return "", fmt.Errorf("colony: unknown adapter %q for bee %q", name, b.Role)
	}
}
