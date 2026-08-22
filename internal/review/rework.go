package review

import (
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/taskledger"
	"github.com/paseka/paseka/internal/tasks"
)

const reworkTaskTitle = "Apply review comments"

// EnsureRequestChanges validates that a final-gate Request changes can plan rework.
func EnsureRequestChanges(snap taskledger.TraceSnapshot) (taskledger.TaskSnapshot, error) {
	source, ok := taskledger.LastCompletedIsolatedProposalTask(snap)
	if !ok {
		return taskledger.TaskSnapshot{}, fmt.Errorf("request changes: no completed isolated code.proposal on this trail")
	}
	if inflight, ok := taskledger.InFlightRework(snap); ok {
		return taskledger.TaskSnapshot{}, fmt.Errorf("request changes: rework already in flight (%s is %s)", inflight.TaskID, inflight.Status)
	}
	return source, nil
}

// ReworkTaskBody builds the AFK task body: comb pointer plus original intent.
func ReworkTaskBody(source taskledger.TaskSnapshot) string {
	var b strings.Builder
	b.WriteString("Apply Beekeeper review comments from the trail comb file `review-comments.md` under ArtifactsDir. ")
	b.WriteString("Open that file for path, line, and body details — do not wait for the full review to appear in Insights.\n\n")
	b.WriteString("Keep edits on this Flight Trail worktree. Do not merge to the default branch.\n\n")
	b.WriteString("Original task: `")
	b.WriteString(source.TaskID)
	b.WriteString("`")
	if title := strings.TrimSpace(source.Title); title != "" {
		b.WriteString(" — ")
		b.WriteString(title)
	}
	b.WriteString("\n")
	if body := strings.TrimSpace(source.Body); body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		b.WriteString("\n")
	}
	return b.String()
}

// PlanReworkTask publishes task.plan + task.ready for a new non-final rework task.
// Events are publish-only so the hive reactor applies them and dispatches.
func PlanReworkTask(ctx context.Context, pub bus.Publisher, source taskledger.TaskSnapshot, traceID, agentID, colonyRoot string) (string, error) {
	if pub == nil {
		return "", fmt.Errorf("nats client is required")
	}
	taskID, err := colony.NewTaskID()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(agentID) == "" {
		agentID = "human"
	}
	defaults := colony.Defaults{}
	if strings.TrimSpace(colonyRoot) != "" {
		if manifest, err := colony.LoadColony(colonyRoot); err == nil {
			defaults = manifest.Defaults
		}
	}
	bee := colony.EffectiveTaskBee(source.Bee, defaults)
	body := ReworkTaskBody(source)
	spec := protocol.TaskSpec{
		TaskID: taskID,
		Title:  reworkTaskTitle,
		Body:   body,
		Bee:    bee,
		Sector: source.Sector,
		Intent: source.Intent,
		Review: protocol.TaskReviewNone,
	}
	planEv, err := tasks.PlanEvent(traceID, agentID, spec)
	if err != nil {
		return "", err
	}
	if err := pub.PublishEvent(ctx, planEv); err != nil {
		return "", err
	}
	readyEv, err := tasks.ReadyEvent(traceID, agentID, taskledger.TaskSnapshot{
		TaskID: taskID,
		Title:  reworkTaskTitle,
		Body:   body,
		Bee:    bee,
		Sector: source.Sector,
		Intent: source.Intent,
	}, defaults)
	if err != nil {
		return "", err
	}
	if err := pub.PublishEvent(ctx, readyEv); err != nil {
		return "", err
	}
	return taskID, nil
}
