package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/spf13/cobra"
)

func newSkillsApproveCmd() *cobra.Command {
	var (
		startDir string
		trail    string
		force    bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "approve <reference>...",
		Short: "Install proposed skills into the repository store",
		Long: `Install one or more proposed skills into the repository store.

A reference is <skill> or <trail>/<skill>; --trail supplies the trail for a bare
name. A bare name that appears in more than one trail is refused and names every
candidate, because a proposal is a trail's output and choosing between two trails
is not this command's decision.

The batch is atomic for the failures an operator can cause: every bundle is
resolved, hash-verified against its discovery, and validated against the one
skill rule set before a single byte is written. One invalid, missing, refused,
or conflicting reference writes nothing.

An existing store entry is refused without --force. With --force the whole batch
still validates before any replacement, and an existing disable record is kept:
the replaced skill remains disabled and the command says so.

Installation writes provenance into the skill's own metadata: source=proposal,
the source trail, the proposing role, the approval time, and the procedure-content
hash. The hash covers the SKILL.md body and the supporting files in a stable
order and excludes the frontmatter, so writing provenance does not change it.
` + "`paseka doctor`" + ` reports a skill whose body or supporting file no longer
matches its recorded hash.

The command appends one installed decision to each source trail's
skill-proposals/decisions.ndjson, names every working-tree path it wrote, and says
that committing is still the Beekeeper's. It never writes a commit.

--dry-run prints the same plan and leaves the store, ledger, and decision files
byte-identical.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			approvals, err := skills.ApproveProposals(
				ctxColony.ColonyRoot, args, trail, force, time.Now(), dryRun,
			)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			for _, approval := range approvals {
				target := approval.Target
				if rel, relErr := filepath.Rel(ctxColony.ColonyRoot, target); relErr == nil {
					target = filepath.ToSlash(rel)
				}
				verb := "installed"
				if dryRun {
					verb = "would install"
				}
				if approval.Replaced {
					verb = "would replace"
					if !dryRun {
						verb = "replaced"
					}
				}
				if _, err := fmt.Fprintf(out, "%s %s/%s -> %s\n",
					verb, approval.Proposal.Trail, approval.Proposal.Name, target); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "  procedure: %s\n", approval.Decision.ProcedureHash); err != nil {
					return err
				}
				if _, err := fmt.Fprintf(out, "  provenance: %s\n", provenanceLine(approval.Provenance)); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(out, "  files:"); err != nil {
					return err
				}
				for _, path := range approval.Files {
					if _, err := fmt.Fprintf(out, "    %s\n", path); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintf(out, "  decision: %s\n",
					skills.ProposalDecisionsPath(ctxColony.ColonyRoot, approval.Proposal.Trail)); err != nil {
					return err
				}
				// The exact line the append writes, so a dry run previews the
				// machine-local record and the real run shows what it recorded.
				if _, err := fmt.Fprintf(out, "  line: %s\n", strings.TrimSpace(string(approval.Line))); err != nil {
					return err
				}
				if approval.Disabled {
					reason := approval.DisableReason
					if strings.TrimSpace(reason) == "" {
						reason = "no reason recorded"
					}
					if _, err := fmt.Fprintf(out,
						"  state: disabled (%s); the existing disable record is kept and the skill remains disabled\n",
						reason); err != nil {
						return err
					}
				}
			}

			paths := 0
			for _, approval := range approvals {
				paths += len(approval.Files)
			}
			if dryRun {
				_, err = fmt.Fprintf(out,
					"dry run: nothing was written; the store, ledger, and decision files are unchanged (the plan above would write %d path(s))\n",
					paths)
				return err
			}
			_, err = fmt.Fprintf(out,
				"paseka skills approve: %d skill(s) installed into the repository store; %d working-tree path(s) were written.\n",
				len(approvals), paths)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out,
				"The working tree now has uncommitted changes; a path that did not exist before is untracked. Committing is the Beekeeper's decision: this command never commits.\n")
			return err
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&trail, "trail", "", "trail id for a bare reference")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing store entry (a disable record is kept)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without touching the store or the decisions")
	return cmd
}

// provenanceLine prints the provenance metadata in a stable order, so an
// operator can read the same facts a later `skills show` will read from the
// installed file.
func provenanceLine(provenance map[string]string) string {
	order := []string{
		skills.MetadataSource,
		skills.MetadataTrail,
		skills.MetadataRole,
		skills.MetadataApprovedAt,
		skills.MetadataProcedureHash,
	}
	parts := make([]string, 0, len(order))
	for _, key := range order {
		if value := strings.TrimSpace(provenance[key]); value != "" {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, " ")
}
