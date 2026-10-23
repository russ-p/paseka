package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/tasks"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
	"github.com/spf13/cobra"
)

func newTrailCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "trail",
		Short: "Inspect flight trails and their configuration binding",
	}
	cmd.AddCommand(newTrailShowCmd())
	cmd.AddCommand(newTrailConfigCmd())
	cmd.AddCommand(newTrailForkCmd())
	return cmd
}

func newTrailForkCmd() *cobra.Command {
	var (
		startDir  string
		source    string
		taskID    string
		sourceRun string
		base      string
		delta     string
		noDelta   bool
		honey     int
		title     string
		requestID string
		dryRun    bool
		jsonOut   bool
	)
	cmd := &cobra.Command{
		Use:   "fork",
		Short: "Preview and create a no-delta control fork of an existing trail",
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(delta) != "" {
				return fmt.Errorf("--delta is not supported yet; use --no-delta for a control arm")
			}
			if !noDelta {
				return fmt.Errorf("choose --no-delta (control arm); --delta is not supported yet")
			}
			if strings.TrimSpace(source) == "" {
				return fmt.Errorf("--source <traceId> is required")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			opts := trailfork.Options{
				Context:       ctxColony,
				SourceTraceID: source,
				TaskID:        taskID,
				SourceRun:     sourceRun,
				BaseSHA:       base,
				Budget:        honey,
				Title:         title,
				RequestID:     requestID,
				Actor:         "cli",
				DryRun:        dryRun,
			}
			if dryRun {
				result, err := trailfork.Preview(opts)
				if err != nil {
					return err
				}
				if jsonOut {
					return printJSON(result)
				}
				printForkResult(result)
				return nil
			}

			session, err := tasks.OpenLedger(ctxColony)
			if err != nil {
				return err
			}
			defer session.Close()
			result, err := trailfork.Create(cmd.Context(), session.Publisher, session.Ledger, opts)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(result)
			}
			printForkResult(result)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&source, "source", "", "source trail id to fork")
	cmd.Flags().StringVar(&taskID, "task", "", "source task id (default: the single unambiguous root task)")
	cmd.Flags().StringVar(&sourceRun, "source-run", "", "source agent run id selecting one recorded attempt")
	cmd.Flags().StringVar(&base, "base", "", "explicit code base commit (removes the equal-base guarantee)")
	cmd.Flags().StringVar(&delta, "delta", "", "configuration patch record (not supported yet)")
	cmd.Flags().BoolVar(&noDelta, "no-delta", false, "create a control arm with the source configuration")
	cmd.Flags().IntVar(&honey, "honey", 0, "initial honey reserve (default: colony energy ceiling)")
	cmd.Flags().StringVar(&title, "title", "", "fork trail title")
	cmd.Flags().StringVar(&requestID, "request-id", "", "idempotency request id recorded in lineage")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and preview without durable changes")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

func newTrailConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect a trail's captured configuration",
	}
	cmd.AddCommand(newTrailConfigApplyCmd())
	cmd.AddCommand(newTrailConfigCancelCmd())
	cmd.AddCommand(newTrailConfigShowCmd())
	return cmd
}

