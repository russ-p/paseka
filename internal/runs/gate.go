package runs

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// CheckLogFileName is the captured output of one runtime check gate.
const CheckLogFileName = "checks.log"

// CheckLogPath returns the captured gate output path for this run.
func (d Dir) CheckLogPath() string {
	return filepath.Join(d.Root(), CheckLogFileName)
}

// WriteGateRecord records a gate verdict on the run's status and result records.
//
// state is the run's final state (completed for a pass, failed for a rejection)
// and reason is the machine-readable classifier the status snapshot carries. A
// note is appended to the existing error text rather than replacing it, so an
// adapter error survives alongside the gate verdict.
//
// The gate writer is one caller of ReconcileRunRecords: the mechanical gate and
// the completion contract share the same reconciliation operation, so both
// records are corrected the same way and atomically.
func (d Dir) WriteGateRecord(gate protocol.GateResult, state protocol.RunStatus, reason, note string) error {
	return d.ReconcileRunRecords(RunRecordReconciliation{
		State:  state,
		Reason: reason,
		Note:   note,
		Gate:   &gate,
	})
}

// RecordedGateRun is one gate verdict recorded on a run under a trace, together
// with the task attempt context its request recorded.
type RecordedGateRun struct {
	TaskID    string
	AttemptID string
	StartedAt time.Time
	Gate      protocol.GateResult
}

// RecordedGateRuns returns every recorded gate verdict under a trace, oldest
// first. A record without a gate is skipped: an un-gated run never clears or
// advances a rejection count, it simply says nothing.
func RecordedGateRuns(colonyRoot, traceID string) ([]RecordedGateRun, error) {
	root := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []RecordedGateRun
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: ent.Name()}
		snap, err := d.ReadStatus()
		if err != nil || snap.Gate == nil {
			continue
		}
		req, _ := d.ReadRequest()
		started := snap.StartedAt
		if started.IsZero() {
			started = req.CreatedAt
		}
		out = append(out, RecordedGateRun{
			TaskID:    req.TaskID,
			AttemptID: req.AttemptID,
			StartedAt: started,
			Gate:      *snap.Gate,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}
