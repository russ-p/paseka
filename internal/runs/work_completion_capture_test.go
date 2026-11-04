package runs

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/taskledger"
)

func TestCaptureWorkCompletionRetainsCodeEvidence(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-capture-code"
	initCaptureRepo(t, root)

	worktreePath := filepath.Join(root, ".paseka", "worktrees", traceID)
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		t.Fatal(err)
	}
	captureGit(t, root, "worktree", "add", "-b", "paseka/"+traceID, worktreePath)
	if err := os.WriteFile(filepath.Join(worktreePath, "feature.txt"), []byte("new feature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	captureGit(t, worktreePath, "add", "feature.txt")
	captureGit(t, worktreePath, "commit", "-m", "feat")
	revision := captureGitOutput(t, worktreePath, "rev-parse", "HEAD")

	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID:      traceID,
		WorkRevision: 1,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {
				TaskID: "task-1", Bee: "builder", Phase: protocol.TaskPhaseWork,
				Status: protocol.TaskStatusCompleted, Commit: revision,
				CreatedAt: at, UpdatedAt: at,
			},
		},
	}
	if err := WriteTaskSnapshot(root, traceID, trace.Tasks["task-1"]); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", "findings.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("finding\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := CaptureWorkCompletion(root, trace, at)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.CompletionID != taskledger.CompletionIDForRevision(traceID, 1) {
		t.Fatalf("completion id = %q", checkpoint.CompletionID)
	}
	if checkpoint.PatchRef == "" || checkpoint.Base == "" || checkpoint.Revision != revision {
		t.Fatalf("checkpoint endpoints = %+v", checkpoint)
	}
	if checkpoint.Base == checkpoint.Revision {
		t.Fatal("the captured diff endpoints are identical")
	}

	patchPath, err := ResolveTrailRef(root, checkpoint.PatchRef)
	if err != nil {
		t.Fatal(err)
	}
	patch, err := os.ReadFile(patchPath)
	if err != nil {
		t.Fatalf("patch evidence: %v", err)
	}
	if !strings.Contains(string(patch), "feature.txt") || !strings.Contains(string(patch), "+new feature") {
		t.Fatalf("patch does not contain the final change:\n%s", patch)
	}

	result := readCompletionResult(t, root, traceID, checkpoint.CompletionID)
	if result.Revision != revision || result.Base != checkpoint.Base || result.Tree == "" {
		t.Fatalf("result evidence = %+v", result)
	}
	if result.PatchRef != checkpoint.PatchRef {
		t.Fatalf("result patchRef = %q, checkpoint = %q", result.PatchRef, checkpoint.PatchRef)
	}
	if len(result.Artifacts) != 1 || !strings.HasSuffix(result.Artifacts[0], "artifacts/findings.md") {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}

	record, ok, err := ReadTrailWorkRecord(root, traceID)
	if err != nil || !ok {
		t.Fatalf("work record: ok=%v err=%v", ok, err)
	}
	if record.Revision != 1 || record.Completion == nil || record.Completion.CompletionID != checkpoint.CompletionID {
		t.Fatalf("work record = %+v", record)
	}
	snapshot, ok, err := ReadWorkCompletionMetrics(root, traceID, checkpoint.CompletionID)
	if err != nil || !ok {
		t.Fatalf("metrics snapshot: ok=%v err=%v", ok, err)
	}
	if snapshot.Work.Completion == nil || snapshot.Work.Completion.CompletionID != checkpoint.CompletionID {
		t.Fatalf("snapshot completion = %+v", snapshot.Work.Completion)
	}

	// Removing the source worktree does not take the retained evidence with it.
	if err := os.RemoveAll(worktreePath); err != nil {
		t.Fatal(err)
	}
	paths := CompletionEvidencePaths(root, traceID, checkpoint.CompletionID)
	if len(paths) != 4 {
		t.Fatalf("evidence paths = %v", paths)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("evidence %q is not readable after worktree removal: %v", path, err)
		}
	}
}

