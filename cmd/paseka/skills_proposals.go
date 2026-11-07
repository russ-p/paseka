package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
	"github.com/spf13/cobra"
)

// proposalEntry is one row of `paseka skills proposals --json`. The field set is
// the contract: state and its reason are separate, bundle validity is its own
// fact, and decisions carries the newest line per destination.
type proposalEntry struct {
	Trail          string                  `json:"trail"`
	Name           string                  `json:"name"`
	Description    string                  `json:"description,omitempty"`
	ProposedAt     string                  `json:"proposedAt"`
	State          string                  `json:"state"`
	Reason         string                  `json:"reason,omitempty"`
	Destination    string                  `json:"destination,omitempty"`
	DecidedAt      string                  `json:"decidedAt,omitempty"`
	Bundle         string                  `json:"bundle"`
	BundleMissing  bool                    `json:"bundleMissing"`
	ProcedureHash  string                  `json:"procedureHash"`
	RecomputedHash string                  `json:"recomputedHash,omitempty"`
	Valid          bool                    `json:"valid"`
	Validation     []string                `json:"validation"`
	Decisions      []proposalDecisionEntry `json:"decisions"`
}

// proposalDecisionEntry is one destination's newest recorded decision.
type proposalDecisionEntry struct {
	Decision    string   `json:"decision"`
	Destination string   `json:"destination"`
	At          string   `json:"at"`
	Reason      string   `json:"reason,omitempty"`
	Roles       []string `json:"roles,omitempty"`
}

// proposalDocument is the whole machine contract of `proposals`. Both lists are
// always arrays so a consumer can range over them without a nil check.
type proposalDocument struct {
	Proposals []proposalEntry `json:"proposals"`
	Issues    []string        `json:"issues"`
}

