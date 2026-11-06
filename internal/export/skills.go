package export

import (
	"fmt"
	"path/filepath"
	"strings"

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
}

// loadSkillsExport resolves every run's offered set from the configuration
// snapshot the run itself pinned. It never reads the live store, so a later
// store edit or removal cannot rewrite what an old export says.
func loadSkillsExport(ctx colony.Context, traceID string, runsView []hiveview.RunView) []RunSkillsExport {
	out := make([]RunSkillsExport, 0, len(runsView))
	for _, run := range runsView {
		out = append(out, loadRunSkillsExport(ctx, traceID, run))
	}
	return out
}

func loadRunSkillsExport(ctx colony.Context, traceID string, run hiveview.RunView) RunSkillsExport {
	entry := RunSkillsExport{AgentID: run.AgentID, Bee: run.Bee}

	req, err := (runs.Dir{ColonyRoot: ctx.ColonyRoot, TraceID: traceID, AgentID: run.AgentID}).ReadRequest()
	if err != nil {
		entry.Omitted = fmt.Sprintf("run request is unreadable: %v", err)
		return entry
	}
	if entry.Bee == "" {
		entry.Bee = req.Bee
	}
	entry.ConfigRef = strings.TrimSpace(req.ConfigRef)
	if entry.ConfigRef == "" {
		entry.Omitted = "the run records no configuration snapshot; its offered skills are unavailable"
		return entry
	}

	bound, err := trailconfig.BindRef(colony.Context{ColonyRoot: ctx.ColonyRoot}, traceID, entry.ConfigRef, req.ConfigRevision)
	if err != nil {
		entry.Omitted = err.Error()
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
	return entry
}
