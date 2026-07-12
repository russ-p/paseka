package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/purge"
	"github.com/spf13/cobra"
)

func newPurgeCmd() *cobra.Command {
	var (
		startDir       string
		purgeRuns      bool
		purgeWorktrees bool
		purgeCache     bool
		purgeState     bool
		purgeBus       bool
		purgeAll       bool
		traceID        string
		yes            bool
	)
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Remove ephemeral colony artifacts (runs, worktrees, cache, state, bus)",
		Long: `Remove gitignored and machine-local artifacts created during bee runs.

Use flags to select what to remove. Without --yes, a summary is shown and
confirmation is required before anything is deleted.

--bus removes JetStream task-ledger KV, stream events, and artifacts for one
trace. It requires --trace and a configured NATS URL. --bus is not included in --all.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if purgeAll {
				purgeRuns = true
				purgeWorktrees = true
				purgeCache = true
				purgeState = true
			}
			if purgeBus && traceID == "" {
				return fmt.Errorf("--trace is required with --bus")
			}
			target := colony.PurgeTarget{
				Runs:      purgeRuns,
				Worktrees: purgeWorktrees,
				Cache:     purgeCache,
				State:     purgeState,
				Bus:       purgeBus,
				TraceID:   traceID,
			}
			if !target.Any() {
				return fmt.Errorf("specify at least one target: --runs, --worktrees, --cache, --state, --all, or --bus")
			}

			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}

			plan, err := purge.Plan(ctx, target)
			if err != nil {
				return err
			}
			if colony.PlanEmpty(plan) {
				fmt.Println("Nothing to purge.")
				return nil
			}

			fmt.Printf("Colony: %s\n\n", ctx.ColonyRoot)
			fmt.Println("Will remove:")
			fmt.Println(colony.FormatPlan(plan))

			if !yes {
				if !confirmPurge() {
					fmt.Println("Aborted.")
					return nil
				}
			}

			res, err := purge.Execute(ctx, target)
			if err != nil {
				return err
			}
			printPurgeResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().BoolVar(&purgeRuns, "runs", false, "remove .paseka/runs/ trace directories")
	cmd.Flags().BoolVar(&purgeWorktrees, "worktrees", false, "remove .paseka/worktrees/ and git worktrees")
	cmd.Flags().BoolVar(&purgeCache, "cache", false, "remove .paseka/cache/")
	cmd.Flags().BoolVar(&purgeState, "state", false, "reset ~/.config/paseka/<slug>/state.json worktree registry")
	cmd.Flags().BoolVar(&purgeBus, "bus", false, "remove JetStream KV, stream events, and artifacts for --trace (requires NATS)")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id (required with --bus)")
	cmd.Flags().BoolVar(&purgeAll, "all", false, "purge runs, worktrees, cache, and state")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func printPurgeResult(res colony.PurgeResult) {
	if len(res.Removed) == 0 && res.Bus == nil {
		fmt.Println("Nothing removed.")
		return
	}
	if len(res.Removed) > 0 {
		fmt.Printf("\nRemoved %d item(s):\n", len(res.Removed))
		for _, p := range res.Removed {
			fmt.Printf("  - %s\n", p)
		}
	}
	if res.Bus != nil {
		fmt.Println("\nBus purge:")
		for _, key := range res.Bus.KeysRemoved {
			fmt.Printf("  - task ledger key: %s\n", key)
		}
		if res.Bus.EventsRemoved > 0 {
			fmt.Printf("  - %d stream event(s)\n", res.Bus.EventsRemoved)
		}
		for _, name := range res.Bus.ObjectsRemoved {
			fmt.Printf("  - artifact: %s\n", name)
		}
		if len(res.Bus.KeysRemoved) == 0 && res.Bus.EventsRemoved == 0 && len(res.Bus.ObjectsRemoved) == 0 {
			fmt.Println("  (nothing removed)")
		}
	}
}

func confirmPurge() bool {
	fmt.Print("\nProceed? [y/N] ")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}