func TestCaptureWorkCompletionRetainsNonCodeArtifacts(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-capture-no-code"
	at := time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID:      traceID,
		WorkRevision: 1,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Bee: "builder", Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at},
		},
	}
	if err := WriteTaskSnapshot(root, traceID, trace.Tasks["task-1"]); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(root, ".paseka", "runs", traceID, "artifacts", "report.md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, []byte("report\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := CaptureWorkCompletion(root, trace, at)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.PatchRef != "" || checkpoint.Base != "" || checkpoint.Revision != "" {
		t.Fatalf("a non-code result captured code endpoints: %+v", checkpoint)
	}
	result := readCompletionResult(t, root, traceID, checkpoint.CompletionID)
	if len(result.Artifacts) != 1 || !strings.HasSuffix(result.Artifacts[0], "artifacts/report.md") {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if _, err := os.Stat(WorkCompletionPatchPath(root, traceID, checkpoint.CompletionID)); !os.IsNotExist(err) {
		t.Fatalf("a non-code completion wrote a patch file: %v", err)
	}
}

func TestCaptureWorkCompletionIsImmutableAcrossRetries(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-capture-retry"
	at := time.Date(2026, 11, 4, 14, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID:      traceID,
		WorkRevision: 1,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at},
		},
	}
	if err := WriteTaskSnapshot(root, traceID, trace.Tasks["task-1"]); err != nil {
		t.Fatal(err)
	}

	first, err := CaptureWorkCompletion(root, trace, at)
	if err != nil {
		t.Fatal(err)
	}
	firstMetrics, err := os.ReadFile(WorkCompletionMetricsPath(root, traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	firstResult, err := os.ReadFile(WorkCompletionResultPath(root, traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}

	second, err := CaptureWorkCompletion(root, trace, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if second.CompletionID != first.CompletionID ||
		second.ResultRef != first.ResultRef ||
		second.MetricsRef != first.MetricsRef ||
		!second.CompletedAt.Equal(first.CompletedAt) {
		t.Fatalf("retry rewrote the completion identity: first=%+v second=%+v", first, second)
	}
	secondMetrics, err := os.ReadFile(WorkCompletionMetricsPath(root, traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := os.ReadFile(WorkCompletionResultPath(root, traceID, first.CompletionID))
	if err != nil {
		t.Fatal(err)
	}
	if string(firstMetrics) != string(secondMetrics) {
		t.Fatal("a retry rewrote the immutable metrics snapshot")
	}
	if string(firstResult) != string(secondResult) {
		t.Fatal("a retry rewrote the immutable result evidence")
	}
}

func TestCaptureWorkCompletionConcurrentCapturesConverge(t *testing.T) {
	root := t.TempDir()
	traceID := "trace-capture-concurrent"
	at := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	trace := taskledger.TraceSnapshot{
		TraceID:      traceID,
		WorkRevision: 1,
		Tasks: map[string]taskledger.TaskSnapshot{
			"task-1": {TaskID: "task-1", Status: protocol.TaskStatusCompleted, CreatedAt: at, UpdatedAt: at},
		},
	}
	if err := WriteTaskSnapshot(root, traceID, trace.Tasks["task-1"]); err != nil {
		t.Fatal(err)
	}

	const captures = 6
	checkpoints := make([]WorkCompletionCheckpoint, captures)
	errs := make([]error, captures)
	var wg sync.WaitGroup
	for i := 0; i < captures; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkpoints[i], errs[i] = CaptureWorkCompletion(root, trace, at.Add(time.Duration(i)*time.Minute))
		}()
	}
	wg.Wait()

	for i := range checkpoints {
		if errs[i] != nil {
			t.Fatalf("capture %d: %v", i, errs[i])
		}
		if checkpoints[i].CompletionID != checkpoints[0].CompletionID ||
			checkpoints[i].ResultRef != checkpoints[0].ResultRef ||
			checkpoints[i].MetricsRef != checkpoints[0].MetricsRef ||
			!checkpoints[i].CompletedAt.Equal(checkpoints[0].CompletedAt) {
			t.Fatalf("capture %d diverged: %+v vs %+v", i, checkpoints[i], checkpoints[0])
		}
	}
	record, ok, err := ReadTrailWorkRecord(root, traceID)
	if err != nil || !ok {
		t.Fatalf("work record: ok=%v err=%v", ok, err)
	}
	if record.Completion == nil || record.Completion.CompletionID != checkpoints[0].CompletionID {
		t.Fatalf("work record completion = %+v", record.Completion)
	}
}

func readCompletionResult(t *testing.T, root, traceID, completionID string) WorkCompletionResult {
	t.Helper()
	data, err := os.ReadFile(WorkCompletionResultPath(root, traceID, completionID))
	if err != nil {
		t.Fatal(err)
	}
	var result WorkCompletionResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func initCaptureRepo(t *testing.T, root string) {
	t.Helper()
	captureGit(t, root, "init")
	captureGit(t, root, "config", "user.email", "test@test.com")
	captureGit(t, root, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	captureGit(t, root, "add", "README.md")
	captureGit(t, root, "commit", "-m", "init")
}

func captureGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func captureGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(string(out))
}
