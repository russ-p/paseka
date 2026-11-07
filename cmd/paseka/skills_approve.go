package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/spf13/cobra"
)

func newSkillsApproveCmd() *cobra.Command {
	var (
		startDir string
		trail    string
		into     string
		roles    []string
		force    bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "approve <reference>...",
		Short: "Install proposed skills into the repository store or trial them on a trail",
		Long: `Install one or more proposed skills into the repository store, or apply them
to one trail's candidate configuration as a trial.

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

--into <trace> --role <role>... applies the same batch as a trial instead. It
builds one candidate from the trail's active snapshot, adds the bundles under that
candidate's store, and makes every named role resolve them in the same candidate:
an inherited default becomes the resolved default list plus the skill, an explicit
list gets the skill appended, and a wildcard or matching pattern is left unchanged
so its gate is not narrowed. The whole candidate is validated with the shared
skill rules and the configuration procedure validation, one revision is applied,
and a trialled decision is appended to each proposing trail. The store and the
working tree are untouched. A trial refuses without --role, while another
configuration change is pending, and while an execution blocker holds activation.

--dry-run prints the same plan and writes nothing.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			into = strings.TrimSpace(into)
			if into != "" {
				if len(roles) == 0 {
					return fmt.Errorf("--role is required with --into: a trial that adds a skill no role carries changes nothing")
				}
				if force {
					return fmt.Errorf("--force applies only to the store destination")
				}
				return runSkillsTrial(cmd, ctxColony, args, trail, into, roles, dryRun)
			}
			if len(roles) > 0 {
				return fmt.Errorf("--role requires --into <trace>: a role selection is part of a trail trial")
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
	cmd.Flags().StringVar(&into, "into", "", "trial destination trail instead of the store")
	cmd.Flags().StringArrayVar(&roles, "role", nil, "role that must resolve the trialled skills (repeatable; requires --into)")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing store entry (a disable record is kept)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without touching the store, decisions, or configuration")
	return cmd
}

// runSkillsTrial applies one batch to a trail's candidate configuration. It
// builds and validates the entire candidate before anything is recorded, so a
// failure leaves the trail's history and the source trails' decisions exactly
// as they were.
func runSkillsTrial(
	cmd *cobra.Command,
	ctxColony colony.Context,
	args []string,
	defaultTrail, into string,
	roles []string,
	dryRun bool,
) error {
	at := time.Now().UTC()
	bundles, err := skills.PrepareTrialBundles(ctxColony.ColonyRoot, args, defaultTrail, into, roles, at)
	if err != nil {
		return err
	}

	binding, err := trailconfig.Read(ctxColony.ColonyRoot, into)
	if err != nil {
		return err
	}
	expected := binding.ConfigRevision
	trialBundles := make([]trailconfig.SkillTrialBundle, 0, len(bundles))
	names := make([]string, 0, len(bundles))
	for _, bundle := range bundles {
		files := make([]trailconfig.SkillTrialFile, 0, len(bundle.Files))
		for _, file := range bundle.Files {
			files = append(files, trailconfig.SkillTrialFile{Rel: file.Rel, Data: file.Data})
		}
		trialBundles = append(trialBundles, trailconfig.SkillTrialBundle{
			Name:          bundle.Name(),
			Files:         files,
			ProcedureHash: bundle.Decision.ProcedureHash,
		})
		names = append(names, bundle.Name())
	}

	result, err := trailconfig.Apply(trailconfig.ApplyOptions{
		Context:          ctxColony,
		TraceID:          into,
		Source:           trailconfig.SourceSkillTrial,
		SkillTrial:       &trailconfig.SkillTrial{Bundles: trialBundles, Roles: roles},
		Actor:            "cli",
		Reason:           fmt.Sprintf("trial %s for role(s) %s", strings.Join(names, ", "), strings.Join(roles, ", ")),
		ExpectedRevision: &expected,
		RequireImmediate: true,
		DryRun:           dryRun,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	revision := result.Manifest.ConfigRevision
	switch {
	case result.Change.Outcome == trailconfig.OutcomeApplied && dryRun:
		revision++
	case !dryRun && result.Change.Revision > 0:
		revision = result.Change.Revision
	}
	if err := printTrialResult(out, result, bundles, into, revision, dryRun); err != nil {
		return err
	}

	if dryRun || result.Change.Outcome != trailconfig.OutcomeApplied {
		return nil
	}
	if err := skills.AppendTrialDecisions(ctxColony.ColonyRoot, bundles); err != nil {
		return fmt.Errorf("skills: trial applied as revision %d, but recording its decisions failed: %w", revision, err)
	}
	for _, bundle := range bundles {
		if _, err := fmt.Fprintf(out, "  recorded: %s\n",
			skills.ProposalDecisionsPath(ctxColony.ColonyRoot, bundle.Proposal.Trail)); err != nil {
			return err
		}
	}
	return nil
}

func printTrialResult(
	out io.Writer,
	result trailconfig.ApplyResult,
	bundles []skills.TrialBundle,
	into string,
	revision int,
	dryRun bool,
) error {
	verb := "applied"
	if dryRun {
		verb = "would apply"
	}
	if _, err := fmt.Fprintf(out, "trial %s %s: revision %d\n", verb, into, revision); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  outcome: %s\n", result.Change.Outcome); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  oldConfigRef: %s\n", result.Change.OldConfigRef); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  newConfigRef: %s\n", result.NewRef); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "  roles: %s\n", strings.Join(bundles[0].Decision.Roles, ", ")); err != nil {
		return err
	}
	if len(result.Diff) > 0 {
		if _, err := fmt.Fprintln(out, "  diff:"); err != nil {
			return err
		}
		for _, line := range result.Diff {
			if _, err := fmt.Fprintf(out, "    %s\n", line); err != nil {
				return err
			}
		}
	}
	if len(result.EffectiveDiff) > 0 {
		if _, err := fmt.Fprintln(out, "  effective diff:"); err != nil {
			return err
		}
		for _, line := range result.EffectiveDiff {
			if _, err := fmt.Fprintf(out, "    %s\n", line); err != nil {
				return err
			}
		}
	}
	for _, warning := range result.SkillWarnings {
		if _, err := fmt.Fprintf(out, "  warning: %s\n", warning); err != nil {
			return err
		}
	}
	if result.Change.Outcome == trailconfig.OutcomeApplied {
		for _, bundle := range bundles {
			verb := "decision"
			if dryRun {
				verb = "would record"
			}
			if _, err := fmt.Fprintf(out, "  %s: %s\n", verb, strings.TrimSpace(string(bundle.Line))); err != nil {
				return err
			}
		}
	}
	if result.Change.Outcome == trailconfig.OutcomeUnchanged {
		_, err := fmt.Fprintln(out,
			"the candidate is already active: no revision was written and no trial decision was recorded")
		return err
	}
	if dryRun {
		_, err := fmt.Fprintln(out,
			"dry run: nothing was written; the store, the working tree, the trail's history, and the decision files are unchanged")
		return err
	}
	_, err := fmt.Fprintln(out,
		"the store and the working tree are unchanged; the trial is one configuration revision on the trail")
	return err
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
