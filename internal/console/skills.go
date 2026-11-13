package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
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

// SkillsInventoryView is the whole read-only surface behind /skills: the
// store inventory with its role resolution, and every proposal the trails hold
// with the decisions taken about it.
type SkillsInventoryView struct {
	Store          string              `json:"store"`
	Skills         []SkillRowView      `json:"skills"`
	Issues         []string            `json:"issues"`
	Proposals      []SkillProposalView `json:"proposals"`
	ProposalIssues []string            `json:"proposalIssues"`
}

// SkillApprovalPlanView is the confirmation an approval shows before it writes
// anything: the skill, the trail it came from, the destination and every path
// the install would create, and the working-tree consequence. It is produced by
// the same validated batch path as `paseka skills approve --dry-run`.
type SkillApprovalPlanView struct {
	Trail         string            `json:"trail"`
	Name          string            `json:"name"`
	Destination   string            `json:"destination"`
	Store         string            `json:"store"`
	Target        string            `json:"target"`
	Files         []string          `json:"files"`
	ProcedureHash string            `json:"procedureHash"`
	Provenance    map[string]string `json:"provenance"`
	Replaced      bool              `json:"replaced"`
	Disabled      bool              `json:"disabled"`
	DisableReason string            `json:"disableReason,omitempty"`
	WorkingTree   string            `json:"workingTree"`
}

// SkillDecisionResultView is what a completed approval or refusal returns: the
// refreshed inventory (so the page does not need a second read), the row's new
// state, and the sentence the operator reads as the receipt.
type SkillDecisionResultView struct {
	Inventory SkillsInventoryView `json:"inventory"`
	Proposal  SkillProposalView   `json:"proposal"`
	Message   string              `json:"message"`
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

// GetSkillApprovalPlan validates an approval without writing. It calls the same
// batch path `paseka skills approve` calls, in dry-run mode, so the plan names
// the destination and every file the real action would write and refuses
// exactly what the real action would refuse.
func GetSkillApprovalPlan(ctx colony.Context, trail, name string) (SkillApprovalPlanView, error) {
	approvals, err := skills.ApproveProposals(
		ctx.ColonyRoot, []string{trail + "/" + name}, "", false, time.Now(), true,
	)
	if err != nil {
		return SkillApprovalPlanView{}, err
	}
	if len(approvals) != 1 {
		return SkillApprovalPlanView{}, fmt.Errorf(
			"skills: expected one approval plan for %s/%s, got %d", trail, name, len(approvals),
		)
	}
	approval := approvals[0]
	target := repoRelativePath(ctx.ColonyRoot, approval.Target)
	return SkillApprovalPlanView{
		Trail:         approval.Proposal.Trail,
		Name:          approval.Proposal.Name,
		Destination:   approval.Decision.Destination,
		Store:         repoRelativePath(ctx.ColonyRoot, skills.Dir(ctx.ColonyRoot)),
		Target:        target,
		Files:         append([]string{}, approval.Files...),
		ProcedureHash: approval.Decision.ProcedureHash,
		Provenance:    approval.Provenance,
		Replaced:      approval.Replaced,
		Disabled:      approval.Disabled,
		DisableReason: approval.DisableReason,
		WorkingTree:   approvalWorkingTreeConsequence(approval, target),
	}, nil
}

// ApproveSkillProposal installs one proposal into the store through the same
// validated batch path as `paseka skills approve`. The console passes no skill
// body and no force flag: the bundle's bytes and the store's conflict rule are
// the only inputs, and the command never commits.
func ApproveSkillProposal(ctx colony.Context, trail, name string) (SkillDecisionResultView, error) {
	approvals, err := skills.ApproveProposals(
		ctx.ColonyRoot, []string{trail + "/" + name}, "", false, time.Now(), false,
	)
	if err != nil {
		return SkillDecisionResultView{}, err
	}
	if len(approvals) != 1 {
		return SkillDecisionResultView{}, fmt.Errorf(
			"skills: expected one installed proposal for %s/%s, got %d", trail, name, len(approvals),
		)
	}
	approval := approvals[0]
	target := repoRelativePath(ctx.ColonyRoot, approval.Target)
	message := fmt.Sprintf(
		"Installed %s/%s into the store at %s; %d working-tree path(s) written. Nothing was committed.",
		trail, name, target, len(approval.Files),
	)
	if approval.Disabled {
		reason := strings.TrimSpace(approval.DisableReason)
		if reason == "" {
			reason = "no reason recorded"
		}
		message += fmt.Sprintf(" The existing disable record (%s) was kept, so the skill stays disabled.", reason)
	}
	return skillDecisionResult(ctx, trail, name, message)
}

// RejectSkillProposal records one refusal through the same terminal decision
// path as `paseka skills reject`. A repeat refusal reports the existing decision
// without appending a second line.
func RejectSkillProposal(ctx colony.Context, trail, name, reason string) (SkillDecisionResultView, error) {
	result, err := skills.RejectProposal(
		ctx.ColonyRoot, trail+"/"+name, "", reason, time.Now(), false,
	)
	if err != nil {
		return SkillDecisionResultView{}, err
	}
	message := fmt.Sprintf(
		"Refused %s/%s; the decision is terminal for this bundle and lives with the source trail. Nothing was committed.",
		trail, name,
	)
	if result.AlreadyRefused {
		message = fmt.Sprintf(
			"Proposal %s/%s was already refused; no new decision was recorded.", trail, name,
		)
	}
	return skillDecisionResult(ctx, trail, name, message)
}

// skillDecisionResult re-reads the one inventory the page shows, so the row and
// the store land updated in the same answer and no manual refresh is needed.
func skillDecisionResult(ctx colony.Context, trail, name, message string) (SkillDecisionResultView, error) {
	inventory, err := GetSkillsInventory(ctx)
	if err != nil {
		return SkillDecisionResultView{}, fmt.Errorf(
			"skills: the decision was recorded, but the inventory could not be re-read: %w", err,
		)
	}
	result := SkillDecisionResultView{Inventory: inventory, Message: message}
	for _, proposal := range inventory.Proposals {
		if proposal.Trail == trail && proposal.Name == name {
			result.Proposal = proposal
			break
		}
	}
	return result, nil
}

// repoRelativePath keeps paths on the wire in the repository's vocabulary, the
// same way the shell names every path an install writes.
func repoRelativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// approvalWorkingTreeConsequence states what the action does to the working
// tree. A replacement is not untracked, and a new entry is, so the two are not
// described with one sentence.
func approvalWorkingTreeConsequence(approval skills.ProposalApproval, target string) string {
	if approval.Replaced {
		return fmt.Sprintf(
			"Replaces the existing skill directory at %s with %d file path(s) in the working tree. Committing is yours; this action never commits.",
			target, len(approval.Files),
		)
	}
	return fmt.Sprintf(
		"Writes %d new untracked file path(s) under %s in the working tree. Committing is yours; this action never commits.",
		len(approval.Files), target,
	)
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
	raw := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/skills/proposals/"), "/")
	segments := strings.Split(raw, "/")
	action := ""
	if len(segments) == 3 {
		switch segments[2] {
		case "plan", "approve", "reject":
			action = segments[2]
			raw = segments[0] + "/" + segments[1]
		default:
			// A path that names no action is nowhere to read: it must not be
			// mistaken for a proposal whose name happens to be the last segment.
			http.NotFound(w, r)
			return
		}
	}
	ref, err := skills.ParseProposalReference(raw, "")
	if err != nil {
		// A path that does not name a proposal is nowhere to read, not a bad
		// request the client can correct into one.
		http.NotFound(w, r)
		return
	}

	switch action {
	case "":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		view, err := GetSkillProposal(a.ctx, ref.Trail, ref.Name)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, view)
	case "plan":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		plan, err := GetSkillApprovalPlan(a.ctx, ref.Trail, ref.Name)
		if err != nil {
			writeSkillDecisionError(w, err)
			return
		}
		writeJSON(w, plan)
	case "approve":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.runSkillDecision(w, func() (SkillDecisionResultView, error) {
			return ApproveSkillProposal(a.ctx, ref.Trail, ref.Name)
		})
	case "reject":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !a.beginSkillDecision(w) {
			return
		}
		defer a.skillMutations.end()
		reason, err := decodeSkillRejectReason(r)
		if err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		result, err := RejectSkillProposal(a.ctx, ref.Trail, ref.Name, reason)
		if err != nil {
			writeSkillDecisionError(w, err)
			return
		}
		writeJSON(w, result)
	}
}

