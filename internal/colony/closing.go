package colony

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/russ-p/paseka/internal/protocol"
	"gopkg.in/yaml.v3"
)

// DefaultClosingMaxRounds bounds closing rounds when the colony does not set
// max_rounds: one review plus one verification round.
const DefaultClosingMaxRounds = 2

// Today's literal merge-gate copy. A colony that does not author merge.task
// keeps this wording, so existing colonies see no change.
const (
	DefaultMergeTaskTitle = "Human review and merge"
	DefaultMergeTaskBody  = "Review accumulated changes and merge into the default branch."
)

// ClosingPolicy is the colony-declared trail closing policy. Every field is
// optional and the whole policy is disabled by default.
type ClosingPolicy struct {
	Enabled   bool        `yaml:"enabled,omitempty"`
	When      ClosingWhen `yaml:"when,omitempty"`
	Task      ClosingTask `yaml:"task,omitempty"`
	MaxRounds *int        `yaml:"max_rounds,omitempty"`
}

// ClosingWhen holds the named predicates that decide whether a closing round
// is seeded. Expressions are out of scope in v1.
type ClosingWhen struct {
	MinWorkTasks        int  `yaml:"min_work_tasks,omitempty"`
	HasIsolatedProposal bool `yaml:"has_isolated_proposal,omitempty"`
}

// UnmarshalYAML decodes the named predicate set and rejects an unknown name at
// load time instead of silently ignoring a typo that would change when a trail
// closes.
func (w *ClosingWhen) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == 0 || node.Tag == "!!null" {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("colony: closing.when must be a mapping of named predicates")
	}
	for i := 0; i < len(node.Content); i += 2 {
		key := strings.TrimSpace(node.Content[i].Value)
		value := node.Content[i+1]
		switch key {
		case "min_work_tasks":
			if err := value.Decode(&w.MinWorkTasks); err != nil {
				return fmt.Errorf("colony: closing.when.min_work_tasks: %w", err)
			}
		case "has_isolated_proposal":
			if err := value.Decode(&w.HasIsolatedProposal); err != nil {
				return fmt.Errorf("colony: closing.when.has_isolated_proposal: %w", err)
			}
		case "":
			return fmt.Errorf("colony: closing.when: predicate name is required")
		default:
			return fmt.Errorf("colony: closing.when: unknown predicate %q (supported: min_work_tasks, has_isolated_proposal)", key)
		}
	}
	return nil
}

// ClosingTask is the template the runtime turns into one closing task. It is
// not a ledger task: the runtime owns the task id, phase, and dependencies.
type ClosingTask struct {
	Bee    string                    `yaml:"bee,omitempty"`
	Intent string                    `yaml:"intent,omitempty"`
	Title  string                    `yaml:"title,omitempty"`
	Review protocol.TaskReviewPolicy `yaml:"review,omitempty"`
	Params map[string]any            `yaml:"params,omitempty"`
	// Phase exists only so a colony that tries to author one is rejected at
	// load: the runtime assigns the closing phase.
	Phase protocol.TaskPhase `yaml:"phase,omitempty"`
}

// MergePolicy configures the copy of the trace-level merge gate.
type MergePolicy struct {
	Task MergeTask `yaml:"task,omitempty"`
}

// MergeTask is the authored title and body of the merge gate. Both are
// text/templates rendered with the trail inventory.
type MergeTask struct {
	Title string `yaml:"title,omitempty"`
	Body  string `yaml:"body,omitempty"`
}

// TrailTask is one task row in the trail inventory available to the closing
// prompt and to gate copy templates.
type TrailTask struct {
	TaskID    string
	Title     string
	Phase     protocol.TaskPhase
	Intent    string
	Status    protocol.TaskStatus
	Commit    string
	Summary   string
	DependsOn []string
}

// TrailInventory is the structured view of a trail handed to the closing task
// and to the merge gate copy renderer. It is not a transcript.
type TrailInventory struct {
	TraceID    string
	TraceTitle string
	Tasks      []TrailTask
}

// Configured reports whether the colony authored any closing field, including
// the disabled-but-present case. An empty block is not pinned.
func (p ClosingPolicy) Configured() bool {
	return p.Enabled ||
		p.When.MinWorkTasks != 0 ||
		p.When.HasIsolatedProposal ||
		p.Task.Bee != "" ||
		p.Task.Intent != "" ||
		p.Task.Title != "" ||
		strings.TrimSpace(string(p.Task.Review)) != "" ||
		len(p.Task.Params) > 0 ||
		strings.TrimSpace(string(p.Task.Phase)) != "" ||
		p.MaxRounds != nil
}

// ResolvedMaxRounds returns the configured round cap, or the platform default.
func (p ClosingPolicy) ResolvedMaxRounds() int {
	if p.MaxRounds == nil {
		return DefaultClosingMaxRounds
	}
	return *p.MaxRounds
}

// Resolved applies the platform and colony defaults a pinned policy should
// carry explicitly: the round cap and the effective closing bee. A policy that
// was never authored stays zero and is not pinned at all.
func (p ClosingPolicy) Resolved(defaults Defaults) ClosingPolicy {
	if p.MaxRounds == nil {
		rounds := DefaultClosingMaxRounds
		p.MaxRounds = &rounds
	}
	if strings.TrimSpace(p.Task.Bee) == "" {
		p.Task.Bee = EffectiveTaskBee("", defaults)
	}
	return p
}

