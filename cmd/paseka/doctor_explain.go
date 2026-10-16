package main

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/launch"
	"github.com/russ-p/paseka/internal/runs"
)

// explainLaunchPlan resolves one bee's launch without running it and prints the
// plan with the layer that set each field, so an operator can debug precedence
// without reading Go. It is the CLI half of the attribution surface; the Console
// adapter panel renders the same layers, so the two cannot disagree.
func explainLaunchPlan(w io.Writer, ctx colony.Context, role string) error {
	bee, _, err := ctx.LoadBee(role)
	if err != nil {
		return err
	}
	adapterName, err := bee.ResolveAdapter()
	if err != nil {
		return err
	}
	aliases := ctx.ModelAliases
	if len(aliases) == 0 {
		if manifest, err := colony.LoadColony(ctx.ColonyRoot); err == nil {
			aliases = manifest.ModelAliases
		}
	}
	plan, err := launch.ResolveLaunch(launch.Request{
		Mode:      launch.AFK,
		Adapter:   adapterName,
		Bee:       bee,
		Extra:     colony.AdapterExtraLayer(ctx, adapterName),
		Aliases:   aliases,
		Workspace: ctx.ColonyRoot,
		RunDir: runs.Dir{
			ColonyRoot: ctx.ColonyRoot,
			TraceID:    "explain",
			AgentID:    "explain",
		},
	})
	if err != nil {
		return err
	}
	renderLaunchPlan(w, role, plan)
	return nil
}

// renderLaunchPlan writes the resolved plan with its per-field attribution. The
// value is always shown next to its layer so an operator sees the declaration
// and the precedence in one line; a credential is reported as set/unset, never
// printed.
func renderLaunchPlan(w io.Writer, role string, plan launch.LaunchPlan) {
	fmt.Fprintf(w, "Launch plan for bee %s\n", role)
	fmt.Fprintf(w, "  adapter: %s\n", plan.Adapter)
	fmt.Fprintf(w, "  mode:    %s\n", plan.Mode)
	fmt.Fprintf(w, "  form:    %s\n", plan.Form)
	fmt.Fprintln(w)

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, field := range plan.Attribution() {
		fmt.Fprintf(tw, "  %s:\t%s\t[%s]\n", field.Field, field.Value, field.Layer)
	}
	_ = tw.Flush()

	warnings := explainWarnings(plan)
	if len(warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warning := range warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
}

// explainWarnings merges the resolver's own notices with the ones only doctor
// reports, so an unmapped reasoning value is named instead of silently dropped.
func explainWarnings(plan launch.LaunchPlan) []string {
	warnings := append([]string(nil), plan.Warnings...)
	if notice := launch.UnmappedReasoning(plan.Adapter, plan.Params.Reasoning); notice != "" {
		warnings = append(warnings, notice)
	}
	return warnings
}
