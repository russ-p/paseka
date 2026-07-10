package colony

import (
	"fmt"
	"strings"
)

// NormalizeInitAdapter returns the adapter used to scaffold a new colony.
// Cursor is the default; unknown values fall back to cursor.
func NormalizeInitAdapter(name string) string {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "pi":
		return "pi"
	default:
		return "cursor"
	}
}

func scoutBeeYAMLFor(adapter string) string {
	switch adapter {
	case "pi":
		return scoutBeePiYAML
	default:
		return scoutBeeYAML
	}
}

func builderBeeYAMLFor(adapter string) string {
	switch adapter {
	case "pi":
		return builderBeePiYAML
	default:
		return builderBeeYAML
	}
}

func homeConfigYAML(repoRoot, slug, adapter string) string {
	adaptersBlock := `adapters:
  cursor:
    api_key_env: CURSOR_API_KEY
`
	if adapter == "pi" {
		adaptersBlock = `adapters:
  pi: {}
`
	}
	return fmt.Sprintf(`colony_root: %q
slug: %q
nats:
  url: nats://127.0.0.1:4222
%s`, repoRoot, slug, adaptersBlock)
}
