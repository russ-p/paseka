package runs

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// RunRecordReconciliation is the runtime-owned correction finalization applies
// to a run's two outcome records, status.json and result.json.
//
// Finalization decides the run's outcome after the adapter exits: the check
// gate, then the completion contract. Both verdicts go through this one
// operation rather than each writer assembling the two records again, so the
// next runtime-owned verdict extends it instead of adding a second mechanism.
//
// State and Reason replace the recorded primary outcome only when State is set.
// An empty State records an accompanying verdict — a contract record, say —
// without relabelling a run another layer already failed, which is how a gate
// rejection, an adapter failure, a timeout, or a cancellation keeps its own
// reason. Note is appended to status.json error text and, when the primary
// outcome changed, to result.json diagnostics.error; appending is idempotent so
// repeating the same reconciliation cannot stack the message.
//
// Every rewrite is one atomic replacement: a temporary file in the same
// directory followed by a rename. A failed rewrite is returned, never swallowed.
type RunRecordReconciliation struct {
	State    protocol.RunStatus
	Reason   string
	Note     string
	Gate     *protocol.GateResult
	Contract *protocol.ContractRecord
}

// ReconcileRunRecords applies a finalization correction to both outcome records.
//
// It preserves every field it does not own: usage, provider session identity,
// process exit code, timestamps, artifacts, diagnostics other than the error
// text, existing gate evidence, and provenance. Repeating the same correction
// produces the same records.
func (d Dir) ReconcileRunRecords(r RunRecordReconciliation) error {
	note := strings.TrimSpace(r.Note)

	snap, err := d.ReadStatus()
	statusExists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !statusExists {
		snap = protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			StartedAt:       time.Now().UTC(),
		}
	}

	statusChanged := false
	if r.State != "" && (snap.State != r.State || snap.Reason != r.Reason) {
		snap.State = r.State
		snap.Reason = r.Reason
		statusChanged = true
	}
	if note != "" {
		if updated := appendNote(snap.Error, note); updated != snap.Error {
			snap.Error = updated
			statusChanged = true
		}
	}
	if r.Gate != nil {
		snap.Gate = r.Gate
		statusChanged = true
	}
	if statusChanged {
		if snap.FinishedAt.IsZero() && snap.State != "" && snap.State != protocol.StatusQueued && snap.State != protocol.StatusRunning {
			snap.FinishedAt = time.Now().UTC()
		}
		if err := d.WriteStatusSnapshot(snap); err != nil {
			return err
		}
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		if os.IsNotExist(err) {
			// No result record means the adapter never persisted one; there is
			// nothing to correct. Interactive sessions and legacy runs share
			// this shape and must not gain a field they never had.
			return nil
		}
		return err
	}
	if r.State != "" {
		res.Status = r.State
		res.Reason = r.Reason
	}
	if note != "" && r.State != "" {
		res.Diagnostics.Error = appendNote(res.Diagnostics.Error, note)
	}
	if r.Gate != nil {
		res.Gate = r.Gate
	}
	if r.Contract != nil {
		res.Contract = r.Contract
	}
	if res.FinishedAt.IsZero() && !snap.FinishedAt.IsZero() {
		res.FinishedAt = snap.FinishedAt
	}
	return d.WriteResult(res)
}

// EnsureResultRecord writes res when the run has no result.json yet.
//
// Adapters persist result.json as soon as the CLI exits, so finalization
// normally corrects an existing record. A fake adapter, a delegated adapter, or
// an adapter that crashed before writing its record otherwise leaves the
// runtime with no place to record its own verdict; finalization owns the
// persisted outcome, so it materializes the record from what the adapter
// returned. An unreadable record is surfaced, not overwritten.
func (d Dir) EnsureResultRecord(res protocol.Result) error {
	if _, err := d.ReadResultJSON(); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return d.WriteResult(res)
}

// appendNote adds note to existing once. Repeating reconciliation with the same
// verdict must not stack the same message, and a reader comparing a retried
// finalization sees identical bytes.
func appendNote(existing, note string) string {
	existing = strings.TrimSpace(existing)
	if note == "" || existing == note || strings.Contains(existing, note) {
		return existing
	}
	if existing == "" {
		return note
	}
	return existing + "; " + note
}

// writeFileAtomic replaces path with data through a temporary file in the same
// directory followed by a rename, so a reader never observes a half-written
// record and a crash leaves either the old record or the new one.
func writeFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	discard := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		discard()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		discard()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		discard()
		return err
	}
	if err := tmp.Close(); err != nil {
		discard()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		discard()
		return fmt.Errorf("runs: replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

// marshalRecord renders one run record with the same stable indentation every
// reader expects.
func marshalRecord(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return data, nil
}
