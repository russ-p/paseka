package main

import (
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/purge"
	"github.com/spf13/cobra"
)

func newPruneCmd() *cobra.Command {
	var (
		startDir      string
		pruneRuns     bool
		pruneWorktree bool
		pruneBus      bool
		pruneAll      bool
		olderThan     string
		yes           bool
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove worktrees and run data older than a retention period (default 14 days)",
		Long: `Remove stale colony artifacts by age instead of removing everything.

Prune scans .paseka/worktrees/ and .paseka/runs/ and removes trace directories
whose last activity is older than --older-than (default 14d). Without --yes a
summary is shown and confirmation is required before anything is deleted.

--runs and --worktrees select what to prune; when neither is given (and --bus is
not the only target), both are pruned. --bus additionally removes JetStream
task-ledger KV, stream events, and artifacts for pruned traces when their
activity can be correlated from the ledger. --bus is never implied by --all.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			retention, err := purge.ParseRetention(olderThan)
			if err != nil {
				return err
			}
			if pruneAll || (!pruneRuns && !pruneWorktree && !pruneBus) {
				pruneRuns = true
				pruneWorktree = true
			}
			target := purge.PruneTarget{
				Runs:      pruneRuns,
				Worktrees: pruneWorktree,
				Bus:       pruneBus,
				OlderThan: retention,
			}

			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}

			plan, err := purge.Prune(ctx, target)
			if err != nil {
				return err
			}
			if purge.PrunePlanEmpty(plan) {
				fmt.Println("Nothing to prune.")
				return nil
			}

			fmt.Printf("Colony: %s\n", ctx.ColonyRoot)
			fmt.Printf("Retention: %s (last used before %s)\n\n", olderThan, plan.Cutoff.UTC().Format(time.RFC3339))
			fmt.Println("Will remove:")
			fmt.Println(purge.FormatPrunePlan(plan))

			if !yes {
				if !confirmPurge() {
					fmt.Println("Aborted.")
					return nil
				}
			}

			res, err := purge.ExecutePrune(ctx, target, plan)
			if err != nil {
				return err
			}
			printPruneResult(res)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().BoolVar(&pruneRuns, "runs", false, "prune .paseka/runs/ trace directories by age")
	cmd.Flags().BoolVar(&pruneWorktree, "worktrees", false, "prune .paseka/worktrees/ and git worktrees by age")
	cmd.Flags().BoolVar(&pruneBus, "bus", false, "also remove JetStream state for correlatable pruned traces (requires NATS)")
	cmd.Flags().BoolVar(&pruneAll, "all", false, "prune runs and worktrees (does not include --bus)")
	cmd.Flags().StringVar(&olderThan, "older-than", "14d", "retention period: 14d, 2w, or any Go duration such as 336h")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
}

func printPruneResult(res purge.PruneResult) {
	if len(res.Removed) == 0 && len(res.Bus) == 0 {
		fmt.Println("Nothing removed.")
		return
	}
	if len(res.Removed) > 0 {
		fmt.Printf("\nRemoved %d item(s):\n", len(res.Removed))
		for _, p := range res.Removed {
			fmt.Printf("  - %s\n", p)
		}
	}
	if len(res.Bus) > 0 {
		fmt.Printf("\nBus prune (%d trace(s)):\n", len(res.Bus))
		for _, b := range res.Bus {
			fmt.Printf("  - %s: %d key(s), %d event(s), %d artifact(s)\n",
				b.TraceID, len(b.KeysRemoved), b.EventsRemoved, len(b.ObjectsRemoved))
		}
	}
}
