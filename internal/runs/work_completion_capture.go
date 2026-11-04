package runs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/gitroot"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

// CaptureWorkCompletion writes the durable evidence for one trail's accepted
// work: the immutable checkpoint, the frozen metrics snapshot, the retained
// result (including the final patch/tree and base/revision for a code result),
// and the current work record.
//
// It is deliberately delivery-independent. The reactor calls it before
// publishing trail.work.completed; an approval or reconcile path calls it
// before a merge, a pull-request publication, or a worktree removal. Both paths
// converge on the same deterministic completion identity, so a second capture
// fills only evidence a crash left absent and never rewrites an existing
// snapshot.
//
// The caller owns the work-completion predicate: capture does not decide that
// work is complete, it only makes a certified boundary durable.
func CaptureWorkCompletion(colonyRoot string, trace taskledger.TraceSnapshot, completedAt time.Time) (WorkCompletionCheckpoint, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: colony root is required")
	}
	traceID := strings.TrimSpace(trace.TraceID)
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	if trace.WorkRevision < 1 {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: trail %q has no accepted work revision", traceID)
	}
	if completedAt.IsZero() {
		completedAt = time.Now().UTC()
	}
	completedAt = completedAt.UTC()

	completionID := taskledger.CompletionIDForRevision(traceID, trace.WorkRevision)

	metrics, err := BuildTrailMetrics(colonyRoot, traceID, completedAt)
	if err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	metrics.Work.Revision = &trace.WorkRevision
	metrics.Work.Completion = &MetricsCompletion{
		CompletionID: completionID,
		WorkRevision: trace.WorkRevision,
		CompletedAt:  completedAt,
		ResultRef:    WorkCompletionResultRef(traceID, completionID),
		MetricsRef:   WorkCompletionMetricsRef(traceID, completionID),
	}

	result := completionResultFromSnapshot(colonyRoot, traceID, trace, metrics)
	evidence := captureWorktreeEvidence(colonyRoot, traceID)
	result.Base = evidence.Base
	result.Revision = evidence.Revision
	result.Tree = evidence.Tree
	result.Patch = evidence.Patch
	result.Artifacts = retainedArtifactRefs(colonyRoot, traceID)

	checkpoint, err := WriteWorkCompletion(colonyRoot, traceID, WorkCompletionCheckpoint{
		CompletionID: completionID,
		WorkRevision: trace.WorkRevision,
		CompletedAt:  completedAt,
		Base:         evidence.Base,
		Revision:     evidence.Revision,
		TaskIDs:      completionTaskIDs(trace),
	}, result, metrics)
	if err != nil {
		return WorkCompletionCheckpoint{}, err
	}

	// The projection is a convenience read; the completion directory above is
	// the immutable evidence a historical reference points at. Use the
	// checkpoint's persisted time in case a concurrent capture won the race.
	if metrics.Work.Completion != nil {
		metrics.Work.Completion.CompletedAt = checkpoint.CompletedAt
	}
	if err := WriteTrailMetrics(colonyRoot, traceID, metrics); err != nil {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: write metrics projection: %w", err)
	}

	// The current work record makes the completion readable offline even when
	// the reactor never gets another event. It never lowers the accepted
	// revision, so a stale capture cannot reopen newer work.
	if err := WriteTrailWorkRecord(colonyRoot, traceID, TrailWorkRecord{
		Revision: trace.WorkRevision,
		Completion: &MetricsCompletion{
			CompletionID: checkpoint.CompletionID,
			WorkRevision: checkpoint.WorkRevision,
			CompletedAt:  checkpoint.CompletedAt,
			ResultRef:    checkpoint.ResultRef,
			MetricsRef:   checkpoint.MetricsRef,
		},
	}); err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	return checkpoint, nil
}

// WorkCompletionCodeEvidence is the captured git evidence for a code result.
// Base and Revision are the diff endpoints; Tree is the result's tree object
// identity. All are empty when no worktree existed at capture time (a non-code
// result or an already-pruned checkout).
type WorkCompletionCodeEvidence struct {
	Base     string
	Revision string
	Tree     string
	Patch    []byte
}

