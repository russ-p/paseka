package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TrailWorkFileName is the durable trail-owned work record: the accepted work
// revision and the trail's current completion, if any. It lives at the trace
// root beside the trail event log so an offline metrics read does not need the
// task ledger, and worktree-only cleanup cannot remove it.
const TrailWorkFileName = "work.json"

// WorkCompletionDirName holds one immutable directory per completion episode.
// The directory is never rewritten: a historical completion keeps the exact
// checkpoint, result evidence, and metrics snapshot it announced.
const WorkCompletionDirName = "completions"

const (
	workCompletionCheckpointFile = "completion.json"
	workCompletionResultFile     = "result.json"
	workCompletionMetricsFile    = TrailMetricsFileName
	trailWorkVersion             = 1
	workCompletionVersion        = 1
)

// TrailWorkRecord is the retained work state a reader projects into metrics.
// Revision is zero when no work has been accepted yet; absent record means the
// trail predates revision evidence and stays unknown.
type TrailWorkRecord struct {
	Version    int                `json:"version"`
	TraceID    string             `json:"traceId"`
	Revision   int                `json:"revision"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	Completion *MetricsCompletion `json:"completion,omitempty"`
}

// TrailWorkPath returns .paseka/runs/<traceId>/work.json.
func TrailWorkPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, TrailWorkFileName)
}

// WriteTrailWorkRecord atomically retains the accepted revision and current
// completion. It never lowers the recorded revision: a late writer with an
// older snapshot cannot reopen accepted work in the retained record.
func WriteTrailWorkRecord(colonyRoot, traceID string, record TrailWorkRecord) error {
	if strings.TrimSpace(colonyRoot) == "" {
		return fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return err
	}
	record.TraceID = traceID
	if record.Version == 0 {
		record.Version = trailWorkVersion
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = time.Now().UTC()
	}
	if existing, ok, err := ReadTrailWorkRecord(colonyRoot, traceID); err != nil {
		return err
	} else if ok {
		if existing.Revision > record.Revision {
			return nil
		}
		if existing.Revision == record.Revision && record.Completion == nil {
			record.Completion = existing.Completion
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(TrailWorkPath(colonyRoot, traceID)), 0o755); err != nil {
		return fmt.Errorf("runs: mkdir trail work dir: %w", err)
	}
	return writeFileAtomic(TrailWorkPath(colonyRoot, traceID), append(data, '\n'), 0o644)
}

// ReadTrailWorkRecord loads the retained work record. A missing record is not
// an error: the caller reports an unknown revision rather than inventing one.
func ReadTrailWorkRecord(colonyRoot, traceID string) (TrailWorkRecord, bool, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return TrailWorkRecord{}, false, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return TrailWorkRecord{}, false, err
	}
	data, err := os.ReadFile(TrailWorkPath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return TrailWorkRecord{}, false, nil
		}
		return TrailWorkRecord{}, false, err
	}
	var record TrailWorkRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return TrailWorkRecord{}, false, fmt.Errorf("runs: parse trail work %q: %w", traceID, err)
	}
	if record.TraceID == "" {
		record.TraceID = traceID
	}
	return record, true, nil
}

// WorkCompletionDir returns .paseka/runs/<traceId>/completions/<completionId>.
func WorkCompletionDir(colonyRoot, traceID, completionID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, WorkCompletionDirName, completionID)
}

// WorkCompletionCheckpointPath returns the immutable checkpoint path.
func WorkCompletionCheckpointPath(colonyRoot, traceID, completionID string) string {
	return filepath.Join(WorkCompletionDir(colonyRoot, traceID, completionID), workCompletionCheckpointFile)
}

// WorkCompletionMetricsPath returns the immutable metrics snapshot path.
func WorkCompletionMetricsPath(colonyRoot, traceID, completionID string) string {
	return filepath.Join(WorkCompletionDir(colonyRoot, traceID, completionID), workCompletionMetricsFile)
}

// WorkCompletionResultPath returns the immutable result evidence path.
func WorkCompletionResultPath(colonyRoot, traceID, completionID string) string {
	return filepath.Join(WorkCompletionDir(colonyRoot, traceID, completionID), workCompletionResultFile)
}

// WorkCompletionResultRef returns the colony-root-relative result reference
// for one completion.
func WorkCompletionResultRef(traceID, completionID string) string {
	return filepath.ToSlash(filepath.Join(".paseka", "runs", traceID, WorkCompletionDirName, completionID, workCompletionResultFile))
}

// WorkCompletionMetricsRef returns the colony-root-relative immutable metrics
// snapshot reference for one completion. It is deliberately not the replaceable
// trail-level metrics.json.
func WorkCompletionMetricsRef(traceID, completionID string) string {
	return filepath.ToSlash(filepath.Join(".paseka", "runs", traceID, WorkCompletionDirName, completionID, workCompletionMetricsFile))
}

// WorkCompletionCheckpoint is the immutable envelope of one completion
// episode. Its references are colony-root-relative and resolve to the
// completion's own snapshot and result evidence, never to the replaceable
// metrics.json convenience file.
type WorkCompletionCheckpoint struct {
	Version      int       `json:"version"`
	TraceID      string    `json:"traceId"`
	CompletionID string    `json:"completionId"`
	WorkRevision int       `json:"workRevision"`
	CompletedAt  time.Time `json:"completedAt"`
	ResultRef    string    `json:"resultRef"`
	MetricsRef   string    `json:"metricsRef"`
	TaskIDs      []string  `json:"taskIds"`
}

// WorkCompletionTask is one accepted task's retained result evidence. It is a
// snapshot, not a live projection: later rework cannot rewrite it.
type WorkCompletionTask struct {
	TaskID       string   `json:"taskId"`
	Phase        string   `json:"phase"`
	Bee          string   `json:"bee,omitempty"`
	Intent       string   `json:"intent,omitempty"`
	Status       string   `json:"status"`
	Commit       string   `json:"commit,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	ProposalType string   `json:"proposalWorkspace,omitempty"`
	Runs         []string `json:"runs,omitempty"`
	Skipped      bool     `json:"skipped,omitempty"`
}

