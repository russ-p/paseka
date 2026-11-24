package runs_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

// unusedHighPID returns a pid the kernel reports as not running, which is how a
// dead run is simulated without depending on a real process that happened to
// exit: an unallocated high pid is the stable stand-in.
func unusedHighPID(t *testing.T) int {
	t.Helper()
	for pid := 4_000_000; pid > 3_900_000; pid-- {
		if !runs.PIDAlive(pid) {
			return pid
		}
	}
	t.Skip("no unused high pid available on this host")
	return 0
}

func writeRunningStatus(t *testing.T, d runs.Dir, pid int, started time.Time) {
	t.Helper()
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusRunning,
		PID:             pid,
		StartedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRunMetaProjectsDeadOwnerAsFailed(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-orphan", "agent-1", started)
	dead := unusedHighPID(t)
	writeRunningStatus(t, d, dead, started)

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusFailed) {
		t.Fatalf("state = %q, want failed for a dead owner", meta.State)
	}
	if meta.Reason != protocol.ReasonProcessGone {
		t.Fatalf("reason = %q, want %q", meta.Reason, protocol.ReasonProcessGone)
	}
	if meta.Interruption == nil {
		t.Fatal("interruption = nil, want the reason an operator can read")
	}
	if meta.Interruption.PID != dead || meta.Interruption.Reason != protocol.ReasonProcessGone {
		t.Fatalf("interruption = %+v, want pid %d and reason %q", meta.Interruption, dead, protocol.ReasonProcessGone)
	}
	if meta.Interruption.Detail == "" {
		t.Fatal("interruption detail is empty; a failed projection must not be silent")
	}
	if meta.Records != nil {
		t.Fatalf("records = %+v, want no integrity marker: nothing was rewritten", meta.Records)
	}

	// The read path reports the interruption; it must not persist it.
	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusRunning {
		t.Fatalf("status.json state = %q, want the recorded running state untouched", snap.State)
	}
	if _, err := d.ReadResultJSON(); !os.IsNotExist(err) {
		t.Fatalf("result.json exists after a read-only projection: %v", err)
	}
}

func TestLoadRunMetaKeepsAliveOwnerRunning(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-alive", "agent-1", started)
	writeRunningStatus(t, d, os.Getpid(), started)

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusRunning) {
		t.Fatalf("state = %q, want running for a live pid", meta.State)
	}
	if meta.Interruption != nil {
		t.Fatalf("interruption = %+v, want none for a live pid", meta.Interruption)
	}
}

func TestLoadRunMetaKeepsShapesWithoutAnOwner(t *testing.T) {
	started := time.Now().UTC()

	t.Run("queued with no pid", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-queued", "agent-1", started)
		if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
			ProtocolVersion: protocol.Version,
			State:           protocol.StatusQueued,
			StartedAt:       started,
		}); err != nil {
			t.Fatal(err)
		}
		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.State != string(protocol.StatusQueued) || meta.Interruption != nil {
			t.Fatalf("meta = %+v, want queued with no interruption", meta)
		}
	})

	t.Run("interactive session with a dead pid", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-session", "agent-1", started)
		writeRunningStatus(t, d, unusedHighPID(t), started)
		if err := d.WriteSession(runs.SessionMeta{ProtocolVersion: protocol.Version, StartedAt: started}); err != nil {
			t.Fatal(err)
		}
		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.State != string(protocol.StatusRunning) || meta.Interruption != nil {
			t.Fatalf("meta = %+v, want the session's running projection unchanged", meta)
		}
	})

	t.Run("legacy record without status.json", func(t *testing.T) {
		root := t.TempDir()
		d := projectionRun(t, root, "trace-legacy", "agent-1", started)
		meta, err := runs.LoadRunMeta(d)
		if err != nil {
			t.Fatal(err)
		}
		if meta.State != string(protocol.StatusQueued) || meta.Interruption != nil {
			t.Fatalf("meta = %+v, want the legacy queued projection unchanged", meta)
		}
	})
}

