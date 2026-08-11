package colony

import (
	"os"
	"path/filepath"
)

// HomeBase returns the paseka config base directory (~/.config/paseka or XDG_CONFIG_HOME).
func HomeBase() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "paseka"), nil
}

// HomeDir returns ~/.config/paseka/<slug>/.
func HomeDir(slug string) (string, error) {
	base, err := HomeBase()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, slug), nil
}

// PasekaPath joins colony root with .paseka/... segments.
func PasekaPath(colonyRoot string, parts ...string) string {
	all := append([]string{colonyRoot, pasekaDir}, parts...)
	return filepath.Join(all...)
}
