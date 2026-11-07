package colony

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// SkillsDeliveryPath advertises each resolved skill's absolute path. A bee
	// reads the file itself and the adapter's transcript is the evidence.
	SkillsDeliveryPath = "path"
	// SkillsDeliveryCommand advertises the `paseka skills load` command instead
	// of a path, so the runtime's load record is the evidence.
	SkillsDeliveryCommand = "command"
)

// SkillSelection is a presence-aware list of skill references. Absent and empty
// mean different things to the per-role gate, so a plain slice cannot carry
// this: absent inherits the colony default, an explicit empty list opts out,
// and a non-empty list is the whole set for the role. The tri-state survives
// YAML parsing because UnmarshalYAML records whether the key was there.
type SkillSelection struct {
	Present bool
	Refs    []string
}

// UnmarshalYAML records presence separately from the contents. A `skills:` key
// with no value is treated as absent; `skills: []` is the explicit opt-out.
func (s *SkillSelection) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			*s = SkillSelection{}
			return nil
		}
		return fmt.Errorf("skills must be a list of references, not %s", node.Tag)
	case yaml.SequenceNode:
		refs := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("skills must be a list of references")
			}
			refs = append(refs, item.Value)
		}
		*s = SkillSelection{Present: true, Refs: refs}
		return nil
	}
	return fmt.Errorf("skills must be a list of references")
}

// MarshalYAML writes the selection back as a YAML list, or as nothing when the
// key is absent, so a config rewrite (trail apply, init, Nuc) does not turn the
// presence-aware field into a `present:`/`refs:` mapping.
func (s SkillSelection) MarshalYAML() (any, error) {
	if !s.Present {
		return nil, nil
	}
	if s.Refs == nil {
		return []string{}, nil
	}
	return s.Refs, nil
}

// Empty reports whether the selection is present and explicitly empty.
func (s SkillSelection) Empty() bool {
	return s.Present && len(s.Refs) == 0
}

// EffectiveSkills is the skill gate for one role after the documented
// precedence: home profile > colony profile > committed, with a role's list
// replacing the default rather than extending it. Source names the layer that
// set the list so an operator can see why a bee has what it has.
type EffectiveSkills struct {
	Refs           []string
	Present        bool
	Source         string
	Delivery       string
	DeliverySource string
}

// ExplicitEmpty reports whether the effective list is an explicit opt-out,
// which doctor warns about but does not treat as an error.
func (e EffectiveSkills) ExplicitEmpty() bool {
	return e.Present && len(e.Refs) == 0
}

// EffectiveSkills resolves one role's skill selection. A role's list replaces
// the colony default; when neither is declared the role receives nothing.
//
// The same tri-state applies in a profile: an absent key leaves the lower
// layer, an empty list opts out, and a list replaces. A home profile wins over
// a colony profile, which wins over the committed value.
func (c Context) EffectiveSkills(role string, bee Bee) (EffectiveSkills, error) {
	defaults := c.defaults
	if !c.manifestLoaded {
		if c.ConfigRootPath() != "" {
			manifest, err := LoadColony(c.ConfigRootPath())
			if err != nil {
				return EffectiveSkills{}, err
			}
			defaults = manifest.Defaults
		}
	}
	return EffectiveSkillsFor(defaults, c.colonyProfile, c.homeProfile, role, bee), nil
}

// EffectiveSkillsFor resolves the gate from already-loaded layers. It exists so
// a resolver that reads a trail's frozen configuration — where the profile
// overlays come from the captured tree rather than the live context — can apply
// exactly the same precedence without a second implementation.
func EffectiveSkillsFor(
	defaults Defaults,
	colonyProfile ColonyProfile,
	homeProfile HomeProfileConfig,
	role string,
	bee Bee,
) EffectiveSkills {
	out := EffectiveSkills{
		Delivery:       SkillsDeliveryPath,
		DeliverySource: "default",
	}
	switch {
	case strings.TrimSpace(homeProfile.SkillsDelivery) != "":
		out.Delivery = strings.TrimSpace(homeProfile.SkillsDelivery)
		out.DeliverySource = "home profile"
	case strings.TrimSpace(colonyProfile.SkillsDelivery) != "":
		out.Delivery = strings.TrimSpace(colonyProfile.SkillsDelivery)
		out.DeliverySource = "colony profile"
	case strings.TrimSpace(defaults.SkillsDelivery) != "":
		out.Delivery = strings.TrimSpace(defaults.SkillsDelivery)
		out.DeliverySource = "defaults"
	}

	if sel, ok := homeProfile.roleSkills(role); ok {
		out.Refs = cloneRefs(sel.Refs)
		out.Present = true
		out.Source = "home profile"
		return out
	}
	if sel, ok := colonyProfile.roleSkills(role); ok {
		out.Refs = cloneRefs(sel.Refs)
		out.Present = true
		out.Source = "colony profile"
		return out
	}
	if bee.Skills.Present {
		out.Refs = cloneRefs(bee.Skills.Refs)
		out.Present = true
		out.Source = "bee"
		return out
	}
	switch {
	case homeProfile.Skills.Present:
		out.Refs = cloneRefs(homeProfile.Skills.Refs)
		out.Present = true
		out.Source = "home profile defaults"
	case colonyProfile.Skills.Present:
		out.Refs = cloneRefs(colonyProfile.Skills.Refs)
		out.Present = true
		out.Source = "colony profile defaults"
	case defaults.Skills.Present:
		out.Refs = cloneRefs(defaults.Skills.Refs)
		out.Present = true
		out.Source = "defaults"
	default:
		out.Source = "none"
	}
	return out
}

// ValidateSkillsDelivery rejects unknown delivery modes. Absent is valid and
// means `path`.
func ValidateSkillsDelivery(value string) error {
	switch strings.TrimSpace(value) {
	case "", SkillsDeliveryPath, SkillsDeliveryCommand:
		return nil
	default:
		return fmt.Errorf("skills_delivery must be path or command (got %q)", value)
	}
}

// skillSelectionFromAny converts a decoded YAML value into a presence-aware
// selection. It exists for the profile maps, whose keys are validated as raw
// maps before the typed decode; a nil value is an absent key.
func skillSelectionFromAny(v any) (SkillSelection, error) {
	if v == nil {
		return SkillSelection{}, nil
	}
	items, ok := v.([]any)
	if !ok {
		return SkillSelection{}, fmt.Errorf("must be a list of references")
	}
	refs := make([]string, 0, len(items))
	for _, item := range items {
		ref, ok := item.(string)
		if !ok {
			return SkillSelection{}, fmt.Errorf("must be a list of references")
		}
		refs = append(refs, ref)
	}
	return SkillSelection{Present: true, Refs: refs}, nil
}

func cloneRefs(refs []string) []string {
	if refs == nil {
		return nil
	}
	out := make([]string, len(refs))
	copy(out, refs)
	return out
}
