package nuc

const (
	APIVersion = "paseka/v1"
	Kind       = "Nuc"
)

// Document is a portable colony bee pack (bees YAML + prompts).
type Document struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

// Metadata describes the nuc pack.
type Metadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

// Spec holds shareable colony config.
type Spec struct {
	Bees    map[string]string `yaml:"bees"`
	Prompts map[string]string `yaml:"prompts"`
	Cues    map[string]string `yaml:"cues,omitempty"`
	// Skills maps a skill name to its files, keyed by slash-form path relative
	// to the skill directory. `SKILL.md` is required; supporting files such as
	// `scripts/` and `references/` travel with it.
	Skills map[string]map[string]string `yaml:"skills,omitempty"`
}

// ExportOptions configures colony → nuc export.
type ExportOptions struct {
	ColonyRoot  string
	Name        string
	Description string
	Bees        []string // empty = all roles
	Cues        []string // empty = all cues when present
	Skills      []string // explicit skill references added to the roles' resolved set
}

// ImportOptions configures nuc → colony import.
type ImportOptions struct {
	ColonyRoot string
	Force      bool
	DryRun     bool
}

// ImportResult summarizes per-file import actions.
type ImportResult struct {
	Created     []string
	Skipped     []string
	Overwritten []string
}
