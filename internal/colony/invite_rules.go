package colony

import (
	"fmt"
	"strings"
)

// AutoInviteRule publishes a pending session.invite when a bus event matches.
type AutoInviteRule struct {
	When   EventRule            `yaml:"when"`
	Match  map[string]string    `yaml:"match,omitempty"`
	Invite AutoInviteInviteSpec `yaml:"invite"`
	Dedupe []string             `yaml:"dedupe,omitempty"`
}

// AutoInviteInviteSpec maps trigger event fields to session.invite payload fields.
type AutoInviteInviteSpec struct {
	Bee         InviteStringField `yaml:"bee"`
	Intent      InviteStringField `yaml:"intent,omitempty"`
	Task        InviteTaskField   `yaml:"task"`
	Status      string            `yaml:"status,omitempty"`
	ArtifactRef InviteStringField `yaml:"artifactRef,omitempty"`
	DoneWhen    *InviteDoneWhen   `yaml:"done_when,omitempty"`
}

// InviteDoneWhen declares when an accepted invite is completed or marked incomplete.
type InviteDoneWhen struct {
	When           EventRule         `yaml:"when"`
	Match          map[string]string `yaml:"match,omitempty"`
	RequireFile    InviteStringField `yaml:"require_file"`
	SetArtifactRef InviteStringField `yaml:"set_artifact_ref,omitempty"`
}

// InviteStringField copies a string from the trigger payload or uses a default.
type InviteStringField struct {
	From    string `yaml:"from,omitempty"`
	Default string `yaml:"default,omitempty"`
}

// InviteTaskField builds invite task text from trace history or trigger fields.
type InviteTaskField struct {
	FromTraceKind  string `yaml:"from_trace_kind,omitempty"`
	FromTraceField string `yaml:"from_trace_field,omitempty"`
	Prefix         string `yaml:"prefix,omitempty"`
	FallbackFrom   string `yaml:"fallback_from,omitempty"`
	From           string `yaml:"from,omitempty"`
	Default        string `yaml:"default,omitempty"`
}

// SampleAutoInviteRules returns fixture rules for unit tests (generic kinds).
// Colonies enable auto-invite by adding rules to .paseka/colony.yaml.
func SampleAutoInviteRules() []AutoInviteRule {
	return []AutoInviteRule{
		{
			When: EventRule{Type: "SIGNAL", Kind: "review.needed"},
			Match: map[string]string{
				"decision": "session",
			},
			Invite: AutoInviteInviteSpec{
				Bee:    InviteStringField{From: "bee", Default: "drone"},
				Intent: InviteStringField{From: "intent", Default: "grilling"},
				Task: InviteTaskField{
					FromTraceKind:  "review.requested",
					FromTraceField: "title",
					Prefix:         "Review: ",
					FallbackFrom:   "rationale",
					Default:        "Review item",
				},
				Status: "pending",
				DoneWhen: &InviteDoneWhen{
					When:           EventRule{Type: "SIGNAL", Kind: "doc.ready"},
					RequireFile:    InviteStringField{From: "ref"},
					SetArtifactRef: InviteStringField{From: "ref"},
				},
			},
			Dedupe: []string{"bee", "intent"},
		},
		{
			When: EventRule{Type: "SIGNAL", Kind: "doc.ready"},
			Invite: AutoInviteInviteSpec{
				Bee:    InviteStringField{Default: "drone"},
				Intent: InviteStringField{Default: "breakdown"},
				ArtifactRef: InviteStringField{
					From: "ref",
				},
				Task: InviteTaskField{
					From:    "ref",
					Prefix:  "Break down ",
					Default: "Break down doc",
				},
				Status: "pending",
			},
			Dedupe: []string{"intent", "artifactRef"},
		},
	}
}

