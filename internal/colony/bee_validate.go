package colony

import (
	"fmt"
	"strings"
)

// ValidateAdapterRequirements checks adapter-specific bee YAML constraints.
//
// Command selection is implicit (params mode without command, command mode with
// command, script mode with adapter script), so a contradictory bee must fail
// here at load rather than at dispatch: a custom command ignores params, and a
// script bee has no argv to run without one.
func (b Bee) ValidateAdapterRequirements() error {
	adapter, err := b.ResolveAdapter()
	if err != nil {
		return err
	}
	if adapter == "script" && !b.Command.IsSet() {
		return fmt.Errorf("colony: bee %q: adapter script requires command", b.Role)
	}
	if b.Command.IsSet() && !b.ParamsAreDefault() {
		keys := b.NonDefaultParamKeys()
		if len(keys) == 0 {
			keys = []string{"params"}
		}
		return fmt.Errorf("colony: bee %q: command and non-default params are mutually exclusive (non-default params: %s)",
			b.Role, strings.Join(keys, ", "))
	}
	return nil
}

// RequiresPrompt reports whether a bee run needs --body or --prompt (script bees do not).
func (b Bee) RequiresPrompt() bool {
	adapter, err := b.ResolveAdapter()
	if err != nil {
		return true
	}
	return adapter != "script"
}
