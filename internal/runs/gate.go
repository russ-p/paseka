package runs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
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
func (d Dir) WriteGateRecord(gate protocol.GateResult, state protocol.RunStatus, reason, note string) error {
	snap, err := d.ReadStatus()
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		snap = protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			StartedAt:       time.Now().UTC(),
		}
	}
	snap.State = state
	snap.Reason = reason
	if note = strings.TrimSpace(note); note != "" {
		if strings.TrimSpace(snap.Error) != "" {
			snap.Error += "; " + note
		} else {
			snap.Error = note
		}
	}
	snap.Gate = &gate
	if snap.FinishedAt.IsZero() {
		snap.FinishedAt = time.Now().UTC()
	}
	if err := d.WriteStatusSnapshot(snap); err != nil {
		return err
	}

	if res, readErr := d.ReadResultJSON(); readErr == nil {
		res.Status = state
		res.Reason = reason
		res.Gate = &gate
		if res.FinishedAt.IsZero() {
			res.FinishedAt = snap.FinishedAt
		}
		if err := d.WriteResult(res); err != nil {
			return err
		}
	}
	return nil
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
