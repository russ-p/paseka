package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
)

// Projected contract verdicts. ContractUnknown is not a protocol contract
// status: it is the reader's word for a record that carries no contract field,
// which covers a bee that declared no contract, an interactive session, and an
// older run. It is never folded into a pass.
const ContractUnknown = "unknown"

// Outcome record integrity markers. Records is nil when the two outcome records
// agree or when the run is still in flight, and carries one of these when a
// reader cannot prove one coherent outcome.
const (
	// RecordConflict means status.json and result.json both carry a terminal
	// state and the states disagree. The projected state is never completed:
	// a stale completed value must not mask a recorded failure.
	RecordConflict = "conflict"
	// RecordIncomplete means one terminal outcome record exists while its
	// partner is missing or still non-terminal. An in-flight run with no
	// terminal result is not incomplete.
	RecordIncomplete = "incomplete"
)

// ContractEvidence is the run projection's read of result.json's contract
// record: the verdict, the effective declared rules, and the observations or
// the evaluation error.
//
// A record without the field projects as ContractUnknown with no rules or
// observations. The reader does not assume a contract passed because it cannot
// see one, and it does not rewrite the record to add the field.
type ContractEvidence struct {
	Status       string                         `json:"status"`
	Rules        []protocol.ContractRule        `json:"rules,omitempty"`
	Observations []protocol.ContractObservation `json:"observations,omitempty"`
	Error        string                         `json:"error,omitempty"`
}

// RecordIntegrity reports a status.json/result.json pair that cannot be read as
// one outcome. Detail names what disagreed or what was missing, so an operator
// sees the interruption rather than a friendlier value.
type RecordIntegrity struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// RunMeta is a read-only projection of one headless adapter run directory.
type RunMeta struct {
	TraceID           string          `json:"traceId"`
	AgentID           string          `json:"agentId"`
	Bee               string          `json:"bee"`
	Adapter           string          `json:"adapter"`
	Implementation    string          `json:"implementation,omitempty"`
	Workspace         string          `json:"workspace"`
	ColonyRoot        string          `json:"colonyRoot"`
	TaskID            string          `json:"taskId,omitempty"`
	Task              string          `json:"task,omitempty"`
	Intent            string          `json:"intent,omitempty"`
	Model             string          `json:"model,omitempty"`
	Profile           string          `json:"profile,omitempty"`
	State             string          `json:"state"`
	Summary           string          `json:"summary,omitempty"`
	Usage             *protocol.Usage `json:"usage,omitempty"`
	ProviderSessionID string          `json:"providerSessionId,omitempty"`
	RunDir            string          `json:"runDir"`
	StartedAt         time.Time       `json:"startedAt"`
	FinishedAt        time.Time       `json:"finishedAt,omitempty"`
	HasEvents         bool            `json:"hasEvents"`
	HasSession        bool            `json:"hasSession"`
	// Gate is the runtime-owned mechanical check verdict, absent when the bee
	// declares no gate or the gate did not run.
	Gate *protocol.GateResult `json:"gate,omitempty"`
	// Contract is the completion-contract evidence, always present as a
	// projection so an absent record reads as unknown rather than as a pass.
	Contract ContractEvidence `json:"contract"`
	// Records marks a conflicting or incomplete outcome-record pair and is
	// absent when the pair agrees or the run is still in flight.
	Records *RecordIntegrity `json:"records,omitempty"`
	// Reason is the recorded classifier for a terminal state, e.g. "timeout"
	// for a cancelled run or "checks_failed" for a gate rejection. A legacy
	// record may carry no reason; absence stays distinguishable from a reason.
	Reason string `json:"reason,omitempty"`
	// Purpose is the admission-time execution purpose. It is empty for legacy
	// records that never recorded one; such a run is unknown, not work.
	Purpose    protocol.ExecutionPurpose `json:"purpose,omitempty"`
	AnalysisOf string                    `json:"analysisOf,omitempty"`
	protocol.Provenance
}