func newSkillsProposalsCmd() *cobra.Command {
	var (
		startDir string
		jsonOut  bool
		trail    string
	)
	cmd := &cobra.Command{
		Use:   "proposals [reference...]",
		Short: "List the skill proposals the trails hold",
		Long: `List every skill proposal the trails hold, with the state of its decision.

A proposal is an INSIGHT/skill.proposed discovery the Archivist published, and
its bundle lives in that trail's artifact comb. The row carries the source
trail, the skill name, when it was proposed, and its state:

  pending    no decision yet; a valid bundle is marked valid
  installed  a Beekeeper approved it into the shared store
  refused    a Beekeeper turned it down; the reason is shown and the refusal is
             terminal for that exact bundle
  trialled   it was applied to a trail's candidate configuration; the
             destination trail is named
  missing    the discovery's bundle is gone

The decision file is the runtime-owned, machine-local
.paseka/runs/<traceId>/skill-proposals/decisions.ndjson. It is not shared through
git, so a colleague sees a proposal as pending until an install is committed,
and the decisions disappear with the trail.

Without arguments every proposal is listed. A reference is <skill> or
<trail>/<skill>; --trail supplies the trail for a bare name. A bare name that
matches more than one trail is refused and names the candidates. --json prints
the same rows as a machine contract.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// A refused reference or an unknown proposal is a verdict, not a
			// usage error: the command ran and reported.
			cmd.SilenceUsage = true

			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			inventory, err := skills.ScanProposals(ctxColony.ColonyRoot)
			if err != nil {
				return err
			}
			selected, err := selectProposals(inventory, args, trail)
			if err != nil {
				return err
			}

			doc := proposalDocument{
				Proposals: make([]proposalEntry, 0, len(selected)),
				Issues:    make([]string, 0, len(inventory.Issues)),
			}
			for _, proposal := range selected {
				doc.Proposals = append(doc.Proposals, proposalRow(proposal))
			}
			for _, issue := range inventory.Issues {
				doc.Issues = append(doc.Issues, issue.Error())
			}

			out := cmd.OutOrStdout()
			if jsonOut {
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(doc)
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "TRAIL\tSKILL\tPROPOSED\tSTATE\tBUNDLE")
			for _, entry := range doc.Proposals {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					entry.Trail, entry.Name, entry.ProposedAt,
					proposalStateColumn(entry), proposalBundleColumn(entry))
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, issue := range doc.Issues {
				if _, err := fmt.Fprintln(out, issue); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&trail, "trail", "", "trail id used for bare references and to filter the listing")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the proposals as JSON on stdout")
	return cmd
}

// selectProposals applies the command's arguments to the inventory. A named
// selection that matches nothing is an error: the operator asked for a fact and
// the answer is that there is none, which is different from an empty listing.
func selectProposals(inventory skills.ProposalInventory, args []string, defaultTrail string) ([]skills.Proposal, error) {
	if len(args) == 0 {
		if defaultTrail == "" {
			return inventory.Proposals, nil
		}
		selected := make([]skills.Proposal, 0, len(inventory.Proposals))
		for _, proposal := range inventory.Proposals {
			if proposal.Trail == defaultTrail {
				selected = append(selected, proposal)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("skills: no proposals in trail %s", defaultTrail)
		}
		return selected, nil
	}

	seen := map[string]bool{}
	selected := make([]skills.Proposal, 0, len(args))
	for _, raw := range args {
		ref, err := skills.ParseProposalReference(raw, defaultTrail)
		if err != nil {
			return nil, err
		}
		matched, err := inventory.Resolve(ref)
		if err != nil {
			return nil, err
		}
		for _, proposal := range matched {
			key := proposal.Trail + "/" + proposal.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			selected = append(selected, proposal)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Trail != selected[j].Trail {
			return selected[i].Trail < selected[j].Trail
		}
		return selected[i].Name < selected[j].Name
	})
	return selected, nil
}

// proposalRow maps the inventory's row onto the machine contract.
func proposalRow(proposal skills.Proposal) proposalEntry {
	entry := proposalEntry{
		Trail:          proposal.Trail,
		Name:           proposal.Name,
		Description:    proposal.Description,
		ProposedAt:     proposalTimeColumn(proposal.ProposedAt),
		State:          proposal.State,
		Reason:         proposal.Reason,
		Destination:    proposal.Destination,
		DecidedAt:      proposalTimeColumn(proposal.DecidedAt),
		Bundle:         proposal.Bundle,
		BundleMissing:  proposal.BundleMissing,
		ProcedureHash:  proposal.ProcedureHash,
		RecomputedHash: proposal.RecomputedHash,
		Valid:          proposal.Valid,
		Validation:     proposal.Validation,
		Decisions:      make([]proposalDecisionEntry, 0, len(proposal.Decisions)),
	}
	if entry.Validation == nil {
		entry.Validation = []string{}
	}
	if proposal.DecidedAt.IsZero() {
		entry.DecidedAt = ""
	}
	for _, decision := range proposal.Decisions {
		entry.Decisions = append(entry.Decisions, proposalDecisionEntry{
			Decision:    decision.Decision,
			Destination: decision.Destination,
			At:          proposalTimeColumn(decision.At),
			Reason:      decision.Reason,
			Roles:       decision.Roles,
		})
	}
	return entry
}

func proposalTimeColumn(at time.Time) string {
	if at.IsZero() {
		return "-"
	}
	return at.UTC().Format(time.RFC3339)
}

// proposalStateColumn folds the state and its one clarifying fact into the one
// column the table has.
func proposalStateColumn(entry proposalEntry) string {
	switch entry.State {
	case skills.ProposalRefused:
		if entry.Reason != "" {
			return "refused: " + entry.Reason
		}
		return "refused"
	case skills.ProposalTrialled:
		if entry.Destination != "" {
			return "trialled: " + entry.Destination
		}
		return "trialled"
	default:
		return entry.State
	}
}

// proposalBundleColumn is the bundle's verdict. A missing bundle is said plainly
// rather than shown as invalid: absence and corruption are different facts, and
// a decided proposal can still lose its bundle without changing the decision.
func proposalBundleColumn(entry proposalEntry) string {
	if entry.BundleMissing {
		return "missing"
	}
	if entry.Valid {
		return "valid"
	}
	if len(entry.Validation) == 0 {
		return "invalid"
	}
	return "invalid: " + strings.Join(entry.Validation, "; ")
}