func TestTrailSummaryStopsCallingAnOrphanActive(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC().Add(-time.Hour)
	orphan := projectionRun(t, root, "trace-stuck", "agent-dead", started)
	writeRunningStatus(t, orphan, unusedHighPID(t), started)
	alive := projectionRun(t, root, "trace-live", "agent-live", started.Add(time.Minute))
	writeRunningStatus(t, alive, os.Getpid(), started.Add(time.Minute))

	summaries, err := runs.ScanAllTraces(root)
	if err != nil {
		t.Fatal(err)
	}
	byTrace := map[string]runs.TraceSummary{}
	for _, s := range summaries {
		byTrace[s.TraceID] = s
	}

	if got := byTrace["trace-stuck"]; got.HasActive || !got.HasFailures {
		t.Fatalf("stuck trail summary = %+v, want failed and not active", got)
	}
	if got := byTrace["trace-live"]; !got.HasActive || got.HasFailures {
		t.Fatalf("live trail summary = %+v, want active and not failed", got)
	}
}

func TestScanOrphanedRunsDetection(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	dead := unusedHighPID(t)

	deadDir := projectionRun(t, root, "trace-a", "agent-dead", started)
	writeRunningStatus(t, deadDir, dead, started)

	noPID := projectionRun(t, root, "trace-a", "agent-no-pid", started)
	if err := noPID.WriteStatusSnapshot(protocol.StatusSnapshot{ProtocolVersion: protocol.Version, State: protocol.StatusRunning}); err != nil {
		t.Fatal(err)
	}

	aliveDir := projectionRun(t, root, "trace-a", "agent-alive", started)
	writeRunningStatus(t, aliveDir, os.Getpid(), started)

	sessionDir := projectionRun(t, root, "trace-a", "agent-session", started)
	writeRunningStatus(t, sessionDir, dead, started)
	if err := sessionDir.WriteSession(runs.SessionMeta{ProtocolVersion: protocol.Version}); err != nil {
		t.Fatal(err)
	}

	withResult := projectionRun(t, root, "trace-a", "agent-result", started)
	writeRunningStatus(t, withResult, dead, started)
	if err := withResult.WriteResult(protocol.Result{Status: protocol.StatusRunning}); err != nil {
		t.Fatal(err)
	}

	terminal := projectionRun(t, root, "trace-a", "agent-done", started)
	if err := terminal.WriteStatusSnapshot(protocol.StatusSnapshot{ProtocolVersion: protocol.Version, State: protocol.StatusCompleted}); err != nil {
		t.Fatal(err)
	}

	orphans, err := runs.ScanOrphanedRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 1 {
		t.Fatalf("orphans = %+v, want only agent-dead", orphans)
	}
	if orphans[0].Dir.AgentID != "agent-dead" || orphans[0].PID != dead {
		t.Fatalf("orphan = %+v, want agent-dead with pid %d", orphans[0], dead)
	}
}

func TestReconcileOrphanedRunsWritesNothingWithoutOrphans(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	alive := projectionRun(t, root, "trace-live", "agent-1", started)
	writeRunningStatus(t, alive, os.Getpid(), started)
	queued := projectionRun(t, root, "trace-queued", "agent-2", started)
	if err := queued.WriteStatusSnapshot(protocol.StatusSnapshot{ProtocolVersion: protocol.Version, State: protocol.StatusQueued}); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(alive.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := runs.ReconcileOrphanedRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconciled) != 0 {
		t.Fatalf("reconciled = %+v, want none", reconciled)
	}
	after, err := os.ReadFile(alive.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("status.json changed on a colony with no orphans:\nbefore %s\nafter  %s", before, after)
	}
	if _, err := alive.ReadResultJSON(); !os.IsNotExist(err) {
		t.Fatalf("a no-orphan pass wrote result.json: %v", err)
	}
}