func newTrailConfigCancelCmd() *cobra.Command {
	var (
		startDir string
		reason   string
		jsonOut  bool
	)
	cmd := &cobra.Command{
		Use:   "cancel <traceId> [changeId]",
		Short: "Cancel a pending configuration change without touching the active version",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := args[0]
			changeID := ""
			if len(args) == 2 {
				changeID = args[1]
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			manifest, err := trailconfig.Cancel(ctxColony.ColonyRoot, traceID, changeID, "cli", reason)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(manifest)
			}
			fmt.Printf("Cancelled pending configuration change for %s\n", traceID)
			fmt.Printf("activeConfigRef: %s (revision %d)\n", manifest.ActiveConfigRef, manifest.ConfigRevision)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&reason, "reason", "", "operator reason recorded in history")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

func newTrailConfigApplyCmd() *cobra.Command {
	var (
		startDir    string
		fromColony  bool
		snapshotRef string
		sets        []string
		reason      string
		requestID   string
		expected    int
		dryRun      bool
		jsonOut     bool
	)
	cmd := &cobra.Command{
		Use:   "apply <traceId>",
		Short: "Preview and apply the current colony configuration to a trail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID := args[0]
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			overrides, err := parseOverrides(sets)
			if err != nil {
				return err
			}
			source := ""
			sourceCount := 0
			if fromColony {
				sourceCount++
				source = trailconfig.SourceColony
			}
			if snapshotRef != "" {
				sourceCount++
				source = trailconfig.SourceSnapshot
			}
			if len(overrides) > 0 {
				sourceCount++
				source = trailconfig.SourceOverrides
			}
			if sourceCount != 1 {
				return fmt.Errorf("choose exactly one source: --from-colony, --snapshot <configRef>, or --set bee.<role>.<param>=<value>")
			}
			opts := trailconfig.ApplyOptions{
				Context:     ctxColony,
				TraceID:     traceID,
				Source:      source,
				SnapshotRef: snapshotRef,
				Overrides:   overrides,
				Actor:       "cli",
				Reason:      reason,
				RequestID:   requestID,
				DryRun:      dryRun,
			}
			if cmd.Flags().Changed("expected-revision") {
				opts.ExpectedRevision = &expected
			} else {
				binding, err := trailconfig.Read(ctxColony.ColonyRoot, traceID)
				if err != nil {
					return err
				}
				current := binding.ConfigRevision
				opts.ExpectedRevision = &current
			}
			result, err := trailconfig.Apply(opts)
			if err != nil {
				return err
			}
			if jsonOut {
				return printJSON(result)
			}
			printApplyResult(result)
			if result.Change.Outcome == trailconfig.OutcomeRejected {
				return fmt.Errorf("configuration change rejected: %s", result.Change.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&fromColony, "from-colony", false, "capture and apply the current colony configuration")
	cmd.Flags().StringVar(&snapshotRef, "snapshot", "", "activate a retained configuration snapshot ref")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "bee parameter override: bee.<role>.<param>=<value>")
	cmd.Flags().StringVar(&reason, "reason", "", "operator reason recorded in history")
	cmd.Flags().StringVar(&requestID, "request-id", "", "idempotency request id for retried deliveries")
	cmd.Flags().IntVar(&expected, "expected-revision", 0, "fail if the active revision differs")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "validate and preview without writing anything")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

func newTrailConfigShowCmd() *cobra.Command {
	var startDir string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <traceId>",
		Short: "Show initial/active ConfigRef, revision, history, and pending action",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}
			manifest, err := trailconfig.Load(ctxColony.ColonyRoot, args[0])
			if err != nil {
				if errors.Is(err, trailconfig.ErrNoBinding) {
					return fmt.Errorf("trail %s has no configuration binding", args[0])
				}
				return err
			}
			if jsonOut {
				return printJSON(manifest)
			}
			printTrailManifest(manifest, trailconfig.ConfigDir(ctxColony.ColonyRoot, manifest.TraceID))
			printConfigHistory(manifest)
			return nil
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print machine-readable JSON")
	return cmd
}

func newTrailShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <traceId>",
		Short: "Show a trail's configuration binding",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return showTrailBinding(args[0])
		},
	}
}

func showTrailBinding(traceID string) error {
	ctxColony, err := colony.ResolveContext("")
	if err != nil {
		return err
	}
	manifest, err := trailconfig.Load(ctxColony.ColonyRoot, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return fmt.Errorf("trail %s has no configuration binding", traceID)
		}
		return err
	}
	printTrailManifest(manifest, trailconfig.ConfigDir(ctxColony.ColonyRoot, manifest.TraceID))
	lineage, err := trailfork.LoadLineage(ctxColony.ColonyRoot, manifest.TraceID)
	switch {
	case err == nil:
		printForkLineage(lineage)
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	printConfigHistory(manifest)
	return nil
}