// WorkCompletionChecks records the mandatory-check evidence for the trail at
// completion time. A nil result means no retained run recorded a gate verdict;
// it is not the same as a passing gate.
type WorkCompletionChecks struct {
	Runs               int `json:"runs"`
	ChecksPassed       int `json:"checksPassed"`
	ChecksFailed       int `json:"checksFailed"`
	VerificationFailed int `json:"verificationFailed"`
	GateRejections     int `json:"gateRejections"`
}

// WorkCompletionResult is the immutable result evidence captured for one
// completion: the accepted task set with its commits and any skipped tasks plus
// the mandatory-check totals. For a code result it retains the final patch
// identity; for a non-code result it records that the work itself completed
// without a proposal.
type WorkCompletionResult struct {
	Version      int                  `json:"version"`
	TraceID      string               `json:"traceId"`
	CompletionID string               `json:"completionId"`
	WorkRevision int                  `json:"workRevision"`
	CompletedAt  time.Time            `json:"completedAt"`
	Tasks        []WorkCompletionTask `json:"tasks"`
	Checks       WorkCompletionChecks `json:"checks"`
}

// WriteWorkCompletion writes one completion's checkpoint, result evidence, and
// immutable metrics snapshot. Files already present for the recorded identity
// are left untouched, so a retry after a partial crash fills only what is
// missing and a retry after publication cannot rewrite history.
func WriteWorkCompletion(colonyRoot, traceID string, checkpoint WorkCompletionCheckpoint, result WorkCompletionResult, metrics TrailMetrics) (WorkCompletionCheckpoint, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	checkpoint.TraceID = traceID
	if err := validateTrailRecordID("completionId", checkpoint.CompletionID); err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	if checkpoint.WorkRevision < 1 {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: completion %q requires a work revision", checkpoint.CompletionID)
	}
	if checkpoint.CompletedAt.IsZero() {
		checkpoint.CompletedAt = time.Now().UTC()
	}
	if checkpoint.Version == 0 {
		checkpoint.Version = workCompletionVersion
	}

	if existing, ok, err := ReadWorkCompletionCheckpoint(colonyRoot, traceID, checkpoint.CompletionID); err != nil {
		return WorkCompletionCheckpoint{}, err
	} else if ok {
		if existing.WorkRevision != checkpoint.WorkRevision {
			return WorkCompletionCheckpoint{}, fmt.Errorf("runs: completion %q already records revision %d, not %d", checkpoint.CompletionID, existing.WorkRevision, checkpoint.WorkRevision)
		}
		// Keep the first capture's refs and time: a retry must not rewrite the
		// evidence the already-announced completion pointed at.
		checkpoint.ResultRef = existing.ResultRef
		checkpoint.MetricsRef = existing.MetricsRef
		checkpoint.CompletedAt = existing.CompletedAt
		checkpoint.TaskIDs = existing.TaskIDs
	} else {
		checkpoint.ResultRef = WorkCompletionResultRef(traceID, checkpoint.CompletionID)
		checkpoint.MetricsRef = WorkCompletionMetricsRef(traceID, checkpoint.CompletionID)
	}

	dir := WorkCompletionDir(colonyRoot, traceID, checkpoint.CompletionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return WorkCompletionCheckpoint{}, fmt.Errorf("runs: mkdir completion dir: %w", err)
	}

	if err := writeImmutableJSON(WorkCompletionCheckpointPath(colonyRoot, traceID, checkpoint.CompletionID), checkpoint); err != nil {
		return WorkCompletionCheckpoint{}, err
	}

	result.TraceID = traceID
	result.CompletionID = checkpoint.CompletionID
	result.WorkRevision = checkpoint.WorkRevision
	result.CompletedAt = checkpoint.CompletedAt
	result.Version = workCompletionVersion
	if result.Tasks == nil {
		result.Tasks = []WorkCompletionTask{}
	}
	if err := writeImmutableJSON(WorkCompletionResultPath(colonyRoot, traceID, checkpoint.CompletionID), result); err != nil {
		return WorkCompletionCheckpoint{}, err
	}

	metrics.SchemaVersion = TrailMetricsSchemaVersion
	metrics.TraceID = traceID
	if metrics.ObservedAt.IsZero() {
		metrics.ObservedAt = checkpoint.CompletedAt
	}
	if err := writeImmutableJSON(WorkCompletionMetricsPath(colonyRoot, traceID, checkpoint.CompletionID), metrics); err != nil {
		return WorkCompletionCheckpoint{}, err
	}
	return checkpoint, nil
}

