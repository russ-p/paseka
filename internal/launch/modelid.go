package launch

import "strings"

// ModelMapping is the per-adapter argv shape of a resolved model id. Model and
// Provider are the values the adapter passes to its own flags; ModelID is the
// whole id before it was split for argv.
type ModelMapping struct {
	Model    string
	Provider string
	// ModelID is the resolved id before the per-adapter split. A run record
	// keeps this rather than re-joining Model and Provider: for pi the vendor
	// comes off a slice that may itself hold a slash, so the two halves do not
	// reconstruct the id.
	ModelID string
}

// MapModel maps a values-level model id into the pair the adapter's CLI
// expects. Model values carry the vendor (`vendor/model[:suffix]`); a bare id
// without a slash stays valid and means "no vendor".
//
//   - cursor and claude pass the id through unchanged;
//   - opencode consumes `provider/model` natively;
//   - pi splits at the first slash into `--provider <vendor> --model <rest>`,
//     where rest keeps every inner slash and colon suffix so OpenRouter-style
//     ids survive the trip.
//
// The returned Provider is derived from the id, never declared separately:
// the vendor half is what pi receives as its own flag.
func MapModel(adapter, model string) ModelMapping {
	model = strings.TrimSpace(model)
	modelID := model

	if adapter == "pi" {
		if model == "" {
			return ModelMapping{}
		}
		if i := strings.IndexByte(model, '/'); i > 0 {
			return ModelMapping{Model: model[i+1:], Provider: model[:i], ModelID: modelID}
		}
	}
	return ModelMapping{Model: model, ModelID: modelID}
}
