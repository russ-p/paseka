package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailview"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect flight trail and run projections",
	}
	cmd.AddCommand(newInspectUsageCmd())
	cmd.AddCommand(newInspectMetricsCmd())
	return cmd
}

func newInspectUsageCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
		since    string
		until    string
		timezone string
		adapters []string
		models   []string
		bees     []string
		purposes []string
		groupBy  string
		jsonOut  bool
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show LLM token usage for a trail, a run, or the colony",
		// A refused range is a one-line reason, not a reason followed by the flag
		// help: the operator asked for numbers, not usage.
		SilenceUsage: true,
		Long: "Show recorded LLM token usage.\n\n" +
			"With --trace the command prints one trail (or one run with --agent).\n" +
			"Without --trace it prints the colony report for a date range, built by the\n" +
			"same reader the Console Usage page uses.\n\n" +
			"The range defaults to the last 30 days ending now and the zone defaults to\n" +
			"the host's. Since is inclusive and until is exclusive; both are local\n" +
			"midnights in the chosen zone, and a record lands on the day of its start\n" +
			"time.\n\n" +
			"--group-by names the axis whose period ranking is printed: adapter (the\n" +
			"default), model, bee, or purpose. The JSON report always carries all four\n" +
			"axes, exactly as GET /api/usage returns them.\n\n" +
			"--adapter, --model, --bee, and --purpose restrict the report before\n" +
			"aggregation and may each be repeated; values on one axis are alternatives\n" +
			"and the axes combine. A filter never moves the range or the day boundaries.\n" +
			"The echoed filter set is printed with the report and carried by --json.",
		RunE: func(cmd *cobra.Command, args []string) error {
			rangeFlags := []string{"since", "until", "timezone", "adapter", "model", "bee", "purpose", "group-by", "json"}
			if traceID == "" {
				if agentID != "" {
					return fmt.Errorf("--agent requires --trace")
				}
				ctx, err := colony.ResolveContext(startDir)
				if err != nil {
					return err
				}
				if !runs.IsUsageAxis(groupBy) {
					return fmt.Errorf("--group-by %q is not one of adapter, model, bee, purpose", groupBy)
				}
				return printUsageRange(cmd.OutOrStdout(), ctx.ColonyRoot, runs.UsageQuery{
					Since:    since,
					Until:    until,
					Timezone: timezone,
					Adapters: adapters,
					Models:   models,
					Bees:     bees,
					Purposes: purposes,
				}, groupBy, jsonOut)
			}
			for _, name := range rangeFlags {
				if flagChanged(cmd, name) {
					return fmt.Errorf("--%s applies only without --trace", name)
				}
			}
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			if agentID != "" {
				return printRunUsage(cmd.OutOrStdout(), ctx.ColonyRoot, traceID, agentID)
			}
			return printTraceUsage(cmd.OutOrStdout(), ctx.ColonyRoot, traceID)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (omit for the colony range report)")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent id (single run; requires --trace)")
	cmd.Flags().StringVar(&since, "since", "", "first day of the range, YYYY-MM-DD (default: 30 days before today)")
	cmd.Flags().StringVar(&until, "until", "", "day after the last day of the range, YYYY-MM-DD (default: now)")
	cmd.Flags().StringVar(&timezone, "timezone", "", "IANA zone for the day boundaries (default: the host's)")
	cmd.Flags().StringArrayVar(&adapters, "adapter", nil, "restrict to an adapter name; repeat for several")
	cmd.Flags().StringArrayVar(&models, "model", nil, "restrict to a resolved model id; repeat for several")
	cmd.Flags().StringArrayVar(&bees, "bee", nil, "restrict to a Bee role; repeat for several")
	cmd.Flags().StringArrayVar(&purposes, "purpose", nil, "restrict to an execution purpose (work, closing, analysis); repeat for several")
	cmd.Flags().StringVar(&groupBy, "group-by", runs.UsageAxisAdapter, "axis ranked in the period breakdown: adapter, model, bee, or purpose")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print the report exactly as GET /api/usage returns it")
	return cmd
}

// printUsageRange builds and prints the colony report for one query. It writes
// nothing until the reader has accepted the range, so a typo cannot leave an
// empty report behind on stdout.
func printUsageRange(w io.Writer, colonyRoot string, query runs.UsageQuery, groupBy string, jsonOut bool) error {
	report, err := runs.ReadUsage(colonyRoot, query)
	if err != nil {
		var usageErr *runs.UsageError
		if errors.As(err, &usageErr) {
			return usageErr
		}
		return err
	}
	if jsonOut {
		// Same encoder settings as the console's writeJSON so a script can
		// compare this document with the endpoint's byte for byte.
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}
	return printUsageReport(w, report, groupBy)
}