// LoadRunMeta reads a run projection from an existing run directory.
func LoadRunMeta(d Dir) (RunMeta, error) {
	req, err := d.ReadRequest()
	if err != nil {
		return RunMeta{}, err
	}

	meta := RunMeta{
		TraceID:    req.TraceID,
		AgentID:    req.AgentID,
		Bee:        req.Bee,
		Adapter:    req.Adapter,
		Workspace:  req.Workspace,
		ColonyRoot: req.ColonyRoot,
		TaskID:     req.TaskID,
		Task:       req.Task,
		Intent:     req.Intent,
		Model:      req.Model,
		Profile:    req.Profile,
		Purpose:    req.Purpose,
		AnalysisOf: req.AnalysisOf,
		RunDir:     d.Root(),
		StartedAt:  req.CreatedAt,
		Provenance: protocol.Provenance{
			AttemptID:            req.AttemptID,
			ConfigRef:            req.ConfigRef,
			ConfigRevision:       req.ConfigRevision,
			InitialCodeBaseSHA:   req.InitialCodeBaseSHA,
			InitialCodeBaseDirty: req.InitialCodeBaseDirty,
		},
	}

	meta.Contract = ContractEvidence{Status: ContractUnknown}

	var snap protocol.StatusSnapshot
	haveStatus := false
	if got, err := d.ReadStatus(); err == nil {
		haveStatus = true
		snap = got
		if !snap.StartedAt.IsZero() {
			meta.StartedAt = snap.StartedAt
		}
		if !snap.FinishedAt.IsZero() {
			meta.FinishedAt = snap.FinishedAt
		}
		if snap.Reason != "" {
			meta.Reason = snap.Reason
		}
		meta.Gate = snap.Gate
	}

	var res protocol.Result
	haveResult := false
	legacyResultText := false
	if got, err := d.ReadResultJSON(); err == nil {
		haveResult = true
		res = got
		if res.Summary != "" {
			meta.Summary = res.Summary
		}
		if res.Usage != nil {
			meta.Usage = res.Usage
		}
		if res.ProviderSessionID != "" {
			meta.ProviderSessionID = res.ProviderSessionID
		}
		if !res.FinishedAt.IsZero() {
			meta.FinishedAt = res.FinishedAt
		}
		if meta.Reason == "" && res.Reason != "" {
			meta.Reason = res.Reason
		}
		if meta.Gate == nil && res.Gate != nil {
			meta.Gate = res.Gate
		}
		if res.Contract != nil && res.Contract.Status != "" {
			meta.Contract = ContractEvidence{
				Status:       res.Contract.Status,
				Rules:        res.Contract.Rules,
				Observations: res.Contract.Observations,
				Error:        res.Contract.Error,
			}
		}
	} else if summary, err := d.ReadResult(); err == nil {
		// A human-readable result without result.json is the legacy shape and
		// is read exactly as before. It is not an interrupted pair merely for
		// being old.
		legacyResultText = true
		meta.Summary = summary
	}

	meta.HasEvents = fileExists(d.EventsPath())
	meta.HasSession = fileExists(d.SessionPath())

	meta.State, meta.Records = projectRunOutcome(outcomeRecords{
		HaveStatus:       haveStatus,
		Status:           snap.State,
		HaveResult:       haveResult,
		Result:           res.Status,
		LegacyResultText: legacyResultText,
		// An interactive session writes no result.json; it is not an AFK
		// outcome pair, so absence is expected rather than an interruption.
		Session: meta.HasSession,
	})

	// Interactive sessions write no result.json; their provider accounting lands
	// in session.json instead.
	if meta.Usage == nil {
		if sess, err := d.ReadSession(); err == nil && sess.Usage != nil {
			meta.Usage = sess.Usage
		}
	}
	if meta.Purpose == "" {
		if sess, err := d.ReadSession(); err == nil {
			meta.Purpose = sess.Purpose
			if meta.AnalysisOf == "" {
				meta.AnalysisOf = sess.AnalysisOf
			}
		}
	}

	if meta.Model == "" {
		if sess, err := d.ReadSession(); err == nil {
			meta.Model = sess.Model
		}
	}

	if meta.State == "" {
		meta.State = string(protocol.StatusQueued)
	}
	if legacy, err := readLegacyMeta(d); err == nil {
		if meta.StartedAt.IsZero() {
			meta.StartedAt = legacy.StartedAt
		}
		if meta.ProviderSessionID == "" {
			meta.ProviderSessionID = legacy.ProviderSessionID
		}
		if meta.Implementation == "" {
			meta.Implementation = legacy.Implementation
		}
		if meta.Purpose == "" {
			meta.Purpose = legacy.Purpose
			if meta.AnalysisOf == "" {
				meta.AnalysisOf = legacy.AnalysisOf
			}
		}
	}

	return meta, nil
}

