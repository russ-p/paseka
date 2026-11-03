package runs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// AnalysisDirName is the reserved trace subdirectory holding one durable record
// per admitted analysis dispatch. It lives with the trail's other retained
// evidence, so a restart can deduplicate a configured analysis reaction without
// relying on the live worktree or an in-memory set.
const AnalysisDirName = "analysis"

// AnalysisAdmissionStatus is the lifecycle of one analysis dispatch.
type AnalysisAdmissionStatus string

const (
	// AnalysisAdmissionRunning means the dispatch was admitted and its outcome
	// is not recorded yet. A restart treats it as taken: an explicitly retried
	// analysis is a new operator action, never an automatic duplicate.
	AnalysisAdmissionRunning AnalysisAdmissionStatus = "running"
	// AnalysisAdmissionCompleted means the analysis run finished successfully.
	AnalysisAdmissionCompleted AnalysisAdmissionStatus = "completed"
	// AnalysisAdmissionFailed means the analysis run failed. The record keeps
	// the failure as analysis evidence and blocks only an automatic duplicate.
	AnalysisAdmissionFailed AnalysisAdmissionStatus = "failed"
)

// AnalysisAdmission is the durable identity of one analysis dispatch. It names
// the studied work-completion milestone and the reaction or action identity, so
// a duplicate delivery or a reactor restart does not start the same analysis
// twice.
type AnalysisAdmission struct {
	AdmissionID  string                    `json:"admissionId"`
	TraceID      string                    `json:"traceId"`
	CompletionID string                    `json:"completionId"`
	Bee          string                    `json:"bee"`
	Purpose      protocol.ExecutionPurpose `json:"purpose"`
	AnalysisOf   string                    `json:"analysisOf"`
	Source       string                    `json:"source,omitempty"`
	Status       AnalysisAdmissionStatus   `json:"status"`
	AgentID      string                    `json:"agentId,omitempty"`
	StartedAt    time.Time                 `json:"startedAt"`
	FinishedAt   *time.Time                `json:"finishedAt,omitempty"`
	Summary      string                    `json:"summary,omitempty"`
}

// AnalysisAdmissionsRoot returns .paseka/runs/<traceId>/analysis/.
func AnalysisAdmissionsRoot(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, AnalysisDirName)
}

// AnalysisAdmissionPath returns the durable record path for one admission.
func AnalysisAdmissionPath(colonyRoot, traceID, admissionID string) string {
	return filepath.Join(AnalysisAdmissionsRoot(colonyRoot, traceID), admissionID+".json")
}

// AnalysisAdmissionID derives the stable identity of one analysis reaction for
// a source milestone and a Bee reaction identity. It is deterministic so the
// same milestone plus the same reaction always resolves to the same record,
// across restarts and duplicate event delivery.
func AnalysisAdmissionID(traceID, completionID, bee string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(traceID),
		strings.TrimSpace(completionID),
		strings.TrimSpace(bee),
	}, "\x00")))
	return "analysis-" + hex.EncodeToString(sum[:8])
}

// WriteAnalysisAdmission atomically records (or updates) one admission.
func WriteAnalysisAdmission(colonyRoot string, admission AnalysisAdmission) error {
	if err := validateTrailRecordIDs(colonyRoot, admission.TraceID, admission.AdmissionID); err != nil {
		return err
	}
	if admission.AnalysisOf == "" {
		admission.AnalysisOf = admission.CompletionID
	}
	if admission.Purpose == "" {
		admission.Purpose = protocol.PurposeAnalysis
	}
	if admission.StartedAt.IsZero() {
		admission.StartedAt = time.Now().UTC()
	}
	data, err := json.MarshalIndent(admission, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(AnalysisAdmissionPath(colonyRoot, admission.TraceID, admission.AdmissionID), append(data, '\n'), 0o644)
}

// ReadAnalysisAdmission loads one admission record. A missing record is not an
// error: ok is false and the caller admits a fresh dispatch.
func ReadAnalysisAdmission(colonyRoot, traceID, admissionID string) (AnalysisAdmission, bool, error) {
	if err := validateTrailRecordIDs(colonyRoot, traceID, admissionID); err != nil {
		return AnalysisAdmission{}, false, err
	}
	data, err := os.ReadFile(AnalysisAdmissionPath(colonyRoot, traceID, admissionID))
	if err != nil {
		if os.IsNotExist(err) {
			return AnalysisAdmission{}, false, nil
		}
		return AnalysisAdmission{}, false, err
	}
	var admission AnalysisAdmission
	if err := json.Unmarshal(data, &admission); err != nil {
		return AnalysisAdmission{}, false, fmt.Errorf("runs: parse analysis admission %q: %w", admissionID, err)
	}
	return admission, true, nil
}

// FinishAnalysisAdmission records the outcome of an admitted analysis run. A
// failed analysis stays an analysis failure; it never rewrites work evidence.
func FinishAnalysisAdmission(colonyRoot string, admission AnalysisAdmission, status AnalysisAdmissionStatus, agentID, summary string, finishedAt time.Time) error {
	admission.Status = status
	if agentID != "" {
		admission.AgentID = agentID
	}
	admission.Summary = summary
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
	}
	finishedAt = finishedAt.UTC()
	admission.FinishedAt = &finishedAt
	return WriteAnalysisAdmission(colonyRoot, admission)
}
