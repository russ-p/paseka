package systeminject

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/paseka/paseka/internal/runs"
)

const (
	CursorPluginDirName = "cursor-plugin"
	cursorPluginSubdir  = ".cursor-plugin"
	cursorRulesSubdir   = "rules"
	cursorRuleFileName  = "bee-system.mdc"
)

// CursorPluginPath returns the run-local path where WriteCursorPlugin materializes
// the ephemeral Cursor plugin (whether or not it has been written yet).
func CursorPluginPath(runDir runs.Dir) string {
	return filepath.Join(runDir.Root(), CursorPluginDirName)
}

// WriteCursorPlugin materializes an ephemeral Cursor plugin under the run dir.
// The plugin carries the rendered system context as an always-on rule.
func WriteCursorPlugin(runDir runs.Dir, systemText string) (string, error) {
	if systemText == "" {
		return "", fmt.Errorf("systeminject: system text is required")
	}
	pluginRoot := CursorPluginPath(runDir)
	manifestDir := filepath.Join(pluginRoot, cursorPluginSubdir)
	rulesDir := filepath.Join(pluginRoot, cursorRulesSubdir)
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return "", fmt.Errorf("systeminject: mkdir cursor plugin: %w", err)
	}
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		return "", fmt.Errorf("systeminject: mkdir cursor plugin manifest: %w", err)
	}

	manifest := []byte(`{
  "name": "paseka-bee-system",
  "version": "1.0.0",
  "description": "Ephemeral Paseka bee system context"
}
`)
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), manifest, 0o644); err != nil {
		return "", fmt.Errorf("systeminject: write plugin.json: %w", err)
	}

	rule := []byte("---\ndescription: Paseka bee system context\nalwaysApply: true\n---\n\n" + systemText)
	if err := os.WriteFile(filepath.Join(rulesDir, cursorRuleFileName), rule, 0o644); err != nil {
		return "", fmt.Errorf("systeminject: write cursor rule: %w", err)
	}
	return pluginRoot, nil
}
