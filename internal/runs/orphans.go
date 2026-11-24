package runs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
)

// ErrNoStatusRecord reports a manual reconciliation asked for a run directory
// that carries no status.json. A legacy run has no lifecycle record to repair,
// and inventing one would forge evidence the runtime never wrote.
var ErrNoStatusRecord = errors.New("runs: status.json is missing")

// OrphanedRun is one run whose status record still claims it is in flight while
// the process it recorded is gone.
type OrphanedRun struct {
	Dir   Dir
	State protocol.RunStatus
	PID   int
}

// ScanOrphanedRuns walks .paseka/runs and returns every run whose outcome pair
// is still in flight but whose owner is no longer running on this host.
//
// The walk reads status.json and checks the recorded pid; it deliberately reads
// no other record, so a startup pass costs one small file per run. A candidate
// is skipped when it records no pid (a queued run has no owner yet, and a run is
// never failed for lacking one), when it is an interactive session (session.json
// owns its liveness), and when a result.json exists (the pair already fail-
// closes through the projection, and rewriting a recorded outcome is not this
// pass's job).
func ScanOrphanedRuns(colonyRoot string) ([]OrphanedRun, error) {
	if colonyRoot == "" {
		return nil, nil
	}
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traces, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var orphans []OrphanedRun
	for _, traceEntry := range traces {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		agents, err := os.ReadDir(filepath.Join(runsRoot, traceID))
		if err != nil {
			continue
		}
		for _, agentEntry := range agents {
			if !agentEntry.IsDir() || IsReservedTraceSubdir(agentEntry.Name()) {
				continue
			}
			d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentEntry.Name()}
			if orphan, ok := orphanedRunIn(d); ok {
				orphans = append(orphans, orphan)
			}
		}
	}
	return orphans, nil
}

// orphanedRunIn reads one run's status.json and reports whether it is an
// orphan. Unreadable records are not candidates: a missing or malformed status
// is a different problem than a dead owner.
func orphanedRunIn(d Dir) (OrphanedRun, bool) {
	snap, err := d.ReadStatus()
	if err != nil || isTerminalRunStatus(snap.State) {
		return OrphanedRun{}, false
	}
	if fileExists(d.SessionPath()) || fileExists(d.ResultJSONPath()) {
		return OrphanedRun{}, false
	}
	if _, interrupted := projectInterruptedRun(string(snap.State), snap.PID, false); !interrupted {
		return OrphanedRun{}, false
	}
	return OrphanedRun{Dir: d, State: snap.State, PID: snap.PID}, true
}

// ReconcileOrphanedRuns persists a terminal failed outcome for every orphan the
// scan finds and returns the runs it repaired.
//
// This is the automatic pass: it runs once at runtime startup, detects from
// status.json plus a pid check, and writes nothing on a colony with no orphans.
// It is idempotent because a repaired run's status is terminal and no longer a
// candidate. One unreadable directory does not stop the others; the joined
// error names every failure.
func ReconcileOrphanedRuns(colonyRoot string) ([]OrphanedRun, error) {
	orphans, err := ScanOrphanedRuns(colonyRoot)
	if err != nil {
		return nil, err
	}
	reconciled := make([]OrphanedRun, 0, len(orphans))
	var errs []error
	for _, orphan := range orphans {
		changed, err := orphan.Dir.ReconcileInterruptedRun(protocol.ReasonProcessGone, "")
		if err != nil {
			errs = append(errs, fmt.Errorf("runs: reconcile %s/%s: %w", orphan.Dir.TraceID, orphan.Dir.AgentID, err))
			continue
		}
		if changed {
			reconciled = append(reconciled, orphan)
		}
	}
	return reconciled, errors.Join(errs...)
}

// ReconcileInterruptedRun writes a terminal failed outcome to both outcome
// records of a run that is still in flight on disk.
//
// The manual escape hatch and the automatic startup pass share it: the status
// record is corrected through ReconcileRunRecords, and a result record the
// adapter never persisted is materialized first so both halves of the outcome
// say failed. Each file is replaced atomically; the pair is written status
// first, so a crash in between still fail-closes in the projection.
//
// A run that is already terminal is left untouched and reports false, which is
// what makes a repeated call a no-op rather than a second note. An empty reason
// defaults to protocol.ReasonProcessGone; an empty note is derived from the
// recorded state and pid.
func (d Dir) ReconcileInterruptedRun(reason, note string) (bool, error) {
	snap, err := d.ReadStatus()
	if err != nil {
		if os.IsNotExist(err) {
			return false, ErrNoStatusRecord
		}
		return false, err
	}
	if isTerminalRunStatus(snap.State) {
		return false, nil
	}

	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = protocol.ReasonProcessGone
	}
	note = strings.TrimSpace(note)
	if note == "" {
		note = interruptedNote(snap)
	}

	if err := d.ensureFailedResult(reason); err != nil {
		return false, err
	}
	if err := d.ReconcileRunRecords(RunRecordReconciliation{
		State:  protocol.StatusFailed,
		Reason: reason,
		Note:   note,
	}); err != nil {
		return false, err
	}
	return true, nil
}

// interruptedNote is the sentence recorded when the caller supplied none. A
// dead pid gets the same detail the Console shows; an explicit operator
// reconciliation of a live or unrecorded process says so instead of claiming a
// check that did not run.
func interruptedNote(snap protocol.StatusSnapshot) string {
	if snap.PID > 0 && !PIDAlive(snap.PID) {
		return InterruptionDetail(snap.State, snap.PID)
	}
	return fmt.Sprintf("operator reconciled an in-flight run: status.json said %s", describeNonTerminal(snap.State))
}

// ensureFailedResult materializes result.json when the adapter never wrote one.
// An existing record is left to ReconcileRunRecords, which owns correcting it.
func (d Dir) ensureFailedResult(reason string) error {
	if _, err := d.ReadResultJSON(); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         d.TraceID,
		AgentID:         d.AgentID,
		Status:          protocol.StatusFailed,
		Reason:          reason,
	})
}
