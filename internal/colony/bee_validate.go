package colony

import "fmt"

// ValidateAdapterRequirements checks adapter-specific bee YAML constraints.
func (b Bee) ValidateAdapterRequirements() error {
	adapter, err := b.ResolveAdapter()
	if err != nil {
		return err
	}
	if adapter == "script" && !b.Command.IsSet() {
		return fmt.Errorf("colony: bee %q: adapter script requires command", b.Role)
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
