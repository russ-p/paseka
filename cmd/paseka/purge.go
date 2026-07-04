package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/paseka/paseka/internal/colony"
	"github.com/spf13/cobra"
)

func newPurgeCmd() *cobra.Command {
	var (
		startDir       string
		purgeRuns      bool
		purgeWorktrees bool
		purgeCache     bool
		purgeState     bool
		purgeAll       bool
		yes            bool
	)
	cmd := &cobra.Command{
		Use:   "purge",
		Short: "Remove ephemeral colony artifacts (runs, worktrees, cache, state)",
		Long: `Remove gitignored and machine-local artifacts created during bee runs.

Use flags to select what to remove. Without --yes, a summary is shown and
confirmation is required before anything is deleted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if purgeAll {
				purgeRuns = true
				purgeWorktrees = true
				purgeCache = true
				purgeState = true
			}
			target := colony.PurgeTarget{
				Runs:      purgeRuns,
				Worktrees: purgeWorktrees,
				Cache:     purgeCache,
				State:     purgeState,
			}
			if !target.Any() {
				return fmt.Errorf("specify at least one target: --runs, --worktrees, --cache, --state, or --all")
			}

			ctx, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}

			plan, err := colony.PlanPurge(ctx, target)
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

			res, err := colony.Purge(ctx, target)
			if err != nil {
				return err
			}
			if len(res.Removed) == 0 {
				fmt.Println("Nothing removed.")
				return nil
			}
			fmt.Printf("\nRemoved %d item(s):\n", len(res.Removed))
			for _, p := range res.Removed {
				fmt.Printf("  - %s\n", p)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository (default: current directory)")
	cmd.Flags().BoolVar(&purgeRuns, "runs", false, "remove .paseka/runs/ trace directories")
	cmd.Flags().BoolVar(&purgeWorktrees, "worktrees", false, "remove .paseka/worktrees/ and git worktrees")
	cmd.Flags().BoolVar(&purgeCache, "cache", false, "remove .paseka/cache/")
	cmd.Flags().BoolVar(&purgeState, "state", false, "reset ~/.config/paseka/<slug>/state.json worktree registry")
	cmd.Flags().BoolVar(&purgeAll, "all", false, "purge runs, worktrees, cache, and state")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	return cmd
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