func printUsageReport(w io.Writer, report runs.UsageReport, groupBy string) error {
	fmt.Fprintf(w, "Usage range: %s .. %s\n", report.Since, report.Until)
	fmt.Fprintf(w, "Time zone:   %s\n", report.Timezone)
	if filterLine := usageFilterLine(report.Filters); filterLine != "" {
		// The echo is what the report actually applied, not what the flags said:
		// duplicates and blanks are already gone.
		fmt.Fprintf(w, "Filters:     %s\n", filterLine)
	}
	fmt.Fprintf(w, "Observed at: %s\n", report.ObservedAt.Format(time.RFC3339))
	if report.EarliestRetainedAt != nil {
		fmt.Fprintf(w, "Earliest retained: %s\n", report.EarliestRetainedAt.Format(time.RFC3339))
	}
	trailWord := usageWord(report.Period.DistinctTrails, "trail", "trails")
	fmt.Fprintf(w, "Coverage: %d executions (%d reported, %d unmeasured) in %d %s\n",
		report.Period.Executions, report.Period.ReportedExecutions,
		report.Period.Executions-report.Period.ReportedExecutions, report.Period.DistinctTrails, trailWord)
	fmt.Fprintln(w, "Period tokens:")
	fmt.Fprintf(w, "  input:       %d\n", report.Period.Tokens.InputTokens)
	fmt.Fprintf(w, "  output:      %d\n", report.Period.Tokens.OutputTokens)
	fmt.Fprintf(w, "  reasoning:   %d\n", report.Period.Tokens.ReasoningTokens)
	fmt.Fprintf(w, "  cache read:  %d\n", report.Period.Tokens.CacheReadTokens)
	fmt.Fprintf(w, "  cache write: %d\n", report.Period.Tokens.CacheWriteTokens)
	fmt.Fprintf(w, "  total:       %d\n", report.Period.Tokens.TotalTokens)
	if report.Period.CostReportedExecutions > 0 {
		fmt.Fprintf(w, "Period recorded cost: %.2f USD across %d %s\n",
			report.Period.RecordedCost, report.Period.CostReportedExecutions,
			usageWord(report.Period.CostReportedExecutions, "execution", "executions"))
	}

	fmt.Fprintf(w, "\nDays (%d):\n", len(report.Days))
	fmt.Fprintf(w, "  %-10s  %6s  %8s  %6s  %12s  %12s  %11s  %12s  %12s  %14s  %s\n",
		"date", "exec", "reported", "trails", "input", "output", "reasoning",
		"cacheRead", "cacheWrite", "tokens", "state")
	for _, day := range report.Days {
		fmt.Fprintf(w, "  %-10s  %6d  %8d  %6d  %12d  %12d  %11d  %12d  %12d  %14d  %s\n",
			day.Date, day.Executions, day.ReportedExecutions, day.DistinctTrails,
			day.Tokens.InputTokens, day.Tokens.OutputTokens, day.Tokens.ReasoningTokens,
			day.Tokens.CacheReadTokens, day.Tokens.CacheWriteTokens, day.Tokens.TotalTokens,
			usageDayState(day))
	}

	if err := printUsageRanking(w, report, groupBy); err != nil {
		return err
	}

	if report.ExcludedUndated > 0 {
		verb := "have"
		if report.ExcludedUndated == 1 {
			verb = "has"
		}
		fmt.Fprintf(w, "\nExcluded undated: %d %s %s no usable start time\n",
			report.ExcludedUndated,
			usageWord(report.ExcludedUndated, "execution", "executions"), verb)
	}
	if len(report.Warnings) > 0 {
		fmt.Fprintln(w, "\nWarnings:")
		for _, warning := range report.Warnings {
			fmt.Fprintf(w, "  %s/%s: %s\n", warning.Group, warning.Kind, warning.Detail)
		}
	}
	return nil
}

// usageFilterLine renders the echoed filter set as `axis=value,value` clauses,
// or the empty string when the report was built over every retained record.
func usageFilterLine(filters runs.UsageFilters) string {
	clauses := make([]string, 0, 4)
	for _, axis := range []struct {
		name   string
		values []string
	}{
		{runs.UsageAxisAdapter, filters.Adapter},
		{runs.UsageAxisModel, filters.Model},
		{runs.UsageAxisBee, filters.Bee},
		{runs.UsageAxisPurpose, filters.Purpose},
	} {
		if len(axis.values) > 0 {
			clauses = append(clauses, axis.name+"="+strings.Join(axis.values, ","))
		}
	}
	return strings.Join(clauses, "; ")
}

