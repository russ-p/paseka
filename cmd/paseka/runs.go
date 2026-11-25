package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/spf13/cobra"
)

// newRunsCmd is the operator's escape hatch for run records. The runtime both
// projects and reconciles orphans on its own; this command exists for the case
// where an operator wants the record closed now, without waiting for a runtime
// start.
func newRunsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "Inspect and repair run outcome records",
	}
	cmd.AddCommand(newRunsReconcileCmd())
	return cmd
}

func newRunsReconcileCmd() *cobra.Command {
	var (
		startDir string
		traceID  string
		agentID  string
		reason   string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Record a terminal failed outcome for a run whose process is gone",
		Long: `Record a terminal failed outcome for an in-flight run whose process is gone.

With --agent, that one run is reconciled even when it recorded no pid. Without
--agent, every orphaned run under the trail is reconciled: a status.json still
in flight, a recorded pid that no longer runs on this host, no result.json, and
no interactive session.

The check proves absence on this host only. A run started in another pid
namespace, or a pid reused by an unrelated process, can be misread; that is why
--agent refuses a recorded pid that is still alive unless --force is passed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			traceID = strings.TrimSpace(traceID)
			agentID = strings.TrimSpace(agentID)
			if traceID == "" {
				return fmt.Errorf("--trace is required")
			}
			if force && strings.TrimSpace(reason) == "" {
				return fmt.Errorf("--force requires --reason: a live process is being failed on purpose")
			}
			ctxColony, err := colony.ResolveContext(startDir)
			if err != nil {
				return err
			}

			if agentID != "" {
				return reconcileOneRun(cmd.OutOrStdout(), ctxColony.ColonyRoot, traceID, agentID, reason, force)
			}
			return reconcileOrphanedTrace(cmd.OutOrStdout(), ctxColony.ColonyRoot, traceID, reason)
		},
	}
	cmd.Flags().StringVarP(&startDir, "path", "C", "", "directory inside the git repository")
	cmd.Flags().StringVar(&traceID, "trace", "", "flight trail id")
	cmd.Flags().StringVar(&agentID, "agent", "", "run id; when omitted, every orphaned run in the trail")
	cmd.Flags().StringVar(&reason, "reason", "", "machine-readable reason (default "+protocol.ReasonProcessGone+", or operator_failed with --force)")
	cmd.Flags().BoolVar(&force, "force", false, "reconcile even when the recorded pid is still running")
	return cmd
}

func reconcileOneRun(out io.Writer, colonyRoot, traceID, agentID, reason string, force bool) error {
	d := runs.Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	snap, err := d.ReadStatus()
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("run %s/%s has no status.json; there is no lifecycle record to reconcile", traceID, agentID)
		}
		return err
	}
	if terminalRunStatus(snap.State) {
		fmt.Fprintf(out, "%s/%s is already %s; nothing to reconcile\n", traceID, agentID, snap.State)
		return nil
	}
	if !force && snap.PID > 0 && colony.ProcessAlive(snap.PID) {
		return fmt.Errorf(
			"run %s/%s records pid %d, which is still running; stop it first or pass --force --reason to fail a live run",
			traceID, agentID, snap.PID,
		)
	}
	changed, err := d.ReconcileInterruptedRun(reasonOrDefault(reason, force), "")
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(out, "%s/%s is already terminal; nothing to reconcile\n", traceID, agentID)
		return nil
	}
	fmt.Fprintf(out, "reconciled %s/%s as failed\n", traceID, agentID)
	return nil
}

func reconcileOrphanedTrace(out io.Writer, colonyRoot, traceID, reason string) error {
	orphans, err := runs.ScanOrphanedRuns(colonyRoot)
	if err != nil {
		return err
	}
	reconciled := 0
	for _, orphan := range orphans {
		if orphan.Dir.TraceID != traceID {
			continue
		}
		changed, err := orphan.Dir.ReconcileInterruptedRun(reasonOrDefault(reason, false), "")
		if err != nil {
			return err
		}
		if changed {
			reconciled++
			fmt.Fprintf(out, "reconciled %s/%s as failed (pid %d)\n", orphan.Dir.TraceID, orphan.Dir.AgentID, orphan.PID)
		}
	}
	if reconciled == 0 {
		fmt.Fprintf(out, "no orphaned runs under trace %s\n", traceID)
	}
	return nil
}

// reasonOrDefault keeps the classifier honest: the default names a dead
// process, and a forced live-run reconciliation gets its own word.
func reasonOrDefault(reason string, force bool) string {
	if reason = strings.TrimSpace(reason); reason != "" {
		return reason
	}
	if force {
		return "operator_failed"
	}
	return protocol.ReasonProcessGone
}

func terminalRunStatus(state protocol.RunStatus) bool {
	switch state {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		return true
	}
	return false
}