func TestReconcileOrphanedRunsClosesTheOutcomePairOnce(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	dead := unusedHighPID(t)
	d := projectionRun(t, root, "trace-stuck", "agent-dead", started)
	writeRunningStatus(t, d, dead, started)

	reconciled, err := runs.ReconcileOrphanedRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reconciled) != 1 || reconciled[0].Dir.AgentID != "agent-dead" {
		t.Fatalf("reconciled = %+v, want the one orphan", reconciled)
	}

	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != protocol.StatusFailed || snap.Reason != protocol.ReasonProcessGone {
		t.Fatalf("status = %+v, want failed/%s", snap, protocol.ReasonProcessGone)
	}
	if snap.FinishedAt.IsZero() {
		t.Fatal("status.json finishedAt is zero, want the reconciliation instant")
	}
	if snap.Error == "" {
		t.Fatal("status.json error is empty, want the interruption reason")
	}

	res, err := d.ReadResultJSON()
	if err != nil {
		t.Fatalf("result.json was not materialized: %v", err)
	}
	if res.Status != protocol.StatusFailed || res.Reason != protocol.ReasonProcessGone {
		t.Fatalf("result = %+v, want failed/%s", res, protocol.ReasonProcessGone)
	}
	if res.Diagnostics.Error == "" {
		t.Fatal("result.json diagnostics.error is empty, want the interruption reason")
	}
	if !res.FinishedAt.Equal(snap.FinishedAt) {
		t.Fatalf("result finishedAt = %s, status finishedAt = %s; want one instant", res.FinishedAt, snap.FinishedAt)
	}

	statusBefore, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultBefore, err := os.ReadFile(d.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}

	again, err := runs.ReconcileOrphanedRuns(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("second pass reconciled %+v, want nothing", again)
	}
	statusAfter, err := os.ReadFile(d.StatusPath())
	if err != nil {
		t.Fatal(err)
	}
	resultAfter, err := os.ReadFile(d.ResultJSONPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(statusBefore) != string(statusAfter) || string(resultBefore) != string(resultAfter) {
		t.Fatal("a repeated pass changed an already-terminal run")
	}

	meta, err := runs.LoadRunMeta(d)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != string(protocol.StatusFailed) || meta.Records != nil || meta.Interruption != nil {
		t.Fatalf("meta after reconcile = %+v, want a plain failed pair", meta)
	}
}

func TestReconcileInterruptedRunIsIdempotentAndKeepsOneNote(t *testing.T) {
	root := t.TempDir()
	started := time.Now().UTC()
	d := projectionRun(t, root, "trace-manual", "agent-1", started)
	writeRunningStatus(t, d, unusedHighPID(t), started)

	changed, err := d.ReconcileInterruptedRun("", "")
	if err != nil || !changed {
		t.Fatalf("first reconcile = %v, %v; want changed", changed, err)
	}
	snap, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	firstNote := snap.Error
	if firstNote == "" {
		t.Fatal("first reconcile wrote no note")
	}

	changed, err = d.ReconcileInterruptedRun("", "")
	if err != nil || changed {
		t.Fatalf("second reconcile = %v, %v; want a no-op", changed, err)
	}
	again, err := d.ReadStatus()
	if err != nil {
		t.Fatal(err)
	}
	if again.Error != firstNote {
		t.Fatalf("note = %q, want the single original note %q", again.Error, firstNote)
	}
}

func TestReconcileInterruptedRunRequiresAStatusRecord(t *testing.T) {
	root := t.TempDir()
	d := projectionRun(t, root, "trace-legacy", "agent-1", time.Now().UTC())

	changed, err := d.ReconcileInterruptedRun("", "")
	if changed || !errors.Is(err, runs.ErrNoStatusRecord) {
		t.Fatalf("reconcile = %v, %v; want ErrNoStatusRecord", changed, err)
	}
	if _, err := os.Stat(filepath.Join(d.Root(), runs.ResultJSONFileName)); !os.IsNotExist(err) {
		t.Fatalf("result.json was written for a run with no status record: %v", err)
	}
}