// printUsageRanking prints the period breakdown of one axis, ranked by token
// volume with each key's share of the period. The reader has already sorted it.
func printUsageRanking(w io.Writer, report runs.UsageReport, axis string) error {
	entries := usageAxisEntries(report, axis)
	if !runs.IsUsageAxis(axis) {
		return fmt.Errorf("unknown axis %q", axis)
	}
	fmt.Fprintf(w, "\nPeriod breakdown by %s (%d %s):\n", axis, len(entries), usageWord(len(entries), "key", "keys"))
	if len(entries) == 0 {
		fmt.Fprintln(w, "  (no executions reported usage in this range)")
		return nil
	}
	fmt.Fprintf(w, "  %-34s  %10s  %14s  %7s  %18s\n", "key", "executions", "tokens", "share", "recorded cost")
	for _, entry := range entries {
		fmt.Fprintf(w, "  %-34s  %10d  %14d  %6.1f%%  %18s\n",
			usageAxisLabel(entry), entry.Executions, entry.Tokens.TotalTokens,
			usageShare(entry.Tokens.TotalTokens, report.Period.Tokens.TotalTokens),
			usageCostCell(entry))
	}
	return nil
}

func usageAxisEntries(report runs.UsageReport, axis string) []runs.UsageAxisEntry {
	switch axis {
	case runs.UsageAxisAdapter:
		return report.Breakdown.Adapter
	case runs.UsageAxisModel:
		return report.Breakdown.Model
	case runs.UsageAxisBee:
		return report.Breakdown.Bee
	case runs.UsageAxisPurpose:
		return report.Breakdown.Purpose
	}
	return nil
}

// usageAxisLabel is the key a reader sees: a name with its secondary label
// beside it, never the label alone.
func usageAxisLabel(entry runs.UsageAxisEntry) string {
	if entry.Label == "" {
		return entry.Key
	}
	return entry.Key + " (" + entry.Label + ")"
}

// usageShare is the key's percentage of the period's reported tokens.
func usageShare(tokens, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return float64(tokens) / float64(total) * 100
}

// usageCostCell is the recorded cost and the number of executions that reported
// it. A key whose executions recorded no cost says so rather than showing zero.
func usageCostCell(entry runs.UsageAxisEntry) string {
	if entry.CostReportedExecutions == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.2f (%d)", entry.RecordedCost, entry.CostReportedExecutions)
}

// usageWord pluralizes a count for a line an operator reads.
func usageWord(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// usageDayState names what a row cannot say in numbers: an empty day is a
// genuine zero, while an unmeasured day has executions that reported nothing
// and an incomplete day still has an execution in flight.
func usageDayState(day runs.UsageDay) string {
	if day.Executions == 0 {
		return "no executions"
	}
	var states []string
	if day.Unmeasured {
		states = append(states, "unmeasured")
	}
	if day.Incomplete {
		states = append(states, "incomplete")
	}
	if len(states) == 0 {
		return "ok"
	}
	return strings.Join(states, ",")
}

func printTraceUsage(w io.Writer, colonyRoot, traceID string) error {
	summary, err := runs.LoadTraceSummary(colonyRoot, traceID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Trace: %s\n", traceID)
	if scope, err := trailview.ResolveScope(colony.Context{ColonyRoot: colonyRoot}, traceID); err == nil && scope.Experimental {
		source := scope.ForkOf
		if source == "" {
			source = "(source unavailable)"
		}
		fmt.Fprintln(w, "  experimental: true")
		fmt.Fprintf(w, "  forkOf: %s\n", source)
	}
	if summary.Usage != nil {
		fmt.Fprintf(w, "  runs: %d (%d with usage)\n", summary.RunCount, summary.Usage.RunCountWithUsage)
		fmt.Fprintf(w, "  input:       %d\n", summary.Usage.InputTokens)
		fmt.Fprintf(w, "  output:      %d\n", summary.Usage.OutputTokens)
		fmt.Fprintf(w, "  cache read:  %d\n", summary.Usage.CacheReadTokens)
		fmt.Fprintf(w, "  cache write: %d\n", summary.Usage.CacheWriteTokens)
	} else {
		fmt.Fprintf(w, "  runs: %d\n", summary.RunCount)
		fmt.Fprintln(w, "  usage: (none)")
	}
	return nil
}

func printRunUsage(w io.Writer, colonyRoot, traceID, agentID string) error {
	meta, ok, err := runs.FindRun(colonyRoot, traceID, agentID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("run not found: %s/%s", traceID, agentID)
	}
	fmt.Fprintf(w, "Run: %s/%s\n", traceID, agentID)
	if meta.Bee != "" {
		fmt.Fprintf(w, "  bee:     %s\n", meta.Bee)
	}
	if meta.ProviderSessionID != "" {
		fmt.Fprintf(w, "  providerSessionId: %s\n", meta.ProviderSessionID)
	}
	if meta.Usage == nil {
		fmt.Fprintln(w, "  usage: (none)")
		return nil
	}
	u := meta.Usage
	fmt.Fprintf(w, "  input:   %d\n", u.InputTokens)
	fmt.Fprintf(w, "  output:  %d\n", u.OutputTokens)
	fmt.Fprintf(w, "  cache read:  %d\n", u.CacheReadTokens)
	fmt.Fprintf(w, "  cache write: %d\n", u.CacheWriteTokens)
	if u.Source != "" {
		fmt.Fprintf(w, "  source:  %s\n", u.Source)
	}
	return nil
}