// runSkillDecision holds the one-mutation gate around an approval and writes its
// result. A second decision already in flight is refused with 409 rather than
// queued, because the first may change the state the second was computed from.
func (a *api) runSkillDecision(w http.ResponseWriter, run func() (SkillDecisionResultView, error)) {
	if !a.beginSkillDecision(w) {
		return
	}
	defer a.skillMutations.end()
	result, err := run()
	if err != nil {
		writeSkillDecisionError(w, err)
		return
	}
	writeJSON(w, result)
}

func (a *api) beginSkillDecision(w http.ResponseWriter) bool {
	if a.skillMutations.tryBegin() {
		return true
	}
	http.Error(w,
		"skills: another proposal decision is in flight; the console refuses a second one rather than queueing it",
		http.StatusConflict)
	return false
}

// decodeSkillRejectReason reads the optional reason. The body has no skill
// content: a console path never accepts a procedure body.
func decodeSkillRejectReason(r *http.Request) (string, error) {
	if r.Body == nil || r.ContentLength == 0 {
		return "", nil
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(req.Reason), nil
}

// writeSkillDecisionError keeps the server's reason in the body: a failed
// decision must say why it wrote nothing, not merely that it failed.
func writeSkillDecisionError(w http.ResponseWriter, err error) {
	message := err.Error()
	status := http.StatusBadRequest
	if strings.Contains(message, "not found") || strings.Contains(message, "no proposal") {
		status = http.StatusNotFound
	}
	http.Error(w, message, status)
}
