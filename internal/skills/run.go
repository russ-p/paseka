package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
)

// SectionHeading opens the advertisement the runtime appends to a run's system
// prompt. The block is metadata only: a body is never inlined, so a procedure
// costs a small fixed amount until a bee reads the file.
const SectionHeading = "## Available skills"

// RunInput is the configuration one run resolves its skill set against. The
// same resolver reads a live colony and a trail's frozen snapshot; ConfigRoot
// is the only difference, and Frozen decides where a home profile lives.
type RunInput struct {
	// ConfigRoot holds the configuration tree in force: the live colony root
	// for an unbound trail, or a trail's frozen snapshot tree.
	ConfigRoot string
	// Slug identifies the machine-local home directory. It is read only when
	// the run is not frozen: a frozen run takes the selected profile from the
	// captured tree.
	Slug string
	// Frozen marks a trail's immutable snapshot. The store, ledger, role list,
	// and selected profile are then read from ConfigRoot only: never from the
	// live checkout, because a store edit after trail start must not change a
	// running trail.
	Frozen bool
	// Role is the bee whose gate is resolved.
	Role string
	// Profile is the selected profile name, empty when none is selected.
	Profile string
}

// RunResolution is one run's effective skill set together with the selection
// that produced it.
type RunResolution struct {
	Resolution
	// Delivery is the effective skills_delivery mode of the selection.
	Delivery string
	// DeliverySource names the layer that set the mode.
	DeliverySource string
	// Source names the layer that set the role's list.
	Source string
	// Role is the bee whose gate this is. A load refusal names it, so an agent
	// can retry with the right role rather than guess.
	Role string
	// ExplicitEmpty reports an explicit `skills: []` opt-out.
	ExplicitEmpty bool
}

// Names returns the resolved skill names in resolution order (sorted by name),
// which is what a run records as offered.
func (r RunResolution) Names() []string {
	names := make([]string, 0, len(r.Skills))
	for _, skill := range r.Skills {
		names = append(names, skill.Name)
	}
	return names
}

// AdapterSkills returns the resolved set as the data an adapter receives: the
// same names, descriptions, and absolute frozen paths the advertisement
// carries, in resolution order. A script bee gets this as PASEKA_SKILLS in
// both delivery modes, so it never depends on the load command.
func (r RunResolution) AdapterSkills() []adapters.SkillRef {
	refs := make([]adapters.SkillRef, 0, len(r.Skills))
	for _, skill := range r.Skills {
		refs = append(refs, adapters.SkillRef{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.Path,
		})
	}
	return refs
}

// SectionContext names the run the advertisement addresses. In `command` mode
// the load command carries these identifiers so a bee can call the runtime for
// the body without being handed a path.
type SectionContext struct {
	TraceID string
	AgentID string
}

// LoadCommand renders the command a bee runs to obtain one skill's body. It is
// the one form the advertisement and the documentation share, so a colony
// cannot advertise a command the shell does not provide.
func LoadCommand(name string, ref SectionContext) string {
	return fmt.Sprintf("paseka skills load %s --trace %s --agent %s", name, ref.TraceID, ref.AgentID)
}

// Section renders the advertisement the runtime appends after a role's
// instructions. It carries the name and description of every resolved skill
// and, per the effective delivery mode, either the absolute path of the frozen
// SKILL.md or the command that serves it — never the body.
//
// `path` mode leaves the advertisement unchanged from the original design.
// `command` mode keeps the path out entirely, so the runtime is the only route
// to the body and a load is a recorded fact rather than an inference. A run
// with no skills has no section in either mode.
func (r RunResolution) Section(ref SectionContext) string {
	if len(r.Skills) == 0 {
		return ""
	}
	delivery := strings.TrimSpace(r.Delivery)
	if delivery == "" {
		delivery = colony.SkillsDeliveryPath
	}

	var b strings.Builder
	b.WriteString(SectionHeading)
	b.WriteString("\n\n")
	if delivery == colony.SkillsDeliveryCommand {
		b.WriteString("Each skill below is a reusable procedure this role may use. Load one with the command shown when the task calls for it: the command prints the procedure body.\n")
		for _, skill := range r.Skills {
			fmt.Fprintf(&b, "\n- %s: %s\n  load: %s\n", skill.Name, skill.Description, LoadCommand(skill.Name, ref))
		}
		return strings.TrimRight(b.String(), "\n")
	}

	b.WriteString("Each skill below is a reusable procedure this role may use. The procedure itself is not included here; read the file at its path when the task calls for it.\n")
	for _, skill := range r.Skills {
		fmt.Fprintf(&b, "\n- %s: %s\n  path: %s\n", skill.Name, skill.Description, skill.Path)
	}
	return strings.TrimRight(b.String(), "\n")
}

