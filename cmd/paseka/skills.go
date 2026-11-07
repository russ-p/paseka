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
	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsShowCmd())
	cmd.AddCommand(newSkillsLoadCmd())
	cmd.AddCommand(newSkillsValidateCmd())
	cmd.AddCommand(newSkillsProposalsCmd())
	cmd.AddCommand(newSkillsApproveCmd())
	cmd.AddCommand(newSkillsRejectCmd())
	cmd.AddCommand(newSkillsDisableCmd())
	cmd.AddCommand(newSkillsEnableCmd())
	cmd.AddCommand(newSkillsRemoveCmd())
	return cmd
}

// loadSkillStore resolves the live repository store and its ledger. Both read
// commands use it, and both refuse a ledger that cannot be read: a corrupt
// ledger must not look like an empty one, because that would turn a recorded
// disable into an offer.
//
// The store is the repository's, never a trail's frozen copy: no trail is in
// scope when a Beekeeper runs a shell command, and the frozen copy exists to
// keep a run reproducible, not to answer an operator's question about the
// store.
func loadSkillStore(startDir string) (colony.Context, skills.Report, skills.Ledger, error) {
	ctxColony, err := colony.ResolveContext(startDir)
	if err != nil {
		return colony.Context{}, skills.Report{}, skills.Ledger{}, err
	}
	store, err := skills.ScanColony(ctxColony.ColonyRoot)
	if err != nil {
		return colony.Context{}, skills.Report{}, skills.Ledger{}, err
	}
	ledger, err := skills.LoadLedger(ctxColony.ColonyRoot)
	if err != nil {
		return colony.Context{}, skills.Report{}, skills.Ledger{}, err
	}
	return ctxColony, store, ledger, nil
}

// selectSkillForMutation resolves one procedure for a verb that acts on one
// skill. A pattern names a set, so it is refused with the verb's name rather
// than silently acting on the first match. A bare name normally matches one
// entry; it can match a second when a different directory declares this name in
// broken frontmatter, so the entry actually named is preferred over the
// declaration.
func selectSkillForMutation(store skills.Report, raw, verb string) (skills.Skill, error) {
	ref, err := skills.ParseReference(raw)
	if err != nil {
		return skills.Skill{}, err
	}
	if ref.IsPattern() {
		return skills.Skill{}, fmt.Errorf(
			"skills: %s acts on one skill; %q is a pattern (use list or validate for a set)", verb, ref,
		)
	}
	matched, err := store.Select(ref)
	if err != nil {
		return skills.Skill{}, err
	}
	skill := matched[0]
	for _, candidate := range matched {
		if candidate.Name == ref.String() {
			skill = candidate
			break
		}
	}
	return skill, nil
}

