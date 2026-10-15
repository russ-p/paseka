package launch

import (
	"fmt"
	"strings"
)

// ModelMapping is the per-adapter argv shape of a resolved model id. Model and
// Provider are the values the adapter passes to its own flags; Warning is a
// non-empty operator notice when the deprecated params.provider key
// contributed to the mapping.
type ModelMapping struct {
	Model    string
	Provider string
	Warning  string
}

// MapModel folds a values-level model id into the pair the adapter's CLI
// expects. Alias values now carry the vendor (`vendor/model[:suffix]`); a bare
// id without a slash stays valid and means "no vendor".
//
//   - cursor and claude pass the id through unchanged;
//   - opencode consumes `provider/model` natively;
//   - pi splits at the first slash into `--provider <vendor> --model <rest>`,
//     where rest keeps every inner slash and colon suffix so OpenRouter-style
//     ids survive the trip.
//
// A deprecated `params.provider` is folded into the model first: `provider: X`
// with a vendor-less `model: Y` becomes `X/Y`, and a model that already carries
// a vendor wins over the separate key. The returned Warning names the mapping
// so a caller can surface it.
func MapModel(adapter, model, provider string) ModelMapping {
	model = strings.TrimSpace(model)
	provider = strings.TrimSpace(provider)

	warning := ""
	if provider != "" {
		switch {
		case model == "":
			warning = "params.provider is deprecated; set params.model to a vendor-qualified id"
		case strings.Contains(model, "/"):
			warning = fmt.Sprintf("params.provider is deprecated and ignored; params.model %q already carries a vendor", model)
		default:
			warning = fmt.Sprintf("params.provider is deprecated; use params.model: %s/%s", provider, model)
		}
	}

	// Provider dissolves into the model id: only a vendor-less model needs the
	// separate key folded in. A model that already has a vendor keeps it.
	if provider != "" && model != "" && !strings.Contains(model, "/") {
		model = provider + "/" + model
	}

	if adapter == "pi" {
		if model == "" {
			return ModelMapping{Model: "", Provider: provider, Warning: warning}
		}
		if i := strings.IndexByte(model, '/'); i > 0 {
			return ModelMapping{Model: model[i+1:], Provider: model[:i], Warning: warning}
		}
	}
	return ModelMapping{Model: model, Warning: warning}
}
