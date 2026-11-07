package main

import (
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/spf13/cobra"
)

func newSkillsRejectCmd() *cobra.Command {
	var (
		startDir string
		trail    string
		reason   string
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "reject <reference>",
		Short: "Refuse one skill proposal and record the decision",
		Long: `Refuse exactly one skill proposal.

The reference is <skill> or <trail>/<skill>; --trail supplies the trail for a
bare name. It is resolved the same way ` + "`paseka skills proposals`" + ` resolves one,
so a pattern, an unknown name, and a bare name that appears in more than one
trail are refused, and an ambiguity names every candidate.

A refusal is a terminal decision for that exact bundle. It appends one
` + "`refused`" + ` line to the proposing trail's decision file with the trail, the
skill, the bundle's procedure hash, the time, and the optional --reason. The
same bundle cannot then be approved or trialled, and rejecting it again reports
the existing decision without adding a second line. A re-proposed bundle with a
different procedure hash is a new proposal and can still be decided.

The bundle must exist, pass the one skill rule set, and not already be in the
store. --dry-run prints the exact line that would be appended and changes
nothing: no store, ledger, or decision file is touched. The append is serialized
under the trail's proposal-decision lock, so a concurrent writer cannot
interleave a partial line. Nothing is committed: the decision is machine-local
trail data, not a repository change.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			result, err := skills.RejectProposal(
				ctxColony.ColonyRoot, args[0], trail, reason, time.Now(), dryRun,
			)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if result.AlreadyRefused {
				decided := result.Proposal.DecidedAt.UTC().Format(time.RFC3339)
				if result.Proposal.Reason != "" {
					_, err = fmt.Fprintf(out,
						"proposal %s/%s was already refused at %s (%s); no new decision recorded\n",
						result.Proposal.Trail, result.Proposal.Name, decided, result.Proposal.Reason)
				} else {
					_, err = fmt.Fprintf(out,
						"proposal %s/%s was already refused at %s; no new decision recorded\n",
						result.Proposal.Trail, result.Proposal.Name, decided)
				}
				return err
			}

			path := skills.ProposalDecisionsPath(ctxColony.ColonyRoot, result.Proposal.Trail)
			if dryRun {
				if _, err := fmt.Fprintf(out, "dry run: would append to %s\n", path); err != nil {
					return err
				}
				_, err := fmt.Fprint(out, string(result.Line))
				return err
			}

			if _, err := fmt.Fprintf(out, "refused %s/%s\n", result.Proposal.Trail, result.Proposal.Name); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "  procedure: %s\n", result.Decision.ProcedureHash); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(out, "  at: %s\n", result.Decision.At.UTC().Format(time.RFC3339)); err != nil {
				return err
			}
			if result.Decision.Reason != "" {
				if _, err := fmt.Fprintf(out, "  reason: %s\n", result.Decision.Reason); err != nil {
					return err
				}
			}
			_, err = fmt.Fprintf(out,
				"  decision: %s (machine-local; it disappears with the trail)\n", path)
			return err
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&trail, "trail", "", "trail id for a bare reference")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "why the proposal is refused (optional)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the decision line without touching anything")
	return cmd
}
