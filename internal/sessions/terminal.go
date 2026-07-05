package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// TerminalKind selects how to open an attach UI.
type TerminalKind string

const (
	TerminalDefault TerminalKind = "default"
	TerminalGhostty TerminalKind = "ghostty"
)

// TerminalConfig is machine-local terminal preferences for session attach.
type TerminalConfig struct {
	Kind          TerminalKind `yaml:"terminal"`
	GhosttyBinary string       `yaml:"ghostty_binary"`
}

// DefaultTerminalConfig returns sensible defaults.
func DefaultTerminalConfig() TerminalConfig {
	return TerminalConfig{
		Kind:          TerminalDefault,
		GhosttyBinary: "ghostty",
	}
}

// LaunchAttach opens a terminal UI that runs attachCmd.
// For default, attachCmd runs in the current terminal (caller should not use this).
// For ghostty, spawns a new Ghostty window.
func LaunchAttach(cfg TerminalConfig, attachCmd []string) error {
	switch cfg.Kind {
	case "", TerminalDefault:
		return fmt.Errorf("sessions: use AttachInPlace for default terminal")
	case TerminalGhostty:
		return launchGhostty(cfg, attachCmd)
	default:
		return fmt.Errorf("sessions: unknown terminal %q", cfg.Kind)
	}
}

func launchGhostty(cfg TerminalConfig, attachCmd []string) error {
	binary := cfg.GhosttyBinary
	if binary == "" {
		binary = "ghostty"
	}
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("sessions: %q not found in PATH (install Ghostty or set terminal: default)", binary)
	}

	// ghostty -e <cmd> [args...]
	args := []string{"-e"}
	args = append(args, attachCmd...)

	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sessions: launch ghostty: %w", err)
	}
	return nil
}

// ResolveTerminalKind parses a terminal kind string.
func ResolveTerminalKind(s string) TerminalKind {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ghostty":
		return TerminalGhostty
	default:
		return TerminalDefault
	}
}
