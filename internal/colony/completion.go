package colony

import (
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
)

// CompletionRule requires a bee run to emit a minimum number of domain events.
type CompletionRule struct {
	Type      string   `yaml:"type"`
	KindOneOf []string `yaml:"kind_one_of,omitempty"`
	Count     int      `yaml:"count,omitempty"`
}

// CompletionContract declares post-run event requirements for a bee.
//
// Required is the default contract, applied to every intent without an
// override. Intents maps a normalized intent name to the contract that run
// must satisfy instead, so one role can demand a different output per intent
// without the runtime special-casing that role.
type CompletionContract struct {
	Required []CompletionRule              `yaml:"required,omitempty"`
	Intents  map[string]CompletionOverride `yaml:"intents,omitempty"`
}

// CompletionOverride is the contract a run under one intent must satisfy.
type CompletionOverride struct {
	Required []CompletionRule `yaml:"required,omitempty"`
}

// ValidateCompletionContract checks event rules at load time.
func (c CompletionContract) ValidateCompletionContract(beeRole string) error {
	if err := validateCompletionRules(c.Required, fmt.Sprintf("colony: bee %q completion_contract.required", beeRole)); err != nil {
		return err
	}
	keys := make([]string, 0, len(c.Intents))
	for key := range c.Intents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := strings.TrimSpace(strings.ToLower(key))
		if normalized == "" {
			return fmt.Errorf("colony: bee %q completion_contract.intents: intent key is required", beeRole)
		}
		if _, ok := seen[normalized]; ok {
			return fmt.Errorf("colony: bee %q completion_contract.intents: duplicate intent %q", beeRole, normalized)
		}
		seen[normalized] = struct{}{}
		override := c.Intents[key]
		if len(override.Required) == 0 {
			return fmt.Errorf("colony: bee %q completion_contract.intents[%q]: at least one rule is required", beeRole, normalized)
		}
		prefix := fmt.Sprintf("colony: bee %q completion_contract.intents[%q].required", beeRole, normalized)
		if err := validateCompletionRules(override.Required, prefix); err != nil {
			return err
		}
	}
	return nil
}

func validateCompletionRules(rules []CompletionRule, prefix string) error {
	for i, rule := range rules {
		if strings.TrimSpace(rule.Type) == "" {
			return fmt.Errorf("%s[%d]: type is required", prefix, i)
		}
		typ := protocol.EventType(rule.Type)
		if !protocol.IsDomainEvent(typ) {
			return fmt.Errorf("%s[%d]: invalid type %q", prefix, i, rule.Type)
		}
		if len(rule.KindOneOf) == 0 {
			return fmt.Errorf("%s[%d]: kind_one_of is required", prefix, i)
		}
	}
	return nil
}

// ForIntent returns the contract a run with the given normalized intent must
// satisfy. An intent without an override falls back to the role's default
// contract, and an empty intent name cannot select an override because the
// runtime always passes the run's normalized intent.
func (c CompletionContract) ForIntent(intent string) CompletionContract {
	intent = strings.TrimSpace(strings.ToLower(intent))
	if intent != "" {
		for key, override := range c.Intents {
			if strings.TrimSpace(strings.ToLower(key)) == intent {
				return CompletionContract{Required: override.Required}
			}
		}
	}
	return CompletionContract{Required: c.Required}
}

// ValidateRunEvents checks whether emitted domain events satisfy the contract.
func (c CompletionContract) ValidateRunEvents(events []protocol.Event) error {
	for _, obs := range c.ObserveRunEvents(events) {
		if err := obs.validate(); err != nil {
			return err
		}
	}
	return nil
}

// CompletionObservation is one declared rule measured against a run's domain
// events. Want is the effective required count, so a reader can tell what the
// rule asked for without re-deriving the default.
type CompletionObservation struct {
	Type      string
	KindOneOf []string
	Want      int
	Got       int
}

// Satisfied reports whether the rule was met exactly.
func (o CompletionObservation) Satisfied() bool { return o.Got == o.Want }

// ObserveRunEvents measures every declared rule against the run's domain
// events. It never short-circuits: a verdict records all observations, not
// only the first rule that failed.
func (c CompletionContract) ObserveRunEvents(events []protocol.Event) []CompletionObservation {
	observations := make([]CompletionObservation, 0, len(c.Required))
	for _, rule := range c.Required {
		observations = append(observations, rule.observe(events))
	}
	return observations
}

// EffectiveRules returns the declared rules with defaults applied: a rule
// without a positive count requires exactly one matching event.
func (c CompletionContract) EffectiveRules() []CompletionRule {
	rules := make([]CompletionRule, 0, len(c.Required))
	for _, rule := range c.Required {
		rules = append(rules, CompletionRule{
			Type:      strings.TrimSpace(rule.Type),
			KindOneOf: trimKinds(rule.KindOneOf),
			Count:     rule.count(),
		})
	}
	return rules
}

func trimKinds(kinds []string) []string {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, strings.TrimSpace(kind))
	}
	return out
}

func (r CompletionRule) count() int {
	if r.Count <= 0 {
		return 1
	}
	return r.Count
}

func (r CompletionRule) observe(events []protocol.Event) CompletionObservation {
	obs := CompletionObservation{
		Type:      strings.TrimSpace(r.Type),
		KindOneOf: trimKinds(r.KindOneOf),
		Want:      r.count(),
	}
	allowed := make(map[string]struct{}, len(r.KindOneOf))
	for _, k := range r.KindOneOf {
		allowed[strings.TrimSpace(k)] = struct{}{}
	}
	for _, ev := range events {
		if string(ev.Type) != obs.Type {
			continue
		}
		kind := protocol.PayloadKind(ev.Payload)
		if _, ok := allowed[kind]; !ok {
			continue
		}
		obs.Got++
	}
	return obs
}

func (o CompletionObservation) validate() error {
	if o.Got < o.Want {
		return fmt.Errorf("completion contract: expected at least %d %s event(s) with kind in %v, got %d",
			o.Want, o.Type, o.KindOneOf, o.Got)
	}
	if o.Got > o.Want {
		return fmt.Errorf("completion contract: expected exactly %d %s event(s) with kind in %v, got %d",
			o.Want, o.Type, o.KindOneOf, o.Got)
	}
	return nil
}
