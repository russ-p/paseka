package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
	"gopkg.in/yaml.v3"
)

const pasekaDir = ".paseka"

// Colony is the project-local manifest under .paseka/colony.yaml.
type Colony struct {
	Slug        string            `yaml:"slug"`
	Defaults    Defaults          `yaml:"defaults"`
	NATS        ColonyNATS        `yaml:"nats"`
	Sectors     map[string]Sector `yaml:"sectors,omitempty"`
	AutoInvites []AutoInviteRule  `yaml:"auto_invites,omitempty"`
}

// ColonyNATS holds project-local NATS overrides.
type ColonyNATS struct {
	SubjectPrefix string `yaml:"subject_prefix"`
}

// Defaults holds colony-wide fallbacks.
type Defaults struct {
	PromptTemplate string `yaml:"prompt_template"`
	SystemTemplate string `yaml:"system_template"`
	EnergyBudget   int    `yaml:"energy_budget,omitempty"`
}

// BeeLocalOverlay holds optional per-machine overrides from bees/<role>.local.yaml.
type BeeLocalOverlay struct {
	PromptTemplate string `yaml:"prompt_template"`
	SystemTemplate string `yaml:"system_template"`
}

// ResolvedEnergyBudget returns the per-trace honey reserve default for this colony.
func (c Colony) ResolvedEnergyBudget() int {
	if c.Defaults.EnergyBudget > 0 {
		return c.Defaults.EnergyBudget
	}
	return protocol.DefaultEnergyBudget
}

// Bee binds a role to an adapter and prompt template.
type Bee struct {
	Role               string             `yaml:"role"`
	Adapter            string             `yaml:"adapter"`
	PromptTemplate     string             `yaml:"prompt_template"`
	SystemTemplate     string             `yaml:"system_template,omitempty"`
	Sector             string             `yaml:"sector,omitempty"`
	Worktree           bool               `yaml:"worktree"`
	Intents            []string           `yaml:"intents,omitempty"`
	DefaultIntent      string             `yaml:"default_intent,omitempty"`
	Command            Command            `yaml:"command,omitempty"`
	PostExec           Command            `yaml:"post_exec,omitempty"`
	Params             map[string]any     `yaml:"params"`
	Subscribes         []SubscriptionRule `yaml:"subscribes,omitempty"`
	Publishes          []PublicationRule  `yaml:"publishes,omitempty"`
	CompletionContract CompletionContract `yaml:"completion_contract,omitempty"`
	RunSummary         RunSummaryPolicy   `yaml:"run_summary,omitempty"`
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
	if err := c.ValidateAutoInvites(); err != nil {
		return Colony{}, err
	}
	return c, nil
}

// ResolvedSystemTemplate returns the configured system template path using overlay precedence.
func ResolvedSystemTemplate(bee Bee, overlay BeeLocalOverlay, defaults Defaults) string {
	if t := strings.TrimSpace(overlay.SystemTemplate); t != "" {
		return t
	}
	if t := strings.TrimSpace(bee.SystemTemplate); t != "" {
		return t
	}
	return strings.TrimSpace(defaults.SystemTemplate)
}

// HasSystemTemplate reports whether a system template is configured for the bee.
func HasSystemTemplate(bee Bee, overlay BeeLocalOverlay, defaults Defaults) bool {
	return ResolvedSystemTemplate(bee, overlay, defaults) != ""
}

// LoadBee reads .paseka/bees/<role>.yaml and optional <role>.local.yaml overlay.
func LoadBee(colonyRoot, role string) (Bee, BeeLocalOverlay, error) {
	if err := validateRole(role); err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}
	basePath := filepath.Join(colonyRoot, pasekaDir, "bees", role+".yaml")
	data, err := os.ReadFile(basePath)
	if err != nil {
		return Bee{}, BeeLocalOverlay{}, fmt.Errorf("colony: read bee %q: %w", role, err)
	}
	var bee Bee
	if err := yaml.Unmarshal(data, &bee); err != nil {
		return Bee{}, BeeLocalOverlay{}, fmt.Errorf("colony: parse bee %q: %w", role, err)
	}
	if bee.Role == "" {
		bee.Role = role
	}
	if err := bee.ValidateEventRules(); err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}
	if err := bee.ValidateRunSummaryPolicy(); err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}
	if err := bee.ValidateAdapterRequirements(); err != nil {
		return Bee{}, BeeLocalOverlay{}, err
	}

	var overlay BeeLocalOverlay
	localPath := filepath.Join(colonyRoot, pasekaDir, "bees", role+".local.yaml")
	localData, err := os.ReadFile(localPath)
	if err == nil {
		if err := yaml.Unmarshal(localData, &overlay); err != nil {
			return Bee{}, BeeLocalOverlay{}, fmt.Errorf("colony: parse bee local %q: %w", role, err)
		}
	} else if !os.IsNotExist(err) {
		return Bee{}, BeeLocalOverlay{}, fmt.Errorf("colony: read bee local %q: %w", role, err)
	}

	return bee, overlay, nil
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
