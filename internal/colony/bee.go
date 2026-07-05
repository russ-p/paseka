package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const pasekaDir = ".paseka"

// Colony is the project-local manifest under .paseka/colony.yaml.
type Colony struct {
	Slug     string     `yaml:"slug"`
	Defaults Defaults   `yaml:"defaults"`
	NATS     ColonyNATS `yaml:"nats"`
}

// ColonyNATS holds project-local NATS overrides.
type ColonyNATS struct {
	SubjectPrefix string `yaml:"subject_prefix"`
}

// Defaults holds colony-wide fallbacks.
type Defaults struct {
	PromptTemplate string `yaml:"prompt_template"`
}

// Bee binds a role to an adapter and prompt template.
type Bee struct {
	Role           string         `yaml:"role"`
	Adapter        string         `yaml:"adapter"`
	PromptTemplate string         `yaml:"prompt_template"`
	Worktree       bool           `yaml:"worktree"`
	Params         map[string]any `yaml:"params"`
}

// LoadColony reads .paseka/colony.yaml. Missing file yields zero values.
func LoadColony(colonyRoot string) (Colony, error) {
	path := filepath.Join(colonyRoot, pasekaDir, "colony.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Colony{}, nil
		}
		return Colony{}, fmt.Errorf("colony: read manifest: %w", err)
	}
	var c Colony
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Colony{}, fmt.Errorf("colony: parse manifest: %w", err)
	}
	return c, nil
}

// LoadBee reads .paseka/bees/<role>.yaml and optional <role>.local.yaml overlay.
func LoadBee(colonyRoot, role string) (Bee, string, error) {
	if err := validateRole(role); err != nil {
		return Bee{}, "", err
	}
	basePath := filepath.Join(colonyRoot, pasekaDir, "bees", role+".yaml")
	data, err := os.ReadFile(basePath)
	if err != nil {
		return Bee{}, "", fmt.Errorf("colony: read bee %q: %w", role, err)
	}
	var bee Bee
	if err := yaml.Unmarshal(data, &bee); err != nil {
		return Bee{}, "", fmt.Errorf("colony: parse bee %q: %w", role, err)
	}
	if bee.Role == "" {
		bee.Role = role
	}

	localTemplate := ""
	localPath := filepath.Join(colonyRoot, pasekaDir, "bees", role+".local.yaml")
	localData, err := os.ReadFile(localPath)
	if err == nil {
		var overlay struct {
			PromptTemplate string `yaml:"prompt_template"`
		}
		if err := yaml.Unmarshal(localData, &overlay); err != nil {
			return Bee{}, "", fmt.Errorf("colony: parse bee local %q: %w", role, err)
		}
		localTemplate = overlay.PromptTemplate
		if overlay.PromptTemplate != "" {
			// local overlay wins over base bee file at resolve time
		}
	} else if !os.IsNotExist(err) {
		return Bee{}, "", fmt.Errorf("colony: read bee local %q: %w", role, err)
	}

	return bee, localTemplate, nil
}

func validateRole(role string) error {
	role = strings.TrimSpace(role)
	if role == "" {
		return fmt.Errorf("colony: bee role is required")
	}
	if strings.Contains(role, "/") || strings.Contains(role, "..") {
		return fmt.Errorf("colony: invalid bee role %q", role)
	}
	return nil
}

// BeesDir returns .paseka/bees under colony root.
func BeesDir(colonyRoot string) string {
	return filepath.Join(colonyRoot, pasekaDir, "bees")
}
