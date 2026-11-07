package skills

import (
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
)

// The listing's verdict vocabulary. A skill is one of three things to an
// operator: usable, broken, or deliberately turned off.
const (
	// StateOK means the skill passed the store rule set and is offered.
	StateOK = "ok"
	// StateInvalid means the skill failed the store rule set; Reason says why.
	StateInvalid = "invalid"
	// StateDisabled means the committed ledger neutralised the skill; Reason
	// carries the Beekeeper's recorded decision.
	StateDisabled = "disabled"

	// ProvenanceHand marks a skill with no provenance record: one written or
	// copied into the store by hand. The other values live in the install path
	// (ProvenanceProposal, ProvenanceNuc).
	ProvenanceHand = "hand"
)

// InventoryEntry is one row of the shared store inventory. It is the one shape
// the shell's `skills list` and the console's skills page read, so state,
// reason, roles, description, and provenance cannot disagree between them.
type InventoryEntry struct {
	Name        string
	State       string
	Reason      string
	Roles       []string
	Description string
	Provenance  string
}

// Inventory is one read of the live repository store, with each skill's
// verdict, the roles whose resolved selection carries it, and the store-wide
// findings that have no row of their own.
type Inventory struct {
	Root   string
	Skills []InventoryEntry
	// Issues carries the store-wide findings — a stray entry at the root, or a
	// budget breach. Per-skill findings are already the row's State and Reason.
	Issues []string
}

// BuildInventory reads the live repository store and its committed ledger for
// one colony, then resolves every role's gate against that same scan. It is the
// inventory behind `paseka skills list` and GET /api/skills: one read, one rule
// set, so the shell and the console cannot describe the store differently.
func BuildInventory(ctx colony.Context) (Inventory, error) {
	store, err := ScanColony(ctx.ColonyRoot)
	if err != nil {
		return Inventory{}, err
	}
	ledger, err := LoadLedger(ctx.ColonyRoot)
	if err != nil {
		return Inventory{}, err
	}
	roles, err := RoleMembership(ctx, store)
	if err != nil {
		return Inventory{}, err
	}

	inventory := Inventory{
		Root:   store.Root,
		Skills: make([]InventoryEntry, 0, len(store.Skills)),
		Issues: make([]string, 0, len(store.Issues())),
	}
	for _, skill := range store.Skills {
		state, reason := State(skill, ledger)
		names := roles[skill.Name]
		if names == nil {
			names = []string{}
		}
		inventory.Skills = append(inventory.Skills, InventoryEntry{
			Name:        skill.Name,
			State:       state,
			Reason:      reason,
			Roles:       names,
			Description: skill.Description,
			Provenance:  Provenance(skill.Metadata),
		})
	}
	for _, issue := range store.Issues() {
		// A per-skill finding is already the row's state; only the store-wide
		// ones have nowhere else to appear.
		if issue.Skill == "" {
			inventory.Issues = append(inventory.Issues, issue.Error())
		}
	}
	return inventory, nil
}

// RoleMembership maps every skill's store name to the roles whose resolved
// selection names it. A pattern or `*` is expanded here: a wildcard role is
// named against every skill it actually resolves to, because that is what the
// gate does, and a listing never shows `*` in place of the roles it covers.
//
// A reference that does not parse is skipped. doctor and validate report it,
// and a listing must not invent a role from a name no resolver accepts.
func RoleMembership(ctx colony.Context, store Report) (map[string][]string, error) {
	bees, err := ctx.LoadAllBeesForDiagnosis()
	if err != nil {
		return nil, err
	}
	roles := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, role := range sortedRoleNames(bees) {
		effective, err := ctx.EffectiveSkills(role, bees[role])
		if err != nil {
			return nil, err
		}
		if !effective.Present {
			continue
		}
		for _, raw := range effective.Refs {
			ref, err := ParseReference(raw)
			if err != nil {
				continue
			}
			for _, skill := range store.Skills {
				if !ref.Names(skill) {
					continue
				}
				if seen[skill.Name] == nil {
					seen[skill.Name] = map[string]bool{}
				}
				if seen[skill.Name][role] {
					continue
				}
				seen[skill.Name][role] = true
				roles[skill.Name] = append(roles[skill.Name], role)
			}
		}
	}
	return roles, nil
}

// State is the listing's verdict for one skill: `ok`, `invalid` with the
// reason, or `disabled` with the Beekeeper's reason. A disabled skill is a
// recorded human decision, so it reads as disabled even if its bytes have since
// gone stale; validate and doctor still report the invalidity.
func State(skill Skill, ledger Ledger) (state, reason string) {
	if record, disabled := ledger.Record(skill.Name); disabled {
		return StateDisabled, record.Reason
	}
	if issues := skill.Issues; len(issues) > 0 {
		reasons := make([]string, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, issue.Reason)
		}
		return StateInvalid, strings.Join(reasons, "; ")
	}
	return StateOK, ""
}

// Provenance reads provenance from the skill's own metadata. The store keeps no
// mirror to reconcile, and a skill with no record was written by hand.
func Provenance(metadata map[string]string) string {
	if source := strings.TrimSpace(metadata[MetadataSource]); source != "" {
		return source
	}
	return ProvenanceHand
}

func sortedRoleNames(bees map[string]colony.Bee) []string {
	roles := make([]string, 0, len(bees))
	for role := range bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	return roles
}