// skillRoles maps a skill's store name to every role whose selection names it.
// The answer is computed from the role configurations on every run rather than
// remembered, and a pattern or `*` is expanded here: a wildcard role is named
// against every skill it actually resolves to, because that is what the gate
// does. A reference that does not parse is skipped — doctor and validate report
// it, and a listing must not invent a role from a name no resolver accepts.
func skillRoles(ctx colony.Context, store skills.Report) (map[string][]string, error) {
	bees, err := ctx.LoadAllBeesForDiagnosis()
	if err != nil {
		return nil, err
	}
	roles := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, role := range sortedBeeRoleNames(bees) {
		effective, err := ctx.EffectiveSkills(role, bees[role])
		if err != nil {
			return nil, err
		}
		if !effective.Present {
			continue
		}
		for _, raw := range effective.Refs {
			ref, err := skills.ParseReference(raw)
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

// skillState is the listing's verdict for one skill: `ok`, `invalid` with the
// reason, or `disabled`. A disabled skill is a recorded human decision, so it
// reads as disabled even if its bytes have since gone stale; validate and
// doctor still report the invalidity.
func skillState(skill skills.Skill, ledger skills.Ledger) (state, reason string) {
	if record, disabled := ledger.Record(skill.Name); disabled {
		return "disabled", record.Reason
	}
	if issues := skill.Issues; len(issues) > 0 {
		reasons := make([]string, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, issue.Reason)
		}
		return "invalid", strings.Join(reasons, "; ")
	}
	return "ok", ""
}

// skillListEntry is one row of `paseka skills list --json`. The field set is
// the contract: state and reason are separate here, while the table folds them
// into one column, so a script never has to parse prose.
type skillListEntry struct {
	Name        string   `json:"name"`
	State       string   `json:"state"`
	Reason      string   `json:"reason,omitempty"`
	Roles       []string `json:"roles"`
	Description string   `json:"description"`
	Provenance  string   `json:"provenance"`
}

// skillListDocument is the whole machine contract of `list`. `skills` and
// `issues` are always arrays so a consumer can range over them without a nil
// check, and `issues` carries the store-wide findings that have no skill row.
type skillListDocument struct {
	Store  string           `json:"store"`
	Skills []skillListEntry `json:"skills"`
	Issues []string         `json:"issues"`
}

func newSkillsListCmd() *cobra.Command {
	var (
		startDir string
		jsonOut  bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the skills in the store with their state and roles",
		Long: `List every skill in the repository store.

The columns are name, state, roles, description, and provenance. The state is
one of ok, invalid (with the reason), or disabled. The roles column names every
role whose selection resolves to the skill, computed from the role
configurations: a role that lists ` + "`*`" + ` or a matching pattern is named
against every skill it actually receives. Provenance is read from the skill's
own metadata: ` + "`proposal`" + `, ` + "`nuc`" + `, or ` + "`hand`" + `.

--json prints the same inventory as a stable machine contract.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, store, ledger, err := loadSkillStore(startDir)
			if err != nil {
				return err
			}
			roles, err := skillRoles(ctxColony, store)
			if err != nil {
				return err
			}

			doc := skillListDocument{
				Store:  store.Root,
				Skills: make([]skillListEntry, 0, len(store.Skills)),
				Issues: make([]string, 0, len(store.Issues())),
			}
			for _, skill := range store.Skills {
				state, reason := skillState(skill, ledger)
				names := roles[skill.Name]
				if names == nil {
					names = []string{}
				}
				doc.Skills = append(doc.Skills, skillListEntry{
					Name:        skill.Name,
					State:       state,
					Reason:      reason,
					Roles:       names,
					Description: skill.Description,
					Provenance:  skillProvenance(skill.Metadata),
				})
			}
			for _, issue := range store.Issues() {
				// A per-skill finding is already the row's state; only the
				// store-wide ones have nowhere else to appear.
				if issue.Skill == "" {
					doc.Issues = append(doc.Issues, issue.Error())
				}
			}

			out := cmd.OutOrStdout()
			if jsonOut {
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(doc)
			}

			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "NAME\tSTATE\tROLES\tDESCRIPTION\tPROVENANCE")
			for _, entry := range doc.Skills {
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					entry.Name, stateColumn(entry), rolesColumn(entry.Roles), entry.Description, entry.Provenance)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			for _, issue := range doc.Issues {
				if _, err := fmt.Fprintf(out, "store: %s\n", issue); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the inventory as JSON on stdout")
	return cmd
}

// stateColumn folds the state and its reason into the one column the table has.
func stateColumn(entry skillListEntry) string {
	return stateWithReason(entry.State, entry.Reason)
}

// stateWithReason is the one rendering of "state, with its reason when it has
// one", shared by the table and by `show`'s prose so the two cannot disagree.
func stateWithReason(state, reason string) string {
	if reason == "" {
		return state
	}
	return state + ": " + reason
}

// rolesColumn keeps an unassigned skill visible: a blank cell reads as a
// formatting accident, and "no role carries this" is the answer the column
// exists to give.
func rolesColumn(roles []string) string {
	if len(roles) == 0 {
		return "-"
	}
	return strings.Join(roles, ", ")
}

// skillShowDocument is the machine contract of `show`. Body is the body without
// frontmatter, exactly what the prose form prints.
type skillShowDocument struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	State       string            `json:"state"`
	Reason      string            `json:"reason,omitempty"`
	Roles       []string          `json:"roles"`
	Path        string            `json:"path"`
	Dir         string            `json:"dir"`
	Provenance  string            `json:"provenance"`
	Metadata    map[string]string `json:"metadata"`
	Body        string            `json:"body"`
	Issues      []string          `json:"issues"`
}

func newSkillsShowCmd() *cobra.Command {
	var (
		startDir string
		pathOnly bool
		jsonOut  bool
	)
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print one skill's metadata, provenance, and body",
		Long: `Show one skill from the repository store.

The reference must name one procedure: a bare name, never a pattern, because a
pattern names a set and show describes one skill. Provenance is read from the
skill's own metadata (` + "`metadata.source`" + `); a skill with no provenance
record is ` + "`hand`" + `.

--path prints only the resolved SKILL.md path, so a script can consume it
without parsing prose. --json prints the same facts as a machine contract.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A refused reference or an unknown name is a verdict, not a usage
			// error: the command ran and reported.
			cmd.SilenceUsage = true
			if pathOnly && jsonOut {
				return fmt.Errorf("skills: --path and --json cannot be combined")
			}

			ctxColony, store, ledger, err := loadSkillStore(startDir)
			if err != nil {
				return err
			}
			skill, err := selectSkillForMutation(store, args[0], "show")
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if pathOnly {
				_, err := fmt.Fprintln(out, skill.Path)
				return err
			}

			state, reason := skillState(skill, ledger)
			roles, err := skillRoles(ctxColony, store)
			if err != nil {
				return err
			}
			names := roles[skill.Name]
			if names == nil {
				names = []string{}
			}
			provenance := skillProvenance(skill.Metadata)
			issues := make([]string, 0, len(skill.Issues))
			for _, issue := range skill.Issues {
				issues = append(issues, issue.Reason)
			}

			if jsonOut {
				doc := skillShowDocument{
					Name:        skill.Name,
					Description: skill.Description,
					State:       state,
					Reason:      reason,
					Roles:       names,
					Path:        skill.Path,
					Dir:         skill.Dir,
					Provenance:  provenance,
					Metadata:    skill.Metadata,
					Body:        skill.Body,
					Issues:      issues,
				}
				if doc.Metadata == nil {
					doc.Metadata = map[string]string{}
				}
				encoder := json.NewEncoder(out)
				encoder.SetIndent("", "  ")
				return encoder.Encode(doc)
			}

			// A skill with no provenance record was written by hand; saying so is
			// more useful than an empty field.
			fmt.Fprintf(out, "name: %s\n", skill.Name)
			fmt.Fprintf(out, "description: %s\n", skill.Description)
			fmt.Fprintf(out, "path: %s\n", skill.Path)
			// A disabled skill says so; an invalid one already lists its findings
			// below, and repeating the reason in a state line would say it twice.
			if state == "disabled" {
				fmt.Fprintf(out, "state: %s\n", stateWithReason(state, reason))
			}
			fmt.Fprintf(out, "provenance: %s\n", provenance)
			if len(skill.Metadata) == 0 {
				fmt.Fprintln(out, "metadata: (none)")
			} else {
				fmt.Fprintln(out, "metadata:")
				keys := make([]string, 0, len(skill.Metadata))
				for key := range skill.Metadata {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					fmt.Fprintf(out, "  %s: %s\n", key, skill.Metadata[key])
				}
			}
			if len(issues) > 0 {
				fmt.Fprintln(out, "issues:")
				for _, issue := range issues {
					fmt.Fprintf(out, "  - %s\n", issue)
				}
			}
			fmt.Fprintln(out)
			// The body is printed without its frontmatter; the blank line the
			// format usually leaves after the closing fence is the separator this
			// command already printed.
			body := strings.TrimLeft(skill.Body, "\n")
			if _, err := fmt.Fprint(out, body); err != nil {
				return err
			}
			if body != "" && !strings.HasSuffix(body, "\n") {
				_, err := fmt.Fprintln(out)
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "colony", "C", "", "directory inside the git repository")
	// `--path` is the path-only mode, as the command's contract names it. The
	// group's usual `--path`/`-C` colony flag is therefore `--colony` here; the
	// `-C` shorthand is unchanged.
	cmd.Flags().BoolVar(&pathOnly, "path", false, "print only the resolved SKILL.md path")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the skill as JSON on stdout")
	return cmd
}

// skillProvenance reads provenance from the skill's own metadata. The store has
// no mirror to reconcile, and a skill with no record was written by hand.
func skillProvenance(metadata map[string]string) string {
	if source := strings.TrimSpace(metadata["source"]); source != "" {
		return source
	}
	return "hand"
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
			_, report, _, err := loadSkillStore(startDir)
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

// skillAffectedRoles renders the blast radius of a store mutation. An empty
// list is said plainly rather than left blank: "no role carries this" is the
// answer that makes a removal safe, and a blank line reads as a formatting
// accident.
func skillAffectedRoles(roles []string) string {
	if len(roles) == 0 {
		return "no role carries this skill"
	}
	return strings.Join(roles, ", ")
}

func newSkillsDisableCmd() *cobra.Command {
	var (
		startDir string
		reason   string
	)
	cmd := &cobra.Command{
		Use:   "disable <name>",
		Short: "Record that a skill must not be offered",
		Long: `Neutralise one skill without deleting it.

The skill stays in the store and keeps its history; the committed ledger
records when it was disabled and why, and the runtime skips it from then on. A
role that still lists it is not a configuration error: the disable is the
Beekeeper's own recorded decision, so the skill is reported as skipped rather
than turning an emergency stop into a colony-wide outage.

The verb acts on one procedure, so a pattern is refused. It names the roles
whose resolved set changes, and it never commits: the ledger change is left in
the working tree for the Beekeeper. Run ` + "`paseka skills enable <name>`" + ` to undo it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, store, ledger, err := loadSkillStore(startDir)
			if err != nil {
				return err
			}
			skill, err := selectSkillForMutation(store, args[0], "disable")
			if err != nil {
				return err
			}
			roles, err := skillRoles(ctxColony, store)
			if err != nil {
				return err
			}
			if err := ledger.Disable(skill.Name, reason, time.Now()); err != nil {
				return err
			}
			if err := skills.SaveLedger(ctxColony.ColonyRoot, ledger); err != nil {
				return err
			}
			record, _ := ledger.Record(skill.Name)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "disabled %q\n", skill.Name)
			fmt.Fprintf(out, "  ledger: %s (uncommitted; the Beekeeper commits it)\n", skills.LedgerPath(ctxColony.ColonyRoot))
			fmt.Fprintf(out, "  at: %s\n", record.At)
			fmt.Fprintf(out, "  reason: %s\n", record.Reason)
			fmt.Fprintf(out, "  affected roles: %s\n", skillAffectedRoles(roles[skill.Name]))
			fmt.Fprintf(out, "  the skill stays in the store; `paseka skills enable %s` restores it\n", skill.Name)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVarP(&reason, "reason", "r", "", "why the skill must not be offered (required)")
	return cmd
}

func newSkillsEnableCmd() *cobra.Command {
	var startDir string
	cmd := &cobra.Command{
		Use:   "enable <name>",
		Short: "Clear a skill's disabled record",
		Long: `Restore one disabled skill to the offer.

The ledger record is removed, so the next resolution offers the skill again.
The verb acts on one procedure, so a pattern is refused. It names the roles
whose resolved set changes and never commits: the ledger change is left in the
working tree for the Beekeeper.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, store, ledger, err := loadSkillStore(startDir)
			if err != nil {
				return err
			}
			skill, err := selectSkillForMutation(store, args[0], "enable")
			if err != nil {
				return err
			}
			roles, err := skillRoles(ctxColony, store)
			if err != nil {
				return err
			}
			changed, err := ledger.Enable(skill.Name)
			if err != nil {
				return err
			}
			if changed {
				if err := skills.SaveLedger(ctxColony.ColonyRoot, ledger); err != nil {
					return err
				}
			}

			out := cmd.OutOrStdout()
			if !changed {
				fmt.Fprintf(out, "%q was not disabled; nothing changed\n", skill.Name)
				return nil
			}
			fmt.Fprintf(out, "enabled %q\n", skill.Name)
			fmt.Fprintf(out, "  ledger: %s (uncommitted; the Beekeeper commits it)\n", skills.LedgerPath(ctxColony.ColonyRoot))
			fmt.Fprintf(out, "  affected roles: %s\n", skillAffectedRoles(roles[skill.Name]))
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	return cmd
}

func newSkillsRemoveCmd() *cobra.Command {
	var (
		startDir string
		force    bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Delete a skill from the store and its ledger record",
		Long: `Delete one skill's store entry and its enabled/disabled record.

Removal refuses while a role still references the skill, because deleting it
would silently strip a procedure from that role; the refusal names the roles,
and --force proceeds anyway. --dry-run prints the same plan — the store entry,
the ledger record, and the affected roles — without touching either.

The verb acts on one procedure, so a pattern is refused. It never commits: the
deleted directory and the ledger change are a working-tree change the Beekeeper
commits. Removing a skill does not delete a trail's decisions about it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			ctxColony, store, ledger, err := loadSkillStore(startDir)
			if err != nil {
				return err
			}
			skill, err := selectSkillForMutation(store, args[0], "remove")
			if err != nil {
				return err
			}
			roles, err := skillRoles(ctxColony, store)
			if err != nil {
				return err
			}
			referencing := roles[skill.Name]
			record, disabled := ledger.Record(skill.Name)

			out := cmd.OutOrStdout()
			if dryRun {
				fmt.Fprintf(out, "would remove %q\n", skill.Name)
			} else {
				fmt.Fprintf(out, "remove %q\n", skill.Name)
			}
			fmt.Fprintf(out, "  store entry: %s\n", skill.Dir)
			if disabled {
				fmt.Fprintf(out, "  ledger record: disabled at %s (%s)\n", record.At, record.Reason)
			} else {
				fmt.Fprintln(out, "  ledger record: none")
			}
			fmt.Fprintf(out, "  affected roles: %s\n", skillAffectedRoles(referencing))

			if len(referencing) > 0 && !force {
				if dryRun {
					fmt.Fprintf(out, "  would refuse: use --force to remove it anyway\n")
					fmt.Fprintln(out, "dry run: nothing was changed")
					return nil
				}
				return fmt.Errorf(
					"skills: remove %q: role(s) %s still reference it; use --force to remove it anyway",
					skill.Name, strings.Join(referencing, ", "),
				)
			}
			if dryRun {
				fmt.Fprintln(out, "dry run: nothing was changed")
				return nil
			}

			if _, err := skills.RemoveSkill(ctxColony.ColonyRoot, skill.Name); err != nil {
				return err
			}
			fmt.Fprintf(out, "removed %q from the store and the ledger\n", skill.Name)
			fmt.Fprintln(out, "  the working-tree change is uncommitted; the Beekeeper commits it")
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&force, "force", false, "remove even while roles still reference the skill")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan without touching the store")
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