// captureWorktreeEvidence snapshots the trace worktree before delivery can
// remove it. A missing or non-git worktree is not an error: many trails have no
// code result, and a retry after the first capture must not fail because the
// worktree is already gone.
func captureWorktreeEvidence(colonyRoot, traceID string) WorkCompletionCodeEvidence {
	worktreePath := filepath.Join(colonyRoot, ".paseka", "worktrees", traceID)
	if !gitroot.IsInsideWorkTree(worktreePath) {
		return WorkCompletionCodeEvidence{}
	}
	revision, err := gitOutput(worktreePath, "rev-parse", "HEAD")
	if err != nil || revision == "" {
		return WorkCompletionCodeEvidence{}
	}
	evidence := WorkCompletionCodeEvidence{Revision: revision}
	if tree, err := gitOutput(worktreePath, "rev-parse", "HEAD^{tree}"); err == nil {
		evidence.Tree = tree
	}
	if defaultBranch, err := gitroot.ResolvedDefaultBranch(colonyRoot); err == nil && defaultBranch != "" {
		if base, err := gitOutput(worktreePath, "merge-base", "HEAD", defaultBranch); err == nil {
			evidence.Base = base
		}
	}
	if evidence.Base != "" && evidence.Base != evidence.Revision {
		if patch, err := gitOutput(worktreePath, "diff", "--binary", "--no-color", evidence.Base, evidence.Revision); err == nil {
			evidence.Patch = []byte(patch)
		}
	}
	return evidence
}

// completionResultFromSnapshot builds the immutable result evidence for one
// accepted work set. Merge tasks are final acceptance and are excluded.
func completionResultFromSnapshot(colonyRoot, traceID string, trace taskledger.TraceSnapshot, metrics TrailMetrics) WorkCompletionResult {
	ids := completionTaskIDs(trace)
	result := WorkCompletionResult{
		TraceID:      traceID,
		WorkRevision: trace.WorkRevision,
		Tasks:        make([]WorkCompletionTask, 0, len(ids)),
	}
	for _, taskID := range ids {
		task := trace.Tasks[taskID]
		taskResult := WorkCompletionTask{
			TaskID:       task.TaskID,
			Phase:        string(taskledger.TaskPhaseOf(task)),
			Bee:          task.Bee,
			Intent:       task.Intent,
			Status:       string(task.Status),
			Commit:       task.Commit,
			Summary:      task.Summary,
			ProposalType: string(task.ProposalWorkspace),
			Skipped:      task.Status == protocol.TaskStatusSkipped,
		}
		taskDir := TaskDir{ColonyRoot: colonyRoot, TraceID: traceID, TaskID: taskID}
		if entries, err := taskDir.ReadTaskRuns(); err == nil {
			for _, entry := range entries {
				if entry.AgentID != "" {
					taskResult.Runs = append(taskResult.Runs, entry.AgentID)
				}
			}
		}
		result.Tasks = append(result.Tasks, taskResult)
	}
	result.Checks = WorkCompletionChecks{
		Runs:               metrics.Runs.Retained,
		ChecksPassed:       metrics.Runs.ChecksPassed,
		ChecksFailed:       metrics.Runs.ChecksFailed,
		VerificationFailed: metrics.VerificationFailed,
		GateRejections:     metrics.GateRejections,
	}
	return result
}

// completionTaskIDs returns the accepted task identities in stable order, with
// merge tasks and analysis tasks excluded.
func completionTaskIDs(trace taskledger.TraceSnapshot) []string {
	ids := make([]string, 0, len(trace.Tasks))
	for taskID, task := range trace.Tasks {
		if taskledger.IsMergeTask(task) || taskledger.IsAnalysisTask(task) {
			continue
		}
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	return ids
}

// retainedArtifactRefs lists trail comb files as colony-root-relative
// references. They live under .paseka/runs/<traceId>/artifacts, so a delivery
// worktree removal cannot take them with it and analysis can still inspect a
// non-code outcome after the fact.
func retainedArtifactRefs(colonyRoot, traceID string) []string {
	root := filepath.Join(colonyRoot, ".paseka", "runs", traceID, "artifacts")
	var refs []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || strings.HasSuffix(d.Name(), "~") {
			return nil
		}
		rel, err := filepath.Rel(colonyRoot, path)
		if err != nil {
			return nil
		}
		refs = append(refs, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(refs)
	return refs
}

// CompletionEvidencePaths resolves one completion's retained evidence to
// absolute paths for an analysis run. It returns nil when the milestone has no
// readable checkpoint: an analysis may still be launched from its own prompt,
// but it must not be told about evidence that does not exist.
func CompletionEvidencePaths(colonyRoot, traceID, completionID string) []string {
	checkpoint, ok, err := ReadWorkCompletionCheckpoint(colonyRoot, traceID, completionID)
	if err != nil || !ok || checkpoint.CompletionID == "" {
		return nil
	}
	candidates := []string{
		WorkCompletionCheckpointPath(colonyRoot, traceID, completionID),
		WorkCompletionResultPath(colonyRoot, traceID, completionID),
		WorkCompletionMetricsPath(colonyRoot, traceID, completionID),
	}
	if checkpoint.PatchRef != "" {
		candidates = append(candidates, WorkCompletionPatchPath(colonyRoot, traceID, completionID))
	}
	paths := make([]string, 0, len(candidates))
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			paths = append(paths, path)
		}
	}
	return paths
}

// gitOutput runs one read-only git command and returns trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
