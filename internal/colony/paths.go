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

type homeConfig struct {
	ColonyRoot string `yaml:"colony_root"`
	Slug       string `yaml:"slug"`
	NATS       struct {
		URL string `yaml:"url"`
	} `yaml:"nats"`
	Adapters map[string]any `yaml:"adapters"`
}

func writeFileIfMissing(path string, content []byte, perm os.FileMode) (created bool, err error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, content, perm); err != nil {
		return false, err
	}
	return true, nil
}

func mkdirAll(path string) error {
	return os.MkdirAll(path, 0o755)
}