// DefaultAutoInviteRules returns the stock feature-ideation rules for new colonies.
func DefaultAutoInviteRules() []AutoInviteRule {
	return []AutoInviteRule{
		{
			When: EventRule{Type: "SIGNAL", Kind: "feature.classified"},
			Match: map[string]string{
				"decision": "grill",
			},
			Invite: AutoInviteInviteSpec{
				Bee:    InviteStringField{From: "bee", Default: "drone"},
				Intent: InviteStringField{From: "intent", Default: "grilling"},
				Task: InviteTaskField{
					FromTraceKind:  "feature.requested",
					FromTraceField: "title",
					Prefix:         "Grill feature: ",
					FallbackFrom:   "rationale",
					Default:        "Grill feature",
				},
				Status: "pending",
				DoneWhen: &InviteDoneWhen{
					When:           EventRule{Type: "SIGNAL", Kind: "spec.ready"},
					RequireFile:    InviteStringField{From: "ref"},
					SetArtifactRef: InviteStringField{From: "ref"},
				},
			},
			Dedupe: []string{"bee", "intent"},
		},
		{
			When: EventRule{Type: "SIGNAL", Kind: "spec.ready"},
			Invite: AutoInviteInviteSpec{
				Bee:    InviteStringField{Default: "drone"},
				Intent: InviteStringField{Default: "breakdown"},
				ArtifactRef: InviteStringField{
					From: "ref",
				},
				Task: InviteTaskField{
					From:    "ref",
					Prefix:  "Break down ",
					Default: "Break down spec",
				},
				Status: "pending",
			},
			Dedupe: []string{"intent", "artifactRef"},
		},
	}
}

// ValidateAutoInvites checks colony-level auto-invite rules.
func (c Colony) ValidateAutoInvites() error {
	for i, rule := range c.AutoInvites {
		if _, err := rule.When.EventType(); err != nil {
			return fmt.Errorf("colony: auto_invites[%d].when: %w", i, err)
		}
		if err := rule.Invite.validate(i); err != nil {
			return err
		}
		if rule.Invite.DoneWhen != nil {
			if err := rule.Invite.DoneWhen.validate(i); err != nil {
				return err
			}
		}
		for j, key := range rule.Dedupe {
			switch strings.TrimSpace(key) {
			case "bee", "intent", "artifactRef":
			default:
				return fmt.Errorf("colony: auto_invites[%d].dedupe[%d]: unknown field %q", i, j, key)
			}
		}
	}
	return nil
}

func (d InviteDoneWhen) validate(ruleIdx int) error {
	if _, err := d.When.EventType(); err != nil {
		return fmt.Errorf("colony: auto_invites[%d].invite.done_when.when: %w", ruleIdx, err)
	}
	if strings.TrimSpace(d.RequireFile.From) == "" {
		return fmt.Errorf("colony: auto_invites[%d].invite.done_when.require_file.from: required", ruleIdx)
	}
	return nil
}

func (s AutoInviteInviteSpec) validate(ruleIdx int) error {
	if strings.TrimSpace(s.Bee.From) == "" && strings.TrimSpace(s.Bee.Default) == "" {
		return fmt.Errorf("colony: auto_invites[%d].invite.bee: from or default required", ruleIdx)
	}
	if !s.Task.resolvable() {
		return fmt.Errorf("colony: auto_invites[%d].invite.task: from_trace, from, fallback_from, or default required", ruleIdx)
	}
	status := strings.TrimSpace(s.Status)
	if status == "" {
		return nil
	}
	switch status {
	case InviteStatusPending, InviteStatusAccepted, InviteStatusCancelled, InviteStatusCompleted, InviteStatusIncomplete:
	default:
		return fmt.Errorf("colony: auto_invites[%d].invite.status: invalid %q", ruleIdx, status)
	}
	return nil
}

func (t InviteTaskField) resolvable() bool {
	if strings.TrimSpace(t.FromTraceKind) != "" && strings.TrimSpace(t.FromTraceField) != "" {
		return true
	}
	if strings.TrimSpace(t.From) != "" {
		return true
	}
	if strings.TrimSpace(t.FallbackFrom) != "" {
		return true
	}
	return strings.TrimSpace(t.Default) != ""
}
