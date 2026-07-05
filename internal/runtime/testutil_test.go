package runtime_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/paseka/paseka/internal/colony"
)

func writeTestColonyWithBees(root string, bees map[string]colony.Bee) error {
	dirs := []string{
		".paseka/bees",
		".paseka/prompts",
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka/colony.yaml"), []byte(`defaults:
  prompt_template: default.md
`), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".paseka/prompts/default.md"), []byte("{{.Task}}"), 0o644); err != nil {
		return err
	}
	for role, bee := range bees {
		content := fmt.Sprintf(`role: %s
adapter: cursor
prompt_template: default.md
`, role)
		if len(bee.Subscribes) > 0 {
			content += "subscribes:\n"
			for _, sub := range bee.Subscribes {
				content += fmt.Sprintf("  - type: %s\n", sub.Type)
				if sub.Kind != "" {
					content += fmt.Sprintf("    kind: %s\n", sub.Kind)
				}
				if sub.Dispatch != "" {
					content += fmt.Sprintf("    dispatch: %s\n", sub.Dispatch)
				}
			}
		}
		if len(bee.Publishes) > 0 {
			content += "publishes:\n"
			for _, pub := range bee.Publishes {
				content += fmt.Sprintf("  - type: %s\n", pub.Type)
				if pub.Kind != "" {
					content += fmt.Sprintf("    kind: %s\n", pub.Kind)
				}
			}
		}
		path := filepath.Join(root, ".paseka/bees", role+".yaml")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
		_ = bee.Role
	}
	return nil
}

func mustWriteTestColony(t interface {
	Helper()
	Fatal(...any)
}, root string, bees map[string]colony.Bee) {
	t.Helper()
	if err := writeTestColonyWithBees(root, bees); err != nil {
		t.Fatal(err)
	}
}