// outcomeRecords is the raw material of one run's persisted outcome: the two
// records if they exist, plus the facts that tell an expected absence from an
// interrupted one.
type outcomeRecords struct {
	HaveStatus       bool
	Status           protocol.RunStatus
	HaveResult       bool
	Result           protocol.RunStatus
	LegacyResultText bool
	Session          bool
}

// projectRunOutcome decides the state a reader may report and, when the two
// records cannot be read as one outcome, the explicit marker saying why.
//
// The rules are deliberately fail-closed:
//   - Two terminal records that agree are the outcome.
//   - Two terminal records that disagree are a conflict and never project as
//     completed, even when one of them is completed.
//   - A terminal record whose partner is missing or still non-terminal is
//     incomplete; a completed record projects as failed because the other half
//     could have been the failure.
//   - An in-flight run with no terminal record is not flagged.
//   - An interactive session and a legacy text-only result are readable shapes,
//     not interruptions.
func projectRunOutcome(rec outcomeRecords) (string, *RecordIntegrity) {
	statusTerminal := isTerminalRunStatus(rec.Status)
	resultTerminal := isTerminalRunStatus(rec.Result)

	if rec.HaveStatus && rec.HaveResult {
		switch {
		case statusTerminal && resultTerminal && rec.Status == rec.Result:
			return string(rec.Status), nil
		case statusTerminal && resultTerminal:
			return string(conflictRunState(rec.Status, rec.Result)), &RecordIntegrity{
				Status: RecordConflict,
				Detail: fmt.Sprintf("status.json says %s but result.json says %s", rec.Status, rec.Result),
			}
		case statusTerminal:
			return string(failClosedRunState(rec.Status)), &RecordIntegrity{
				Status: RecordIncomplete,
				Detail: fmt.Sprintf("result.json is %s for a %s run", describeNonTerminal(rec.Result), rec.Status),
			}
		case resultTerminal:
			return string(failClosedRunState(rec.Result)), &RecordIntegrity{
				Status: RecordIncomplete,
				Detail: fmt.Sprintf("status.json is %s for a %s result", describeNonTerminal(rec.Status), rec.Result),
			}
		default:
			// Both records exist and neither is terminal: the run is in flight.
			if rec.Result != "" {
				return string(rec.Result), nil
			}
			return string(rec.Status), nil
		}
	}

	if rec.HaveStatus {
		if !statusTerminal {
			return string(rec.Status), nil
		}
		if rec.Session || rec.LegacyResultText {
			return string(rec.Status), nil
		}
		return string(failClosedRunState(rec.Status)), &RecordIntegrity{
			Status: RecordIncomplete,
			Detail: fmt.Sprintf("result.json is missing for a %s run", rec.Status),
		}
	}

	if rec.HaveResult {
		if !resultTerminal {
			return string(rec.Result), nil
		}
		return string(failClosedRunState(rec.Result)), &RecordIntegrity{
			Status: RecordIncomplete,
			Detail: fmt.Sprintf("status.json is missing for a %s result", rec.Result),
		}
	}

	// No outcome record at all: a legacy run or an untouched run directory.
	// Its current readable shape is preserved.
	return "", nil
}

func isTerminalRunStatus(state protocol.RunStatus) bool {
	switch state {
	case protocol.StatusCompleted, protocol.StatusFailed, protocol.StatusCancelled:
		return true
	}
	return false
}

// conflictRunState picks a state that is never the friendlier completed value.
// A failed record wins, then a cancelled one; two different non-completed
// terminal records still report a conflict.
func conflictRunState(a, b protocol.RunStatus) protocol.RunStatus {
	if a == protocol.StatusFailed || b == protocol.StatusFailed {
		return protocol.StatusFailed
	}
	if a == protocol.StatusCancelled || b == protocol.StatusCancelled {
		return protocol.StatusCancelled
	}
	return protocol.StatusFailed
}

// failClosedRunState refuses to report a completed run whose partner outcome
// record cannot prove completion.
func failClosedRunState(state protocol.RunStatus) protocol.RunStatus {
	if state == protocol.StatusCompleted {
		return protocol.StatusFailed
	}
	return state
}

func describeNonTerminal(state protocol.RunStatus) string {
	if state == "" {
		return "missing"
	}
	return string(state)
}

