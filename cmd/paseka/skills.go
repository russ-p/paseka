package main

import (
	"fmt"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Inspect the colony skill store",
		Long: `Inspect the colony-owned Agent Skills store at .paseka/skills/.

Each immediate directory is one skill, named for that directory, holding a
SKILL.md with YAML frontmatter (name, description, optional metadata) and a
Markdown body. scripts/, references/, and assets/ are optional supporting
content.

The group is a namespace, not an action: run a verb.`,
	}
	cmd.AddCommand(newSkillsValidateCmd())
	return cmd
}

func newSkillsValidateCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "validate [reference|pattern|*]",
		Short: "Validate the skill store, or the skills a reference names",
		Long: `Validate skills against the one rule set the runtime uses.

With no argument the whole store is checked. With a reference only the skills it
names are checked, plus the store-wide budgets. A pattern that matches nothing is
itself a failure. The command exits non-zero when anything is invalid, so it can
guard a commit.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// An invalid store is a verdict, not a usage error: the command ran and
			// reported. Only the flag layer should print usage.
			cmd.SilenceUsage = true
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			report, err := skills.ScanColony(ctxColony.ColonyRoot)
			if err != nil {
				return err
			}

			var issues []skills.Issue
			if len(args) == 0 {
				issues = report.Issues()
			} else {
				ref, err := skills.ParseReference(args[0])
				if err != nil {
					return err
				}
				issues = report.Find(ref)
			}

			out := cmd.OutOrStdout()
			if len(issues) == 0 {
				fmt.Fprintf(out, "skills: ok (%d skill(s), %s of %s)\n",
					len(report.Skills), formatSkillBytes(report.Bytes), formatSkillBytes(skills.MaxStoreBytes))
				return nil
			}
			for _, issue := range issues {
				fmt.Fprintln(out, issue.Error())
			}
			return fmt.Errorf("skills: %d issue(s) found", len(issues))
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

// formatSkillBytes renders a store size for the operator, who reads a bound in
// KiB or MiB far more easily than in bytes.
func formatSkillBytes(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
