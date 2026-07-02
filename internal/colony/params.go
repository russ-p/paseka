package colony

import (
	"fmt"

	"github.com/paseka/paseka/internal/adapters"
)

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
	return p
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
	case "cursor":
		return name, nil
	default:
		return "", fmt.Errorf("colony: unknown adapter %q for bee %q", name, b.Role)
	}
}
