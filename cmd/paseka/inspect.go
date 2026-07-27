package main

import (
	"fmt"
	"io"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Inspect flight trail and run projections",
	}
	cmd.AddCommand(newInspectUsageCmd())
	return cmd
}

func newInspectUsageCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
	)
	cmd := &cobra.Command{
		Use:   "usage",
		Short: "Show LLM token usage for a trace or run",
		RunE: func(cmd *cobra.Command, args []string) error {
			if traceID == "" {
				return fmt.Errorf("--trace is required")
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
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&agentID, "agent", "", "agent id (single run; default: trace aggregate)")
	_ = cmd.MarkFlagRequired("trace")
	return cmd
}

func printTraceUsage(w io.Writer, colonyRoot, traceID string) error {
	summary, err := runs.LoadTraceSummary(colonyRoot, traceID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Trace: %s\n", traceID)
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