// ResolveRun resolves one run's effective skill set from the configuration
// tree in force. A missing or corrupt input blocks rather than falling back to
// the live store: the set a run records must be the set its configuration
// declared.
//
// The returned error covers an unreadable store, ledger, colony manifest, role,
// or profile, and every invalid resolution finding (a missing reference, an
// unmatched pattern, an invalid skill, a budget breach). Each finding names the
// offending skill or reference.
func ResolveRun(in RunInput) (RunResolution, error) {
	role := strings.TrimSpace(in.Role)
	if role == "" {
		return RunResolution{}, fmt.Errorf("skills: role is required to resolve a skill set")
	}
	configRoot := strings.TrimSpace(in.ConfigRoot)
	if configRoot == "" {
		return RunResolution{}, fmt.Errorf("skills: configuration root is required to resolve a skill set")
	}
	absRoot, err := filepath.Abs(configRoot)
	if err != nil {
		return RunResolution{}, fmt.Errorf("skills: resolve configuration root %s: %w", configRoot, err)
	}

	report, err := Scan(Dir(absRoot))
	if err != nil {
		return RunResolution{}, err
	}
	ledger, err := LoadLedgerFile(LedgerPath(absRoot))
	if err != nil {
		return RunResolution{}, err
	}
	manifest, err := colony.LoadColony(absRoot)
	if err != nil {
		return RunResolution{}, fmt.Errorf("skills: read colony configuration at %s: %w", absRoot, err)
	}
	bee, _, err := colony.LoadBee(absRoot, role)
	if err != nil {
		return RunResolution{}, fmt.Errorf("skills: read role %q: %w", role, err)
	}

	colonyProfile, homeProfile, err := loadSelectedProfile(in, absRoot)
	if err != nil {
		return RunResolution{}, err
	}
	selection := colony.EffectiveSkillsFor(manifest.Defaults, colonyProfile, homeProfile, role, bee)

	resolution := Resolve(report, ledger, selection.Refs)
	// A store-wide budget breach stops the run even for a bare reference: the
	// store is one configuration input, and an over-budget store is not made
	// acceptable by the fact that the role named something else. `Resolve` adds
	// these itself for a wildcard, so only the missing findings are appended.
	for _, issue := range report.StoreIssues() {
		if !containsIssue(resolution.Issues, issue) {
			resolution.Issues = append(resolution.Issues, issue)
		}
	}
	sortIssues(resolution.Issues)
	out := RunResolution{
		Resolution:     resolution,
		Delivery:       selection.Delivery,
		DeliverySource: selection.DeliverySource,
		Source:         selection.Source,
		Role:           role,
		ExplicitEmpty:  selection.ExplicitEmpty(),
	}
	if len(resolution.Issues) > 0 {
		return RunResolution{}, fmt.Errorf(
			"skills: bee %q: %s", role, joinIssues(resolution.Issues),
		)
	}
	return out, nil
}

// loadSelectedProfile reads the selected profile's overlays from the
// configuration tree in force. A frozen run reads the captured home profile
// config, so a later machine-local edit cannot change it; an unbound run reads
// the live home directory.
func loadSelectedProfile(in RunInput, configRoot string) (colony.ColonyProfile, colony.HomeProfileConfig, error) {
	profile := strings.TrimSpace(in.Profile)
	if profile == "" {
		return colony.ColonyProfile{}, colony.HomeProfileConfig{}, nil
	}

	var colonyProfile colony.ColonyProfile
	colonyPath := colony.ColonyProfilePath(configRoot, profile)
	if exists, err := fileExists(colonyPath); err != nil {
		return colony.ColonyProfile{}, colony.HomeProfileConfig{}, err
	} else if exists {
		loaded, err := colony.LoadColonyProfileFile(colonyPath)
		if err != nil {
			return colony.ColonyProfile{}, colony.HomeProfileConfig{}, fmt.Errorf("skills: %w", err)
		}
		colonyProfile = loaded
	}

	var homePath string
	if in.Frozen {
		homePath = filepath.Join(configRoot, "home", "profiles", profile, "config.yaml")
	} else if strings.TrimSpace(in.Slug) != "" {
		resolved, err := colony.HomeProfileConfigPath(in.Slug, profile)
		if err != nil {
			return colony.ColonyProfile{}, colony.HomeProfileConfig{}, fmt.Errorf("skills: %w", err)
		}
		homePath = resolved
	}
	var homeProfile colony.HomeProfileConfig
	if homePath != "" {
		if exists, err := fileExists(homePath); err != nil {
			return colony.ColonyProfile{}, colony.HomeProfileConfig{}, err
		} else if exists {
			loaded, err := colony.LoadHomeProfileConfigFile(homePath)
			if err != nil {
				return colony.ColonyProfile{}, colony.HomeProfileConfig{}, fmt.Errorf("skills: %w", err)
			}
			homeProfile = loaded
		}
	}
	return colonyProfile, homeProfile, nil
}

func fileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("skills: inspect %s: %w", path, err)
	}
	if info.IsDir() {
		return false, nil
	}
	return true, nil
}

func joinIssues(issues []Issue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "; ")
}

func containsIssue(issues []Issue, want Issue) bool {
	for _, issue := range issues {
		if issue == want {
			return true
		}
	}
	return false
}
