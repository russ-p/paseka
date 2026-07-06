package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// DispatchMode selects how a subscription triggers bee execution.
type DispatchMode string

const (
	DispatchTask   DispatchMode = "task"
	DispatchDirect DispatchMode = "direct"
)

// EventRule identifies a bus event by top-level type and optional payload.kind.
type EventRule struct {
	Type string `yaml:"type"`
	Kind string `yaml:"kind,omitempty"`
}

// SubscriptionRule declares which events a bee listens to and how dispatch works.
type SubscriptionRule struct {
	EventRule `yaml:",inline"`
	Dispatch  DispatchMode `yaml:"dispatch,omitempty"`
}

// PublicationRule declares which domain events a bee may emit (advisory contract).
type PublicationRule struct {
	EventRule `yaml:",inline"`
}

// EventKey is a normalized match key for routing.
type EventKey struct {
	Type protocol.EventType
	Kind string
}

// EventType parses the rule type string into a protocol event type.
func (r EventRule) EventType() (protocol.EventType, error) {
	t := strings.TrimSpace(r.Type)
	if t == "" {
		return "", fmt.Errorf("colony: event rule missing type")
	}
	typ := protocol.EventType(t)
	if !protocol.IsDomainEvent(typ) {
		return "", fmt.Errorf("colony: invalid event type %q", t)
	}
	return typ, nil
}

// Matches reports whether an event matches this rule.
func (r EventRule) Matches(evType protocol.EventType, kind string) bool {
	ruleType, err := r.EventType()
	if err != nil {
		return false
	}
	if ruleType != evType {
		return false
	}
	ruleKind := strings.TrimSpace(r.Kind)
	if ruleKind == "" {
		return true
	}
	return ruleKind == strings.TrimSpace(kind)
}

// ResolvedDispatch returns the effective dispatch mode for a subscription rule.
func (s SubscriptionRule) ResolvedDispatch() DispatchMode {
	if s.Dispatch != "" {
		return s.Dispatch
	}
	if strings.HasPrefix(strings.TrimSpace(s.Kind), "task.") {
		return DispatchTask
	}
	return DispatchDirect
}

// LoadAllBees reads every .paseka/bees/<role>.yaml (excluding *.local.yaml).
func LoadAllBees(colonyRoot string) (map[string]Bee, error) {
	dir := BeesDir(colonyRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Bee{}, nil
		}
		return nil, fmt.Errorf("colony: list bees: %w", err)
	}
	out := make(map[string]Bee)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if !strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".local.yaml") {
			continue
		}
		role := strings.TrimSuffix(name, ".yaml")
		bee, _, err := LoadBee(colonyRoot, role)
		if err != nil {
			return nil, err
		}
		out[role] = bee
	}
	return out, nil
}

// ValidateEventRules checks subscription and publication rules at load time.
func (b Bee) ValidateEventRules() error {
	for i, sub := range b.Subscribes {
		if _, err := sub.EventType(); err != nil {
			return fmt.Errorf("colony: bee %q subscribes[%d]: %w", b.Role, i, err)
		}
		switch sub.ResolvedDispatch() {
		case DispatchTask, DispatchDirect:
		default:
			return fmt.Errorf("colony: bee %q subscribes[%d]: invalid dispatch %q", b.Role, i, sub.Dispatch)
		}
	}
	for i, pub := range b.Publishes {
		if _, err := pub.EventType(); err != nil {
			return fmt.Errorf("colony: bee %q publishes[%d]: %w", b.Role, i, err)
		}
	}
	return b.CompletionContract.ValidateCompletionContract(b.Role)
}

// CanHandleTaskReady reports whether a bee is allowed to execute task.ready dispatches.
// Empty subscribes means backward-compatible allow-all.
func (b Bee) CanHandleTaskReady() bool {
	if len(b.Subscribes) == 0 {
		return true
	}
	for _, sub := range b.Subscribes {
		if sub.ResolvedDispatch() != DispatchTask {
			continue
		}
		if sub.Matches(protocol.EventSignal, string(protocol.TaskEventReady)) {
			return true
		}
	}
	return false
}

// DirectSubscribers returns bee roles with a direct subscription matching the event.
func DirectSubscribers(bees map[string]Bee, evType protocol.EventType, kind string) []string {
	var roles []string
	for role, bee := range bees {
		for _, sub := range bee.Subscribes {
			if sub.ResolvedDispatch() != DispatchDirect {
				continue
			}
			if sub.Matches(evType, kind) {
				roles = append(roles, role)
				break
			}
		}
	}
	return roles
}

// DeclaresPublish reports whether a bee declares the given event in publishes (advisory).
func (b Bee) DeclaresPublish(evType protocol.EventType, kind string) bool {
	if len(b.Publishes) == 0 {
		return true
	}
	for _, pub := range b.Publishes {
		if pub.Matches(evType, kind) {
			return true
		}
	}
	return false
}

// BeeYAMLPath returns the path to a bee config file.
func BeeYAMLPath(colonyRoot, role string) string {
	return filepath.Join(BeesDir(colonyRoot), role+".yaml")
}
