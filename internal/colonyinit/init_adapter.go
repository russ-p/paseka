package colonyinit

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
	case "opencode":
		return "opencode"
	case "opencodev1":
		return "opencodev1"
	case "opencodev2":
		return "opencodev2"
	default:
		return "cursor"
	}
}

func scoutBeeYAMLFor(adapter string) string {
	switch adapter {
	case "pi":
		return scoutBeePiYAML
	case "opencode":
		return scoutBeeOpenCodeYAML
	case "opencodev1", "opencodev2":
		return pinOpenCodeBee(scoutBeeOpenCodeYAML, adapter)
	default:
		return scoutBeeYAML
	}
}

func builderBeeYAMLFor(adapter string) string {
	switch adapter {
	case "pi":
		return builderBeePiYAML
	case "opencode":
		return builderBeeOpenCodeYAML
	case "opencodev1", "opencodev2":
		return pinOpenCodeBee(builderBeeOpenCodeYAML, adapter)
	default:
		return builderBeeYAML
	}
}

func hivewrightBeeYAMLFor(adapter string) string {
	switch adapter {
	case "pi":
		return hivewrightBeePiYAML
	case "opencode":
		return hivewrightBeeOpenCodeYAML
	case "opencodev1", "opencodev2":
		return pinOpenCodeBee(hivewrightBeeOpenCodeYAML, adapter)
	default:
		return hivewrightBeeYAML
	}
}

// pinOpenCodeBee rewrites the scaffolded adapter line for a bee pinned to one
// OpenCode major, leaving every other line untouched.
func pinOpenCodeBee(yaml, adapter string) string {
	return strings.Replace(yaml, "adapter: opencode\n", "adapter: "+adapter+"\n", 1)
}

func homeConfigYAML(repoRoot, slug, adapter string) string {
	adaptersBlock := `adapters:
  cursor:
    api_key_env: CURSOR_API_KEY
`
	switch adapter {
	case "pi":
		adaptersBlock = `adapters:
  pi: {}
`
	case "opencode", "opencodev1", "opencodev2":
		adaptersBlock = `adapters:
  opencode: {}
`
	}
	return fmt.Sprintf(`colony_root: %q
slug: %q
nats:
  url: nats://127.0.0.1:4222
%s
# forge:
#   command: ["examples/forge/tea.sh"]
`, repoRoot, slug, adaptersBlock)
}
