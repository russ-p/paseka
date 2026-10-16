package launch

import "strings"

// ModelMapping is the per-adapter argv shape of a resolved model id. Model and
// Provider are the values the adapter passes to its own flags.
type ModelMapping struct {
	Model    string
	Provider string
}

// MapModel maps a values-level model id into the pair the adapter's CLI
// expects. Alias values carry the vendor (`vendor/model[:suffix]`); a bare id
// without a slash stays valid and means "no vendor".
//
//   - cursor and claude pass the id through unchanged;
//   - opencode consumes `provider/model` natively;
//   - pi splits at the first slash into `--provider <vendor> --model <rest>`,
//     where rest keeps every inner slash and colon suffix so OpenRouter-style
//     ids survive the trip.
func MapModel(adapter, model string) ModelMapping {
	model = strings.TrimSpace(model)

	if adapter == "pi" {
		if i := strings.IndexByte(model, '/'); i > 0 {
			return ModelMapping{Model: model[i+1:], Provider: model[:i]}
		}
	}
	return ModelMapping{Model: model}
}