// ReadWorkCompletionCheckpoint loads one completion's checkpoint. A missing
// record is not an error: the caller decides whether the absence is expected.
func ReadWorkCompletionCheckpoint(colonyRoot, traceID, completionID string) (WorkCompletionCheckpoint, bool, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return WorkCompletionCheckpoint{}, false, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return WorkCompletionCheckpoint{}, false, err
	}
	if err := validateTrailRecordID("completionId", completionID); err != nil {
		return WorkCompletionCheckpoint{}, false, err
	}
	data, err := os.ReadFile(WorkCompletionCheckpointPath(colonyRoot, traceID, completionID))
	if err != nil {
		if os.IsNotExist(err) {
			return WorkCompletionCheckpoint{}, false, nil
		}
		return WorkCompletionCheckpoint{}, false, err
	}
	var checkpoint WorkCompletionCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return WorkCompletionCheckpoint{}, false, fmt.Errorf("runs: parse completion %q: %w", completionID, err)
	}
	if checkpoint.TraceID == "" {
		checkpoint.TraceID = traceID
	}
	return checkpoint, true, nil
}

// ReadWorkCompletionMetrics loads one completion's immutable metrics snapshot.
func ReadWorkCompletionMetrics(colonyRoot, traceID, completionID string) (TrailMetrics, bool, error) {
	if err := validateTrailRecordID("completionId", completionID); err != nil {
		return TrailMetrics{}, false, err
	}
	data, err := os.ReadFile(WorkCompletionMetricsPath(colonyRoot, traceID, completionID))
	if err != nil {
		if os.IsNotExist(err) {
			return TrailMetrics{}, false, nil
		}
		return TrailMetrics{}, false, err
	}
	var metrics TrailMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return TrailMetrics{}, false, fmt.Errorf("runs: parse completion metrics %q: %w", completionID, err)
	}
	return metrics, true, nil
}

// ResolveTrailRef resolves a colony-root-relative reference recorded by a
// completion. It refuses a path that escapes the colony root.
func ResolveTrailRef(colonyRoot, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("runs: empty trail reference")
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("runs: trail reference %q must be colony-root relative", ref)
	}
	root, err := filepath.Abs(colonyRoot)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, filepath.FromSlash(ref))
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("runs: trail reference %q escapes the colony root", ref)
	}
	return path, nil
}

func writeImmutableJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		// The completion directory is immutable. A retry may fill a file a
		// crash left absent, but it must never rewrite evidence that exists.
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'), 0o644)
}
