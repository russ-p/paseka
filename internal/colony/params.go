package colony

import (
	"fmt"

	"github.com/paseka/paseka/internal/adapters"
)

// MergeRunParams overlays non-zero/explicit fields from over onto base.
func MergeRunParams(base, over adapters.RunParams) adapters.RunParams {
	if over.Binary != "" {
		base.Binary = over.Binary
	}
	if over.APIKey != "" {
		base.APIKey = over.APIKey
	}
	if over.Model != "" {
		base.Model = over.Model
	}
	if over.OutputFormat != "" {
		base.OutputFormat = over.OutputFormat
	}
	if over.Trust {
		base.Trust = true
	}
	if over.Force {
		base.Force = true
	}
	if over.Plan {
		base.Plan = true
	}
	if over.Provider != "" {
		base.Provider = over.Provider
	}
	if over.Thinking != "" {
		base.Thinking = over.Thinking
	}
	return base
}

// RunParamsFromBee maps bee YAML params to adapter RunParams.
func RunParamsFromBee(b Bee) adapters.RunParams {
	p := adapters.RunParams{
		Trust: true,
		Force: true,
	}
	if b.Params == nil {
		return p
	}
	if v, ok := stringParam(b.Params, "model"); ok {
		p.Model = v
	}
	if v, ok := stringParam(b.Params, "output_format"); ok {
		p.OutputFormat = v
	}
	if v, ok := boolParam(b.Params, "trust"); ok {
		p.Trust = v
	}
	if v, ok := boolParam(b.Params, "force"); ok {
		p.Force = v
	}
	if v, ok := boolParam(b.Params, "plan"); ok {
		p.Plan = v
	}
	if v, ok := stringParam(b.Params, "binary"); ok {
		p.Binary = v
	}
	if v, ok := stringParam(b.Params, "provider"); ok {
		p.Provider = v
	}
	if v, ok := stringParam(b.Params, "thinking"); ok {
		p.Thinking = v
	}
	return p
}

// AdapterExtra returns machine-local binary and API key for the resolved adapter.
func AdapterExtra(ctx Context, adapterName string) adapters.RunParams {
	switch adapterName {
	case "pi":
		return adapters.RunParams{
			Binary: ctx.Pi.Binary,
			APIKey: ctx.Pi.APIKey(),
		}
	default:
		return adapters.RunParams{
			Binary: ctx.Cursor.Binary,
			APIKey: ctx.Cursor.APIKey(),
		}
	}
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
	case "cursor", "pi":
		return name, nil
	default:
		return "", fmt.Errorf("colony: unknown adapter %q for bee %q", name, b.Role)
	}
}