// Matches evaluates the v1 predicates against a ledger summary. An unset
// min_work_tasks imposes no minimum, and has_isolated_proposal only bites when
// it is true.
func (p ClosingPolicy) Matches(workTasks int, hasIsolatedProposal bool) bool {
	if p.When.MinWorkTasks > 0 && workTasks < p.When.MinWorkTasks {
		return false
	}
	if p.When.HasIsolatedProposal && !hasIsolatedProposal {
		return false
	}
	return true
}

// Validate checks the closing policy at load time. bees may be nil when the
// caller only wants the static checks; a non-nil map (even an empty one) makes
// an enabled policy prove that its bee exists and can handle task.ready.
func (p ClosingPolicy) Validate(bees map[string]Bee, defaults Defaults) error {
	if p.When.MinWorkTasks < 0 {
		return fmt.Errorf("colony: closing.when.min_work_tasks must not be negative (got %d)", p.When.MinWorkTasks)
	}

	review := strings.TrimSpace(string(p.Task.Review))
	switch protocol.TaskReviewPolicy(review) {
	case "", protocol.TaskReviewNone, protocol.TaskReviewRequired:
	case protocol.TaskReviewFinal:
		return fmt.Errorf("colony: closing.task.review: review %q is not allowed in the closing task template (the merge gate is derived from a final review task)", review)
	default:
		return fmt.Errorf("colony: closing.task.review: unknown review policy %q (expected none or required)", review)
	}

	if phase := strings.TrimSpace(string(p.Task.Phase)); phase != "" {
		return fmt.Errorf("colony: closing.task.phase: phase %q must not be set in the closing task template (the runtime assigns closing)", phase)
	}

	if err := ValidateRunParams(p.Task.Params); err != nil {
		return fmt.Errorf("colony: closing.task.params: %w", err)
	}

	if p.MaxRounds != nil && *p.MaxRounds < 1 {
		return fmt.Errorf("colony: closing.max_rounds must be at least 1 (got %d)", *p.MaxRounds)
	}

	if !p.Enabled || bees == nil {
		return nil
	}
	beeName := EffectiveTaskBee(p.Task.Bee, defaults)
	bee, ok := bees[beeName]
	if !ok {
		return fmt.Errorf("colony: closing.task.bee: bee %q is not defined", beeName)
	}
	if !bee.CanHandleTaskReady() {
		return fmt.Errorf("colony: closing.task.bee: bee %q cannot handle task.ready dispatches", beeName)
	}
	return nil
}

// Configured reports whether the colony authored any merge copy.
func (p MergePolicy) Configured() bool {
	return strings.TrimSpace(p.Task.Title) != "" || strings.TrimSpace(p.Task.Body) != ""
}

// ValidatePolicies checks the closing and merge blocks against the colony's
// committed bee definitions. An enabled closing policy must name a bee that
// exists and can run a task.ready dispatch.
func (c Colony) ValidatePolicies(colonyRoot string) error {
	var bees map[string]Bee
	if c.Closing.Enabled {
		loaded, err := LoadAllBees(colonyRoot)
		if err != nil {
			return err
		}
		bees = loaded
	}
	if err := c.Closing.Validate(bees, c.Defaults); err != nil {
		return err
	}
	return c.Merge.Validate()
}

// Validate parses and renders both templates against an empty trail inventory,
// so a malformed gate copy fails colony load rather than the review screen.
func (p MergePolicy) Validate() error {
	if _, err := p.RenderTitle(TrailInventory{}); err != nil {
		return fmt.Errorf("colony: merge.task.title: %w", err)
	}
	if _, err := p.RenderBody(TrailInventory{}); err != nil {
		return fmt.Errorf("colony: merge.task.body: %w", err)
	}
	return nil
}

// RenderTitle renders the configured gate title, or empty when unset.
func (p MergePolicy) RenderTitle(inv TrailInventory) (string, error) {
	return renderCopy("merge.task.title", p.Task.Title, inv)
}

// RenderBody renders the configured gate body, or empty when unset.
func (p MergePolicy) RenderBody(inv TrailInventory) (string, error) {
	return renderCopy("merge.task.body", p.Task.Body, inv)
}

// ResolvedTitle returns the rendered gate title, falling back to today's copy
// when the colony authored none.
func (p MergePolicy) ResolvedTitle(inv TrailInventory) (string, error) {
	title, err := p.RenderTitle(inv)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(title) == "" {
		return DefaultMergeTaskTitle, nil
	}
	return title, nil
}

// ResolvedBody returns the rendered gate body, falling back to today's copy
// when the colony authored none.
func (p MergePolicy) ResolvedBody(inv TrailInventory) (string, error) {
	body, err := p.RenderBody(inv)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(body) == "" {
		return DefaultMergeTaskBody, nil
	}
	return body, nil
}

func renderCopy(name, body string, inv TrailInventory) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, inv); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	return buf.String(), nil
}