func parseOverrides(values []string) ([]trailconfig.Override, error) {
	var overrides []trailconfig.Override
	for _, raw := range values {
		eq := strings.Index(raw, "=")
		if eq < 0 {
			return nil, fmt.Errorf("invalid override %q: expected bee.<role>.<param>=<value>", raw)
		}
		key := strings.TrimSpace(raw[:eq])
		value := strings.TrimSpace(raw[eq+1:])
		parts := strings.Split(key, ".")
		if len(parts) != 3 || parts[0] != "bee" || parts[1] == "" || parts[2] == "" {
			return nil, fmt.Errorf("invalid override %q: expected bee.<role>.<param>=<value>", raw)
		}
		overrides = append(overrides, trailconfig.Override{Role: parts[1], Key: parts[2], Value: value})
	}
	return overrides, nil
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printTrailManifest(manifest trailconfig.Manifest, configDir string) {
	fmt.Printf("trace: %s\n", manifest.TraceID)
	fmt.Printf("policy: %s\n", manifest.Policy)
	fmt.Printf("revision: %d\n", manifest.ConfigRevision)
	fmt.Printf("initialConfigRef: %s\n", manifest.InitialConfigRef)
	fmt.Printf("activeConfigRef: %s\n", manifest.ActiveConfigRef)
	fmt.Printf("config: %s\n", configDir)
	if manifest.Pending != nil {
		fmt.Printf("pending: %s source=%s expected=%d\n", manifest.Pending.ChangeID, manifest.Pending.Source, manifest.Pending.ExpectedRevision)
		for _, blocker := range manifest.Pending.Blockers {
			fmt.Printf("  blocked by: %s\n", blocker)
		}
	}
}

func printConfigHistory(manifest trailconfig.Manifest) {
	if len(manifest.History) == 0 {
		return
	}
	fmt.Println("history:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  REVISION\tOUTCOME\tSOURCE\tACTOR\tOLD\tNEW\tEXPECTED\tREQUESTED\tAPPLIED")
	for _, change := range manifest.History {
		applied := ""
		if change.AppliedAt != nil {
			applied = change.AppliedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "  %d\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\n",
			change.Revision, change.Outcome, change.Source, change.Actor,
			change.OldConfigRef, change.NewConfigRef, change.ExpectedRevision,
			formatApplyTime(change.RequestedAt), applied)
	}
	_ = w.Flush()
}

func printForkLineage(lineage trailfork.Lineage) {
	fmt.Printf("forkId: %s\n", lineage.ForkID)
	fmt.Printf("forkOf: %s\n", lineage.SourceTrace)
	fmt.Printf("sourceTask: %s\n", lineage.SourceTask)
	fmt.Printf("sourceAttempt: %s\n", lineage.SourceAttempt)
	fmt.Printf("sourceConfigRef: %s (revision %d)\n", lineage.SourceConfigRef, lineage.SourceConfigRevision)
	fmt.Printf("baseSha: %s\n", lineage.BaseSHA)
	if lineage.BaseOverridden {
		fmt.Printf("baseOverriddenFrom: %s\n", lineage.SourceBaseSHA)
	}
	fmt.Printf("delta: %s\n", lineage.Delta.Form)
	fmt.Printf("workspace: %s\n", lineage.Workspace)
	fmt.Printf("branch: %s\n", lineage.Branch)
	fmt.Printf("budget: %d\n", lineage.Budget)
	fmt.Printf("state: %s\n", lineage.State)
}

func printForkResult(result trailfork.Result) {
	plan := result.Plan
	fmt.Printf("source: %s\n", plan.SourceTrace)
	fmt.Printf("source task: %s (attempt %s)\n", plan.SourceTask, plan.SourceAttempt)
	if plan.SourceRun != "" {
		fmt.Printf("source run: %s\n", plan.SourceRun)
	}
	fmt.Printf("source config: %s (revision %d)\n", plan.SourceConfigRef, plan.SourceConfigRevision)
	fmt.Printf("source task digest: %s\n", plan.Task.SourceTaskDigest)
	if plan.BaseOverridden {
		fmt.Printf("base: %s (overridden from %s)\n", plan.BaseSHA, plan.SourceBaseSHA)
	} else {
		fmt.Printf("base: %s\n", plan.BaseSHA)
	}
	fmt.Printf("fork trace: %s\n", plan.TraceID)
	fmt.Printf("fork task: %s\n", plan.Task.TaskID)
	fmt.Printf("task: %s\n", plan.Task.Title)
	fmt.Printf("bee: %s\n", plan.Task.Bee)
	fmt.Printf("delta: %s\n", plan.Delta.Form)
	fmt.Printf("policy: %s\n", plan.Policy)
	fmt.Printf("workspace: %s\n", plan.Workspace)
	fmt.Printf("branch: %s\n", plan.Branch)
	fmt.Printf("budget: %d\n", plan.Budget)
	fmt.Println("admission: colony NATS and task ledger required for creation")
	if result.Created {
		fmt.Printf("Created fork: lineage at %s\n", result.LineagePath)
	} else {
		fmt.Println("Dry run: no durable changes")
	}
	if len(plan.Limitations) > 0 {
		fmt.Println("Limitations:")
		for _, limitation := range plan.Limitations {
			fmt.Println("  - " + limitation)
		}
	}
}

func printApplyResult(result trailconfig.ApplyResult) {
	change := result.Change
	fmt.Printf("trace: %s\n", result.Manifest.TraceID)
	fmt.Printf("source: %s\n", change.Source)
	fmt.Printf("actor: %s\n", change.Actor)
	fmt.Printf("expected revision: %d\n", change.ExpectedRevision)
	fmt.Printf("oldConfigRef: %s\n", change.OldConfigRef)
	fmt.Printf("newConfigRef: %s\n", change.NewConfigRef)
	if result.RemainingWork.Evaluated {
		if !result.RemainingWork.Compatible {
			fmt.Println("remaining work: incompatible")
			for _, problem := range result.RemainingWork.Problems {
				fmt.Println("  - " + problem)
			}
		} else if len(result.RemainingWork.Checked) == 0 {
			fmt.Println("remaining work: none")
		} else {
			fmt.Printf("remaining work: compatible (%d unstarted: %s)\n",
				len(result.RemainingWork.Checked), strings.Join(result.RemainingWork.Checked, ", "))
		}
	}
	if result.DryRun {
		fmt.Printf("Dry run: %s -> %s\n", change.OldConfigRef, result.NewRef)
	} else {
		switch change.Outcome {
		case trailconfig.OutcomeApplied:
			fmt.Printf("Configuration applied: %s -> %s (revision %d)\n", change.OldConfigRef, change.NewConfigRef, result.Manifest.ConfigRevision)
		case trailconfig.OutcomeUnchanged:
			fmt.Printf("Configuration unchanged: %s (revision %d)\n", change.NewConfigRef, result.Manifest.ConfigRevision)
		case trailconfig.OutcomePending:
			fmt.Printf("Configuration pending: %s -> %s (revision %d)\n", change.OldConfigRef, change.NewConfigRef, result.Manifest.ConfigRevision)
		case trailconfig.OutcomeRejected:
			fmt.Printf("Configuration rejected: %s\n", change.Detail)
		}
	}
	fmt.Printf("outcome: %s\n", change.Outcome)
	if len(change.Blockers) > 0 {
		fmt.Println("Blocked by:")
		for _, blocker := range change.Blockers {
			fmt.Println("  - " + blocker)
		}
	}
	if len(result.Diff) > 0 {
		fmt.Println("Diff:")
		for _, line := range result.Diff {
			fmt.Println("  " + line)
		}
	}
	if len(result.EffectiveDiff) > 0 {
		fmt.Println("Effective diff:")
		for _, line := range result.EffectiveDiff {
			fmt.Println("  " + line)
		}
	}
	if len(result.ShadowedDiff) > 0 {
		fmt.Println("Shadowed changes:")
		for _, line := range result.ShadowedDiff {
			fmt.Println("  " + line)
		}
	}
}

func formatApplyTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