// FindRun loads one run by trace and agent identifiers.
func FindRun(colonyRoot, traceID, agentID string) (RunMeta, bool, error) {
	if colonyRoot == "" || traceID == "" || agentID == "" {
		return RunMeta{}, false, nil
	}
	d := Dir{ColonyRoot: colonyRoot, TraceID: traceID, AgentID: agentID}
	if !fileExists(d.RequestPath()) {
		return RunMeta{}, false, nil
	}
	meta, err := LoadRunMeta(d)
	if err != nil {
		return RunMeta{}, false, err
	}
	return meta, true, nil
}

// ScanRecentRuns walks .paseka/runs and returns up to limit run directories with
// request.json, newest first by StartedAt.
func ScanRecentRuns(colonyRoot string, limit int) ([]RunMeta, error) {
	if limit <= 0 {
		return nil, nil
	}
	metas, err := ScanRuns(colonyRoot)
	if err != nil {
		return nil, err
	}
	if len(metas) > limit {
		metas = metas[:limit]
	}
	return metas, nil
}

// ScanRuns walks .paseka/runs and returns every run directory carrying a
// request.json, newest first by StartedAt.
//
// The walk is unbounded and the truncation is ScanRecentRuns' job, because a
// caller that needs one run per bee cannot reach it through a recent-page cap:
// a colony with more runs than the cap holds simply never sees its quiet bees.
// The read it costs is the same one ScanRecentRuns already pays.
func ScanRuns(colonyRoot string) ([]RunMeta, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	traceDirs, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []RunMeta
	for _, traceEntry := range traceDirs {
		if !traceEntry.IsDir() {
			continue
		}
		traceID := traceEntry.Name()
		traceMetas, err := listRunsInTraceDir(colonyRoot, traceID, filepath.Join(runsRoot, traceID))
		if err != nil {
			continue
		}
		metas = append(metas, traceMetas...)
	}

	sortRunsByStartedAt(metas)
	return metas, nil
}

// ListRunsForTrace returns every valid run under .paseka/runs/<traceId>/,
// newest first by StartedAt. The tasks projection directory is ignored.
func ListRunsForTrace(colonyRoot, traceID string) ([]RunMeta, error) {
	if colonyRoot == "" || traceID == "" {
		return nil, nil
	}
	tracePath := filepath.Join(colonyRoot, ".paseka", "runs", traceID)
	metas, err := listRunsInTraceDir(colonyRoot, traceID, tracePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sortRunsByStartedAt(metas)
	return metas, nil
}

func listRunsInTraceDir(colonyRoot, traceID, tracePath string) ([]RunMeta, error) {
	agentDirs, err := os.ReadDir(tracePath)
	if err != nil {
		return nil, err
	}
	var metas []RunMeta
	for _, agentEntry := range agentDirs {
		if !agentEntry.IsDir() || IsReservedTraceSubdir(agentEntry.Name()) {
			continue
		}
		d := Dir{
			ColonyRoot: colonyRoot,
			TraceID:    traceID,
			AgentID:    agentEntry.Name(),
		}
		if !fileExists(d.RequestPath()) {
			continue
		}
		meta, err := LoadRunMeta(d)
		if err != nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// ReadEventsAfter returns protocol events with index > after and the next cursor.
//
// An exhausted cursor yields an empty page, never nil: this slice is serialized
// straight into the console's events page, whose `entries` field is an array on
// the wire, and a nil slice marshals to `null` — which the console reads as
// "no array" and trips over.
func (d Dir) ReadEventsAfter(after int) ([]protocol.Event, int, error) {
	events, err := d.readEventsFrom(after)
	if err != nil {
		return nil, after, err
	}
	if events == nil {
		events = []protocol.Event{}
	}
	next := after + len(events)
	return events, next, nil
}

func (d Dir) readEventsFrom(skip int) ([]protocol.Event, error) {
	all, err := d.ReadEvents()
	if err != nil {
		return nil, err
	}
	if skip >= len(all) {
		return nil, nil
	}
	return all[skip:], nil
}

func readLegacyMeta(d Dir) (Meta, error) {
	data, err := os.ReadFile(d.MetaPath())
	if err != nil {
		return Meta{}, err
	}
	var legacy Meta
	if err := json.Unmarshal(data, &legacy); err != nil {
		return Meta{}, err
	}
	return legacy, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sortRunsByStartedAt(metas []RunMeta) {
	for i := 0; i < len(metas); i++ {
		for j := i + 1; j < len(metas); j++ {
			if metas[j].StartedAt.After(metas[i].StartedAt) {
				metas[i], metas[j] = metas[j], metas[i]
			}
		}
	}
}
