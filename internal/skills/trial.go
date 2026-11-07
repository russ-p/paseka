package skills

import (
	"fmt"
	"strings"
	"time"
)

// TrialFile is one file of a trialled skill bundle, relative to the skill
// directory and with the bytes a candidate store would receive.
type TrialFile struct {
	Rel  string
	Data []byte
}

// TrialBundle is one proposal prepared for a trail's candidate configuration:
// the bytes to add under the candidate store, the provenance written into the
// skill, and the decision line the trial will append when it applies.
type TrialBundle struct {
	Proposal   Proposal
	Files      []TrialFile
	Provenance map[string]string
	Decision   ProposalDecision
	Line       []byte
}

// Name returns the skill name the candidate store will use.
func (b TrialBundle) Name() string { return b.Proposal.Name }

// PrepareTrialBundles resolves one or more proposal references for a trial
// into the exact bundles a destination trail's candidate store would receive.
//
// The same reference rules and bundle validation as `approve` apply: a pattern,
// an ambiguous bare name, a missing bundle, an invalid bundle, a changed
// procedure hash, and a refused proposal are all refused here. Nothing is
// written; the candidate and the decisions are assembled by the caller, and a
// decision is recorded only after the candidate applies.
func PrepareTrialBundles(
	colonyRoot string,
	raws []string,
	defaultTrail string,
	destination string,
	roles []string,
	at time.Time,
) ([]TrialBundle, error) {
	if len(raws) == 0 {
		return nil, fmt.Errorf("skills: approve needs at least one proposal reference")
	}
	destination = strings.TrimSpace(destination)
	if destination == "" {
		return nil, fmt.Errorf("skills: a trial needs a destination trail")
	}
	if destination == ProposalStoreDestination {
		return nil, fmt.Errorf(
			"skills: a trial's destination is a trail, not %q; omit --into to install into the store",
			ProposalStoreDestination,
		)
	}
	if !validTrailToken(destination) {
		return nil, fmt.Errorf("skills: destination %q is not a trail id", destination)
	}
	roles = normalizeTrialRoles(roles)
	if len(roles) == 0 {
		return nil, fmt.Errorf(
			"skills: a trial needs at least one --role; a trial that adds a skill no role carries changes nothing",
		)
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()

	inventory, err := ScanProposals(colonyRoot)
	if err != nil {
		return nil, err
	}
	selected, err := selectApprovalProposals(inventory, raws, defaultTrail)
	if err != nil {
		return nil, err
	}

	bundles := make([]TrialBundle, 0, len(selected))
	for _, proposal := range selected {
		switch proposal.State {
		case ProposalRefused:
			return nil, fmt.Errorf(
				"skills: proposal %s/%s was already refused; a refusal is terminal for that exact bundle",
				proposal.Trail, proposal.Name,
			)
		case ProposalMissing:
			return nil, fmt.Errorf(
				"skills: proposal %s/%s has no bundle at %q; there is nothing to trial",
				proposal.Trail, proposal.Name, proposal.Bundle,
			)
		}
		if proposal.BundleMissing {
			return nil, fmt.Errorf(
				"skills: proposal %s/%s has no bundle at %q; there is nothing to trial",
				proposal.Trail, proposal.Name, proposal.Bundle,
			)
		}
		if !proposal.Valid {
			validation := "invalid bundle"
			if len(proposal.Validation) > 0 {
				validation = strings.Join(proposal.Validation, "; ")
			}
			return nil, fmt.Errorf(
				"skills: proposal %s/%s cannot be trialled: %s",
				proposal.Trail, proposal.Name, validation,
			)
		}

		plan, hash, err := readValidatedBundle(colonyRoot, proposal)
		if err != nil {
			return nil, err
		}
		provenance := map[string]string{
			MetadataSource:        ProvenanceProposal,
			MetadataTrail:         proposal.Trail,
			MetadataRole:          proposalRole(colonyRoot, proposal),
			MetadataApprovedAt:    at.Format(time.RFC3339),
			MetadataProcedureHash: hash,
		}
		manifestIndex := -1
		for i, file := range plan.files {
			if file.Rel == ManifestName {
				manifestIndex = i
				break
			}
		}
		updated, err := writeProvenance(plan.files[manifestIndex].Data, provenance)
		if err != nil {
			return nil, fmt.Errorf("skills: proposal %s/%s: %w", proposal.Trail, proposal.Name, err)
		}
		plan.files[manifestIndex].Data = updated

		files := make([]TrialFile, 0, len(plan.files))
		for _, file := range plan.files {
			files = append(files, TrialFile(file))
		}

		decision := ProposalDecision{
			Skill:         proposal.Name,
			Trail:         proposal.Trail,
			ProcedureHash: hash,
			Destination:   destination,
			Decision:      ProposalTrialled,
			Roles:         append([]string(nil), roles...),
			At:            at,
		}
		line, err := EncodeProposalDecision(decision)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, TrialBundle{
			Proposal:   proposal,
			Files:      files,
			Provenance: provenance,
			Decision:   decision,
			Line:       line,
		})
	}
	return bundles, nil
}

// AppendTrialDecisions records the trialled decision for each applied bundle.
// Each decision goes to the trail that proposed that skill; the destination
// trail is only named in the record, never written by this function.
func AppendTrialDecisions(colonyRoot string, bundles []TrialBundle) error {
	for _, bundle := range bundles {
		if err := AppendProposalDecision(colonyRoot, bundle.Proposal.Trail, bundle.Decision); err != nil {
			return err
		}
	}
	return nil
}

// normalizeTrialRoles trims and deduplicates the role list while preserving
// the order the operator wrote, so the decision reads back the same way.
func normalizeTrialRoles(roles []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roles))
	for _, raw := range roles {
		role := strings.TrimSpace(raw)
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		out = append(out, role)
	}
	return out
}
