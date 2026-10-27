package trailconfig

import (
	"fmt"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// ProcedureWork is one accepted task that must still dispatch after the
// candidate configuration becomes active. It is the common shape of a ledger
// task and of the task a fork carries into its new trail.
type ProcedureWork struct {
	TaskID string
	Title  string
	Body   string
	Bee    string
	Sector string
	Intent string
	Review protocol.TaskReviewPolicy
}

// RemainingProcedureWork returns the accepted work that has not started yet and
// therefore must run under the candidate. Blocked and failed tasks are included
// because a later readiness event or an explicit retry can dispatch them again;
// the synthetic final-review task is a trace gate rather than procedure work.
func RemainingProcedureWork(snapshot taskledger.TraceSnapshot) []ProcedureWork {
	work := make([]ProcedureWork, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		if taskledger.IsFinalReviewTask(task) {
			continue
		}
		switch task.Status {
		case protocol.TaskStatusPlanned, protocol.TaskStatusReady,
			protocol.TaskStatusBlocked, protocol.TaskStatusFailed:
		default:
			continue
		}
		work = append(work, ProcedureWork{
			TaskID: task.TaskID,
			Title:  task.Title,
			Body:   task.Body,
			Bee:    task.Bee,
			Sector: task.Sector,
			Intent: task.Intent,
			Review: task.Review,
		})
	}
	return work
}

// ValidateProcedure validates every part of a complete candidate procedure
// before it can be recorded or activated.
//
// policy is the canonical colony policy of the configuration the candidate
// replaces; the candidate may change the procedure but not identity, transport,
// energy, or delivery. The check reads the candidate tree only: live colony
// files never fill a gap in a retained snapshot.
func ValidateProcedure(candidateRoot string, policy colony.Colony, work []ProcedureWork) error {
	root := strings.TrimSpace(candidateRoot)
	if root == "" {
		return fmt.Errorf("trailconfig: candidate root is required")
	}
	manifestPath := colony.PasekaPath(root, "colony.yaml")
	if _, err := os.Stat(manifestPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("trailconfig: candidate is missing .paseka/colony.yaml")
		}
		return fmt.Errorf("trailconfig: inspect candidate colony manifest: %w", err)
	}
	candidate, err := colony.LoadColony(root)
	if err != nil {
		return fmt.Errorf("trailconfig: candidate colony manifest is invalid: %w", err)
	}
	if err := validatePolicyUnchanged(policy, candidate); err != nil {
		return err
	}

	bees, err := colony.LoadAllBees(root)
	if err != nil {
		return fmt.Errorf("trailconfig: candidate bee definitions are invalid: %w", err)
	}
	if err := validateBeeTemplates(root, candidate, bees); err != nil {
		return err
	}
	if err := validateSkillReferences(root); err != nil {
		return err
	}
	for _, item := range work {
		if err := validateProcedureWork(candidate, bees, item); err != nil {
			return err
		}
	}
	return nil
}

// validateBeeTemplates checks the prompt and system templates dispatch will
// actually resolve for every bee, honouring the same overlay and default
// precedence. The adapter decides whether defaults apply: script bees run a
// command instead of a rendered prompt.
func validateBeeTemplates(root string, manifest colony.Colony, bees map[string]colony.Bee) error {
	for role, bee := range bees {
		_, localOverlay, err := colony.LoadBee(root, role)
		if err != nil {
			return fmt.Errorf("trailconfig: candidate bee %q is invalid: %w", role, err)
		}
		adapter, err := bee.ResolveAdapter()
		if err != nil {
			return fmt.Errorf("trailconfig: candidate bee %q is invalid: %w", role, err)
		}
		promptTemplate := strings.TrimSpace(localOverlay.PromptTemplate)
		if promptTemplate == "" {
			promptTemplate = strings.TrimSpace(bee.PromptTemplate)
		}
		if promptTemplate == "" && adapter != "script" {
			promptTemplate = strings.TrimSpace(manifest.Defaults.PromptTemplate)
		}
		if promptTemplate != "" {
			if err := validatePromptTemplateFile(root, promptTemplate); err != nil {
				return fmt.Errorf("trailconfig: candidate bee %q: %w", role, err)
			}
		}
		systemTemplate := colony.ResolvedSystemTemplate(bee, localOverlay, manifest.Defaults)
		if systemTemplate != "" {
			if err := validatePromptTemplateFile(root, systemTemplate); err != nil {
				return fmt.Errorf("trailconfig: candidate bee %q: %w", role, err)
			}
		}
	}
	return nil
}

// validateProcedureWork checks one task against the candidate's roles, sectors,
// and review policy.
func validateProcedureWork(manifest colony.Colony, bees map[string]colony.Bee, work ProcedureWork) error {
	beeRole := colony.EffectiveTaskBee(work.Bee, manifest.Defaults)
	bee, ok := bees[beeRole]
	if !ok {
		return fmt.Errorf("trailconfig: candidate removes bee %q needed by task %s: candidate has no bee %q", beeRole, work.TaskID, beeRole)
	}
	if err := bee.ValidateAdapterRequirements(); err != nil {
		return fmt.Errorf("trailconfig: candidate invalid for task %s: %w", work.TaskID, err)
	}
	if sector := strings.TrimSpace(work.Sector); sector != "" {
		if _, err := manifest.ResolveSector(sector); err != nil {
			return fmt.Errorf("trailconfig: candidate invalid for task %s: %w", work.TaskID, err)
		}
	}
	spec := protocol.TaskSpec{
		TaskID: work.TaskID,
		Title:  work.Title,
		Body:   work.Body,
		Bee:    beeRole,
		Sector: work.Sector,
		Intent: work.Intent,
		Review: work.Review,
	}
	return colony.ValidateTaskReviewPolicy(spec, bees, manifest.Defaults)
}
