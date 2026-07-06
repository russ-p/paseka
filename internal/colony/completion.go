package colony

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/protocol"
)

// CompletionRule requires a bee run to emit a minimum number of domain events.
type CompletionRule struct {
	Type      string   `yaml:"type"`
	KindOneOf []string `yaml:"kind_one_of,omitempty"`
	Count     int      `yaml:"count,omitempty"`
}

// CompletionContract declares post-run event requirements for a bee.
type CompletionContract struct {
	Required []CompletionRule `yaml:"required,omitempty"`
}

// ValidateCompletionContract checks event rules at load time.
func (c CompletionContract) ValidateCompletionContract(beeRole string) error {
	for i, rule := range c.Required {
		if strings.TrimSpace(rule.Type) == "" {
			return fmt.Errorf("colony: bee %q completion_contract.required[%d]: type is required", beeRole, i)
		}
		typ := protocol.EventType(rule.Type)
		if !protocol.IsDomainEvent(typ) {
			return fmt.Errorf("colony: bee %q completion_contract.required[%d]: invalid type %q", beeRole, i, rule.Type)
		}
		if len(rule.KindOneOf) == 0 {
			return fmt.Errorf("colony: bee %q completion_contract.required[%d]: kind_one_of is required", beeRole, i)
		}
	}
	return nil
}

// ValidateRunEvents checks whether emitted domain events satisfy the contract.
func (c CompletionContract) ValidateRunEvents(events []protocol.Event) error {
	for _, rule := range c.Required {
		if err := rule.validate(events); err != nil {
			return err
		}
	}
	return nil
}

func (r CompletionRule) validate(events []protocol.Event) error {
	want := r.Count
	if want <= 0 {
		want = 1
	}
	allowed := make(map[string]struct{}, len(r.KindOneOf))
	for _, k := range r.KindOneOf {
		allowed[strings.TrimSpace(k)] = struct{}{}
	}
	got := 0
	for _, ev := range events {
		if string(ev.Type) != strings.TrimSpace(r.Type) {
			continue
		}
		kind := protocol.PayloadKind(ev.Payload)
		if _, ok := allowed[kind]; !ok {
			continue
		}
		got++
	}
	if got < want {
		return fmt.Errorf("completion contract: expected at least %d %s event(s) with kind in %v, got %d",
			want, r.Type, r.KindOneOf, got)
	}
	if got > want {
		return fmt.Errorf("completion contract: expected exactly %d %s event(s) with kind in %v, got %d",
			want, r.Type, r.KindOneOf, got)
	}
	return nil
}
