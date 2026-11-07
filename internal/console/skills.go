package console

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/skills"
)

// SkillRowView is one line of the read-only skills inventory. State and reason
// are separate here, while the page folds them into one cell, so the client
// never has to parse a state word out of prose.
type SkillRowView struct {
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Reason      string   `json:"reason,omitempty"`
	Roles       []string `json:"roles"`
	Description string   `json:"description"`
	Provenance  string   `json:"provenance"`
}

// SkillProposalView is one row of the proposals pane. It mirrors the shell's
// `paseka skills proposals --json` contract: state and its reason are separate,
// the destination names the trail a trial went to, and bundle validity is its
// own fact.
type SkillProposalView struct {
	Trail          string   `json:"trail"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	ProposedReason string   `json:"proposedReason,omitempty"`
	ProposedAt     string   `json:"proposedAt"`
	State          string   `json:"state"`
	Reason         string   `json:"reason,omitempty"`
	Destination    string   `json:"destination,omitempty"`
	DecidedAt      string   `json:"decidedAt,omitempty"`
	Bundle         string   `json:"bundle"`
	BundleMissing  bool     `json:"bundleMissing"`
	Valid          bool     `json:"valid"`
	Validation     []string `json:"validation"`
}

// SkillProposalDetailView is one proposal opened for review: the row's facts
// plus the `SKILL.md` body and, when the bundle cannot be read, the reason it
// could not be. A missing bundle is reported in place rather than turning the
// read into a failure.
type SkillProposalDetailView struct {
	SkillProposalView
	Body      string `json:"body"`
	BodyError string `json:"bodyError,omitempty"`
}

// SkillsInventoryView is the whole read-only surface behind /next/skills: the
// store inventory with its role resolution, and every proposal the trails hold
// with the decisions taken about it.
type SkillsInventoryView struct {
	Store          string              `json:"store"`
	Skills         []SkillRowView      `json:"skills"`
	Issues         []string            `json:"issues"`
	Proposals      []SkillProposalView `json:"proposals"`
	ProposalIssues []string            `json:"proposalIssues"`
}

// GetSkillsInventory reads the live store through the same inventory the shell
// uses, and joins every trail's proposals. An unreadable trail is carried as a
// proposal issue beside the rows rather than hidden, a bundle finding stays on
// its own row, and a store-wide finding is carried the same way.
func GetSkillsInventory(ctx colony.Context) (SkillsInventoryView, error) {
	inventory, err := skills.BuildInventory(ctx)
	if err != nil {
		return SkillsInventoryView{}, err
	}
	proposals, err := skills.ScanProposals(ctx.ColonyRoot)
	if err != nil {
		return SkillsInventoryView{}, err
	}

	view := SkillsInventoryView{
		Store:          inventory.Root,
		Skills:         make([]SkillRowView, 0, len(inventory.Skills)),
		Issues:         append([]string{}, inventory.Issues...),
		Proposals:      make([]SkillProposalView, 0, len(proposals.Proposals)),
		ProposalIssues: make([]string, 0, len(proposals.Issues)),
	}
	for _, row := range inventory.Skills {
		roles := row.Roles
		if roles == nil {
			roles = []string{}
		}
		view.Skills = append(view.Skills, SkillRowView{
			Name:        row.Name,
			State:       row.State,
			Reason:      row.Reason,
			Roles:       roles,
			Description: row.Description,
			Provenance:  row.Provenance,
		})
	}
	for _, proposal := range proposals.Proposals {
		view.Proposals = append(view.Proposals, skillProposalView(proposal))
	}
	for _, issue := range proposals.Issues {
		view.ProposalIssues = append(view.ProposalIssues, issue.Error())
	}
	return view, nil
}

// GetSkillProposal opens one proposal read-only: its row plus the procedure
// body a Beekeeper reads before deciding. A bundle that cannot be read is
// reported on the view rather than failing the read, because a missing bundle
// is a state of the proposal a reviewer has to see.
func GetSkillProposal(ctx colony.Context, trail, name string) (SkillProposalDetailView, error) {
	inventory, err := skills.ScanProposals(ctx.ColonyRoot)
	if err != nil {
		return SkillProposalDetailView{}, err
	}
	for _, proposal := range inventory.Proposals {
		if proposal.Trail != trail || proposal.Name != name {
			continue
		}
		detail := SkillProposalDetailView{
			SkillProposalView: skillProposalView(proposal),
		}
		body, err := skills.ReadProposalBody(ctx.ColonyRoot, proposal)
		if err != nil {
			detail.BodyError = err.Error()
		} else {
			detail.Body = body
		}
		return detail, nil
	}
	return SkillProposalDetailView{}, fmt.Errorf("skills: not found: no proposal %q in trail %s", name, trail)
}

func skillProposalView(proposal skills.Proposal) SkillProposalView {
	validation := proposal.Validation
	if validation == nil {
		validation = []string{}
	}
	return SkillProposalView{
		Trail:          proposal.Trail,
		Name:           proposal.Name,
		Description:    proposal.Description,
		ProposedReason: proposal.ProposedReason,
		ProposedAt:     timeColumn(proposal.ProposedAt),
		State:          proposal.State,
		Reason:         proposal.Reason,
		Destination:    proposal.Destination,
		DecidedAt:      timeColumn(proposal.DecidedAt),
		Bundle:         proposal.Bundle,
		BundleMissing:  proposal.BundleMissing,
		Valid:          proposal.Valid,
		Validation:     validation,
	}
}

// timeColumn keeps absent times empty rather than inventing an epoch: the page
// prints a dash for an absent time, and the wire says "never" not "1970".
func timeColumn(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format(time.RFC3339)
}

func (a *api) handleSkills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view, err := GetSkillsInventory(a.ctx)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}

func (a *api) handleSkillProposal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/skills/proposals/"), "/")
	ref, err := skills.ParseProposalReference(raw, "")
	if err != nil {
		// A path that does not name a proposal is nowhere to read, not a bad
		// request the client can correct into one.
		http.NotFound(w, r)
		return
	}
	view, err := GetSkillProposal(a.ctx, ref.Trail, ref.Name)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, view)
}
