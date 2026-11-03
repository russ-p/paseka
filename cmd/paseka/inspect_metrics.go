package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/spf13/cobra"
)

func newInspectMetricsCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		asJSON   bool
		write    bool
	)
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Show a trail's cost, run counts, and work stage",
		Long: "Project one flight trail from its retained disk evidence: cue provenance,\n" +
			"entry task, work stage, honey, tokens, run counts, and durations. The read\n" +
			"works without NATS. --write persists the same projection to\n" +
			".paseka/runs/<traceId>/metrics.json without declaring work completed.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			// One projection, one observation time: the printed and persisted
			// forms are the same measurement, not two reads that may race.
			metrics, err := runs.BuildTrailMetrics(ctx.ColonyRoot, traceID, time.Now().UTC())
			if err != nil {
				return err
			}
			if write {
				if err := runs.WriteTrailMetrics(ctx.ColonyRoot, traceID, metrics); err != nil {
					return err
				}
			}
			out := cmd.OutOrStdout()
			if asJSON {
				data, err := json.MarshalIndent(metrics, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(out, string(data))
				return err
			}
			return printTrailMetrics(out, metrics)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the projection as JSON")
	cmd.Flags().BoolVar(&write, "write", false, "persist the projection to .paseka/runs/<traceId>/metrics.json")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}

func printTrailMetrics(w io.Writer, m runs.TrailMetrics) error {
	fmt.Fprintf(w, "Trace: %s\n", m.TraceID)
	fmt.Fprintf(w, "  observedAt: %s\n", m.ObservedAt.UTC().Format(time.RFC3339))

	if m.Class == nil {
		fmt.Fprintln(w, "  class: (unknown)")
	} else {
		descriptor := m.Class.TaskID
		var details []string
		if m.Class.Bee != "" {
			details = append(details, m.Class.Bee)
		}
		if m.Class.Intent != "" {
			details = append(details, m.Class.Intent)
		}
		if len(details) > 0 {
			descriptor += " (" + strings.Join(details, ", ") + ")"
		}
		fmt.Fprintf(w, "  class: %s, %d task(s)\n", descriptor, m.Class.TaskCount)
	}

	fmt.Fprintf(w, "  cue: %s (origin coverage: %s)\n", cueLabel(m), m.Coverage.Origin)
	if len(m.CueIngresses) > 0 {
		origin, addition, unknown := 0, 0, 0
		for _, ingress := range m.CueIngresses {
			switch ingress.Attribution {
			case runs.CueAttributionOrigin:
				origin++
			case runs.CueAttributionAddition:
				addition++
			default:
				unknown++
			}
		}
		fmt.Fprintf(w, "  ingresses: %d (origin %d, addition %d, unknown %d)\n",
			len(m.CueIngresses), origin, addition, unknown)
	}

	state := "unknown"
	if m.Work.State != nil {
		state = *m.Work.State
	}
	fmt.Fprintf(w, "  work: %s (stage %s)\n", state, m.Work.Stage)
	if m.Work.PendingCount > 0 || m.Work.Pause != nil {
		fmt.Fprintf(w, "  pending dispatches: %d\n", m.Work.PendingCount)
		for _, reason := range m.Work.PendingReasons {
			fmt.Fprintf(w, "    %s: %d\n", reason.Reason, reason.Count)
		}
		if m.Work.Pause != nil {
			reason := m.Work.Pause.Reason
			if reason == "" {
				reason = "(no reason recorded)"
			}
			fmt.Fprintf(w, "  paused: %s (%s, %d pending)\n",
				m.Work.Pause.EpisodeID, reason, m.Work.Pause.PendingCount)
		}
	}

	budget := "unknown"
	if m.Honey.Budget != nil {
		budget = strconv.Itoa(*m.Honey.Budget)
	}
	spent := "unknown"
	if m.Honey.Spent != nil {
		spent = strconv.Itoa(*m.Honey.Spent)
	}
	fmt.Fprintf(w, "  honey: budget %s, spent %s\n", budget, spent)
	reasons := make([]string, 0, len(m.Honey.ByReason))
	for reason := range m.Honey.ByReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		fmt.Fprintf(w, "    %s: %d\n", reason, m.Honey.ByReason[reason])
	}

	if m.Tokens == nil {
		fmt.Fprintln(w, "  tokens: (unavailable)")
	} else {
		partial := ""
		if m.Tokens.Partial {
			partial = ", partial"
		}
		fmt.Fprintf(w, "  tokens: input %d, output %d, cache read %d, cache write %d (%d/%d runs with usage%s)\n",
			m.Tokens.InputTokens, m.Tokens.OutputTokens, m.Tokens.CacheReadTokens, m.Tokens.CacheWriteTokens,
			m.Tokens.RunsWithUsage, m.Tokens.Runs, partial)
	}

	fmt.Fprintf(w, "  runs: %d", m.Runs.Retained)
	if len(m.Runs.ByBee) > 0 {
		bees := make([]string, 0, len(m.Runs.ByBee))
		for bee := range m.Runs.ByBee {
			bees = append(bees, bee)
		}
		sort.Strings(bees)
		var parts []string
		for _, bee := range bees {
			parts = append(parts, fmt.Sprintf("%s %d", bee, m.Runs.ByBee[bee]))
		}
		fmt.Fprintf(w, " (%s)", strings.Join(parts, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "    failed %d, cancelled %d, contract failed %d, contract unavailable %d, timeout %d, checks failed %d, checks passed %d\n",
		m.Runs.Failed, m.Runs.Cancelled, m.Runs.ContractFailed, m.Runs.ContractUnavailable,
		m.Runs.Timeout, m.Runs.ChecksFailed, m.Runs.ChecksPassed)

	fmt.Fprintf(w, "  active: %s\n", secondsLabel(m.ActiveTimeSec))
	fmt.Fprintf(w, "  wall: %s\n", secondsLabel(m.WallTimeSec))

	fmt.Fprintf(w, "  coverage: events %s, runs %s, tasks %s, origin %s, honey %s, tokens %s, durations %s\n",
		m.Coverage.Events, m.Coverage.Runs, m.Coverage.Tasks, m.Coverage.Origin,
		m.Coverage.Honey, m.Coverage.Tokens, m.Coverage.Durations)

	if len(m.Warnings) == 0 {
		fmt.Fprintln(w, "  warnings: (none)")
		return nil
	}
	fmt.Fprintln(w, "  warnings:")
	for _, warning := range m.Warnings {
		fmt.Fprintf(w, "    - [%s/%s] %s\n", warning.Group, warning.Kind, warning.Detail)
	}
	return nil
}

func cueLabel(m runs.TrailMetrics) string {
	if m.CueID != "" {
		return m.CueID
	}
	if len(m.CueIngresses) > 0 {
		return "(unknown origin)"
	}
	return "(none)"
}

func secondsLabel(value *float64) string {
	if value == nil {
		return "unknown"
	}
	return strconv.FormatFloat(*value, 'f', -1, 64) + "s"
}
