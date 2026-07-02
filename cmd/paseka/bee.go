package main

import (
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/runtime"
	"github.com/spf13/cobra"
)

func newBeeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bee",
		Short: "Run colony bees",
	}
	cmd.AddCommand(newBeeRunCmd())
	return cmd
}

func newBeeRunCmd() *cobra.Command {
	var (
		startDir     string
		task         string
		traceID      string
		inlinePrompt string
	)
	cmd := &cobra.Command{
		Use:   "run <role>",
		Short: "Dispatch one bee (one-shot agent run)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if task == "" && inlinePrompt == "" {
				return fmt.Errorf("provide --task or --prompt")
			}
			d := runtime.NewDispatcher()
			res, err := d.BeeRun(cmd.Context(), runtime.BeeRunRequest{
				StartDir:     startDir,
				Bee:          args[0],
				TraceID:      traceID,
				Task:         task,
				InlinePrompt: inlinePrompt,
			})
			if err != nil {
				return err
			}
			printBeeRunResult(res)
			if res.Result != nil && res.Result.Status == "failed" {
				if res.Result.Err != nil {
					return res.Result.Err
				}
				return fmt.Errorf("bee run failed (exit %d)", res.Result.ExitCode)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().StringVarP(&task, "task", "t", "", "task body passed to the prompt template")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (generated if omitted)")
	cmd.Flags().StringVar(&inlinePrompt, "prompt", "", "inline prompt override (skips template)")
	return cmd
}

func printBeeRunResult(res *runtime.BeeRunResult) {
	status := "unknown"
	if res.Result != nil {
		status = res.Result.Status
	}
	fmt.Printf("Bee run %s\n", status)
	fmt.Printf("  trace:     %s\n", res.TraceID)
	fmt.Printf("  agent:     %s\n", res.AgentID)
	fmt.Printf("  workspace: %s\n", res.Workspace)
	fmt.Printf("  run dir:   %s\n", res.RunDir)

	if res.Result == nil {
		return
	}
	if out := strings.TrimSpace(res.Result.Output); out != "" {
		fmt.Println("\n--- output ---")
		fmt.Println(out)
	}
	for _, a := range res.Result.Artifacts {
		if a.Kind != "diff" || strings.TrimSpace(a.Content) == "" {
			continue
		}
		lines := strings.Count(a.Content, "\n")
		if a.Content != "" && !strings.HasSuffix(a.Content, "\n") {
			lines++
		}
		fmt.Printf("\n--- diff (%d lines) ---\n", lines)
		fmt.Print(a.Content)
		if !strings.HasSuffix(a.Content, "\n") {
			fmt.Println()
		}
	}
}
