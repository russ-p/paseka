package export

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
)

// SkillExport is one procedure a run was offered.
type SkillExport struct {
	Name        string
	Description string
	// Path is the frozen SKILL.md's location inside the run's own configuration
	// snapshot (the snapshot tree's logical path), so the report stays readable
	// on another checkout while ConfigRef still identifies the exact bytes.
	Path string
}

// RunSkillsExport is one run's offered set, resolved from that run's pinned
// configuration snapshot rather than the trail's current one.
//
// None is true when the snapshot resolved successfully and declared no skills.
// Omitted is set instead when the offered set could not be determined; a reader
// must never see an unreadable snapshot as an empty set.
type RunSkillsExport struct {
	AgentID   string
	Bee       string
	ConfigRef string
	Offered   []SkillExport
	None      bool
	Omitted   string

	// Used is the union of transcript evidence and load records, deduplicated by
	// name. None is true only when both readings ran and neither named a
	// procedure; Omitted carries the capability's reason when the run's usage
	// cannot be determined. Note qualifies a partial measurement.
	Used        []SkillUseExport
	UsedNone    bool
	UsedOmitted string
	Note        string
}

// loadSkillsExport resolves every run's offered and used sets from the
// configuration snapshot the run itself pinned. It never reads the live store,
// so a later store edit or removal cannot rewrite what an old export says.
func loadSkillsExport(ctx colony.Context, traceID string, runsView []hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) []RunSkillsExport {
	out := make([]RunSkillsExport, 0, len(runsView))
	for _, run := range runsView {
		out = append(out, loadRunSkillsExport(ctx, traceID, run, lookup, timeout))
	}
	return out
}

func loadRunSkillsExport(ctx colony.Context, traceID string, run hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) RunSkillsExport {
	entry := RunSkillsExport{AgentID: run.AgentID, Bee: run.Bee}

	req, err := (runs.Dir{ColonyRoot: ctx.ColonyRoot, TraceID: traceID, AgentID: run.AgentID}).ReadRequest()
	if err != nil {
		reason := fmt.Sprintf("run request is unreadable: %v", err)
		entry.Omitted = reason
		applyRunUsage(&entry, resolveRunUsage(ctx.ColonyRoot, traceID, run, nil, reason, lookup, timeout))
		return entry
	}
	if entry.Bee == "" {
		entry.Bee = req.Bee
	}
	entry.ConfigRef = strings.TrimSpace(req.ConfigRef)
	if entry.ConfigRef == "" {
		reason := "the run records no configuration snapshot; its offered skills are unavailable"
		entry.Omitted = reason
		applyRunUsage(&entry, resolveRunUsage(ctx.ColonyRoot, traceID, run, nil, reason, lookup, timeout))
		return entry
	}

	bound, err := trailconfig.BindRef(colony.Context{ColonyRoot: ctx.ColonyRoot}, traceID, entry.ConfigRef, req.ConfigRevision)
	if err != nil {
		entry.Omitted = err.Error()
		applyRunUsage(&entry, resolveRunUsage(ctx.ColonyRoot, traceID, run, nil, entry.Omitted, lookup, timeout))
		return entry
	}

	resolved, err := skills.ResolveRun(skills.RunInput{
		ConfigRoot: bound.ConfigRoot,
		Frozen:     true,
		Role:       req.Bee,
		Profile:    bound.Profile,
	})
	if err != nil {
		entry.Omitted = err.Error()
		applyRunUsage(&entry, resolveRunUsage(ctx.ColonyRoot, traceID, run, nil, entry.Omitted, lookup, timeout))
		return entry
	}

	for _, skill := range resolved.Skills {
		path := skill.Path
		if rel, err := filepath.Rel(bound.ConfigRoot, skill.Path); err == nil {
			// Report the location inside the frozen tree, not a machine-local
			// absolute path: the ConfigRef above says which snapshot it belongs
			// to, and the report stays readable on another checkout.
			path = filepath.ToSlash(rel)
		}
		entry.Offered = append(entry.Offered, SkillExport{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        path,
		})
	}
	entry.None = len(entry.Offered) == 0
	applyRunUsage(&entry, resolveRunUsage(ctx.ColonyRoot, traceID, run, resolved.Skills, "", lookup, timeout))
	return entry
}

func applyRunUsage(entry *RunSkillsExport, usage runUsage) {
	entry.Used = usage.Used
	entry.UsedNone = usage.None
	entry.UsedOmitted = usage.Omitted
	entry.Note = usage.Note
}
