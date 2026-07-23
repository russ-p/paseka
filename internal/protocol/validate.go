package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationDetail describes one schema validation failure.
type ValidationDetail struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError is returned when event input fails validation.
type ValidationError struct {
	Code    string
	Details []ValidationDetail
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

// EventInput is the JSON shape agents send to `paseka event emit` / `validate`.
type EventInput struct {
	TraceID string          `json:"traceId"`
	TaskID  string          `json:"taskId,omitempty"`
	AgentID string          `json:"agentId,omitempty"`
	Type    EventType       `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// EventCLIResult is the machine-readable response from emit/validate commands.
type EventCLIResult struct {
	OK           bool               `json:"ok"`
	TraceID      string             `json:"traceId,omitempty"`
	Type         EventType          `json:"type,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Subject      string             `json:"subject,omitempty"`
	EventLogPath string             `json:"eventLogPath,omitempty"`
	Error        string             `json:"error,omitempty"`
	Details      []ValidationDetail `json:"details,omitempty"`
}

// ParseEventInput decodes one event JSON object from raw bytes.
func ParseEventInput(raw []byte) (EventInput, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return EventInput{}, &ValidationError{
			Code: "invalid_json",
			Details: []ValidationDetail{{
				Path:    "",
				Message: "input must be a single JSON object",
			}},
		}
	}
	if !json.Valid([]byte(trimmed)) {
		return EventInput{}, &ValidationError{
			Code: "invalid_json",
			Details: []ValidationDetail{{
				Path:    "",
				Message: "input must be valid JSON",
			}},
		}
	}
	var in EventInput
	if err := json.Unmarshal([]byte(trimmed), &in); err != nil {
		return EventInput{}, &ValidationError{
			Code: "invalid_json",
			Details: []ValidationDetail{{
				Path:    "",
				Message: err.Error(),
			}},
		}
	}
	return in, nil
}

// Validate checks envelope and payload-specific requirements.
func (in EventInput) Validate() []ValidationDetail {
	var details []ValidationDetail

	if strings.TrimSpace(in.TraceID) == "" {
		details = append(details, ValidationDetail{Path: "traceId", Message: "required"})
	}
	if strings.TrimSpace(string(in.Type)) == "" {
		details = append(details, ValidationDetail{Path: "type", Message: "required"})
	} else if !IsDomainEvent(in.Type) {
		details = append(details, ValidationDetail{
			Path:    "type",
			Message: fmt.Sprintf("must be one of SIGNAL, INSIGHT, MUTATION, VERIFICATION (got %q)", in.Type),
		})
	}
	if len(in.Payload) == 0 {
		details = append(details, ValidationDetail{Path: "payload", Message: "required"})
		return details
	}
	if !json.Valid(in.Payload) {
		details = append(details, ValidationDetail{Path: "payload", Message: "must be valid JSON"})
		return details
	}

	kind := PayloadKind(in.Payload)
	if kind == "" {
		details = append(details, ValidationDetail{Path: "payload.kind", Message: "required"})
		return details
	}

	details = append(details, validatePayloadKind(in.Type, kind, in.Payload)...)
	return details
}

func validatePayloadKind(eventType EventType, kind string, payload json.RawMessage) []ValidationDetail {
	if mismatch := expectedEventType(kind); mismatch != "" && mismatch != eventType {
		return []ValidationDetail{{
			Path:    "type",
			Message: fmt.Sprintf("payload.kind %q requires type %s", kind, mismatch),
		}}
	}

	switch TaskEventKind(kind) {
	case TaskEventPlan:
		return validateTaskPlan(payload)
	case TaskEventReady:
		return validateTaskReady(payload)
	case TaskEventStatus:
		return validateTaskStatus(payload)
	case TaskEventCompleted:
		return validateTaskCompleted(payload)
	}

	switch EnergyEventKind(kind) {
	case SignalEnergyAdd:
		return validateEnergyAdd(payload)
	case SignalEnergyConsume:
		return validateEnergyConsume(payload)
	}

	switch VerificationKind(kind) {
	case VerificationSuccess, VerificationFailed:
		return validateVerification(payload)
	}

	switch MutationKind(kind) {
	case MutationCodeProposal, MutationCodeProposalIsolated, MutationCodeProposalRoot:
		return validateCodeProposal(payload)
	}

	switch InsightKind(kind) {
	case InsightRunSummary, InsightReviewNote, InsightContextNote:
		return validateNarrativeInsight(payload)
	case InsightHumanFeedback:
		return validateHumanFeedback(payload)
	case InsightTraceTitle:
		return validateTraceTitle(payload)
	case InsightTraceSummary:
		return validateTraceSummary(payload)
	}

	switch InviteEventKind(kind) {
	case SignalSessionInvite:
		return validateSessionInvite(payload)
	case SignalBeekeeperReady:
		return validateBeekeeperReady(payload)
	}

	return nil
}

func expectedEventType(kind string) EventType {
	switch TaskEventKind(kind) {
	case TaskEventPlan:
		return EventInsight
	case TaskEventReady, TaskEventStatus:
		return EventSignal
	case TaskEventCompleted:
		return EventVerification
	}
	switch EnergyEventKind(kind) {
	case SignalEnergyAdd, SignalEnergyConsume:
		return EventSignal
	}
	switch VerificationKind(kind) {
	case VerificationSuccess, VerificationFailed:
		return EventVerification
	}
	switch MutationKind(kind) {
	case MutationCodeProposal, MutationCodeProposalIsolated, MutationCodeProposalRoot:
		return EventMutation
	}
	if t := InsightKindForEventType(kind); t != "" {
		return t
	}
	switch InviteEventKind(kind) {
	case SignalSessionInvite, SignalBeekeeperReady:
		return EventSignal
	}
	return ""
}

func validateTaskPlan(payload json.RawMessage) []ValidationDetail {
	var p TaskPlanPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid task.plan payload"}}
	}
	if len(p.Tasks) == 0 {
		return []ValidationDetail{{Path: "payload.tasks", Message: "required"}}
	}
	var details []ValidationDetail
	for i, task := range p.Tasks {
		if strings.TrimSpace(task.TaskID) == "" {
			details = append(details, ValidationDetail{
				Path:    fmt.Sprintf("payload.tasks[%d].taskId", i),
				Message: "required",
			})
		}
		if strings.TrimSpace(task.Title) == "" {
			details = append(details, ValidationDetail{
				Path:    fmt.Sprintf("payload.tasks[%d].title", i),
				Message: "required",
			})
		}
	}
	return details
}

func validateTaskReady(payload json.RawMessage) []ValidationDetail {
	var p TaskReadyPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid task.ready payload"}}
	}
	if strings.TrimSpace(p.TaskID) == "" {
		return []ValidationDetail{{Path: "payload.taskId", Message: "required"}}
	}
	return nil
}

func validateTaskStatus(payload json.RawMessage) []ValidationDetail {
	var p TaskStatusPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid task.status payload"}}
	}
	var details []ValidationDetail
	if strings.TrimSpace(p.TaskID) == "" {
		details = append(details, ValidationDetail{Path: "payload.taskId", Message: "required"})
	}
	if strings.TrimSpace(string(p.Status)) == "" {
		details = append(details, ValidationDetail{Path: "payload.status", Message: "required"})
	}
	return details
}

func validateTaskCompleted(payload json.RawMessage) []ValidationDetail {
	var p TaskCompletedPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid task.completed payload"}}
	}
	var details []ValidationDetail
	if strings.TrimSpace(p.TaskID) == "" {
		details = append(details, ValidationDetail{Path: "payload.taskId", Message: "required"})
	}
	if strings.TrimSpace(string(p.Status)) == "" {
		details = append(details, ValidationDetail{Path: "payload.status", Message: "required"})
	}
	return details
}

func validateVerification(payload json.RawMessage) []ValidationDetail {
	var p VerificationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid verification payload"}}
	}
	if strings.TrimSpace(p.Summary) == "" {
		return []ValidationDetail{{Path: "payload.summary", Message: "required"}}
	}
	return nil
}

func validateCodeProposal(payload json.RawMessage) []ValidationDetail {
	var p MutationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid code.proposal payload"}}
	}
	if strings.TrimSpace(p.Diff) == "" && strings.TrimSpace(p.Ref) == "" && strings.TrimSpace(p.Summary) == "" {
		return []ValidationDetail{{Path: "payload", Message: "at least one of diff, ref, or summary is required"}}
	}
	return validateCodeProposalProvenance(p)
}

func validateCodeProposalProvenance(p MutationPayload) []ValidationDetail {
	var details []ValidationDetail
	kind := NormalizeCodeProposalKind(p.Kind)

	if ws := strings.TrimSpace(string(p.Workspace)); ws != "" {
		switch ProposalWorkspace(ws) {
		case ProposalWorkspaceIsolated, ProposalWorkspaceRoot:
			expected := ProposalWorkspaceIsolated
			if kind == MutationCodeProposalRoot {
				expected = ProposalWorkspaceRoot
			}
			if ProposalWorkspace(ws) != expected {
				details = append(details, ValidationDetail{
					Path:    "payload.workspace",
					Message: fmt.Sprintf("must be %q for kind %q", expected, kind),
				})
			}
		default:
			details = append(details, ValidationDetail{
				Path:    "payload.workspace",
				Message: "must be one of isolated, root",
			})
		}
	}

	if baseSha := strings.TrimSpace(p.BaseSha); baseSha != "" && len(baseSha) < 4 {
		details = append(details, ValidationDetail{
			Path:    "payload.baseSha",
			Message: "must be a non-empty git object id",
		})
	}

	if wt := strings.TrimSpace(p.WorktreePath); wt != "" {
		if kind == MutationCodeProposalRoot {
			details = append(details, ValidationDetail{
				Path:    "payload.worktreePath",
				Message: "must not be set for code.proposal.root",
			})
		}
	}

	if sector := strings.TrimSpace(p.Sector); sector != "" && strings.Contains(sector, "\n") {
		details = append(details, ValidationDetail{
			Path:    "payload.sector",
			Message: "must be a single-line sector id",
		})
	}

	return details
}

func validateNarrativeInsight(payload json.RawMessage) []ValidationDetail {
	var p NarrativeInsightPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid narrative insight payload"}}
	}
	if strings.TrimSpace(p.Summary) == "" {
		return []ValidationDetail{{Path: "payload.summary", Message: "required"}}
	}
	return nil
}

func validateHumanFeedback(payload json.RawMessage) []ValidationDetail {
	var p HumanFeedbackPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid human.feedback payload"}}
	}
	var details []ValidationDetail
	if strings.TrimSpace(p.TaskID) == "" {
		details = append(details, ValidationDetail{Path: "payload.taskId", Message: "required"})
	}
	if strings.TrimSpace(p.Message) == "" {
		details = append(details, ValidationDetail{Path: "payload.message", Message: "required"})
	}
	return details
}

func validateTraceTitle(payload json.RawMessage) []ValidationDetail {
	var p TraceTitlePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid trace.title payload"}}
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		return []ValidationDetail{{Path: "payload.title", Message: "required"}}
	}
	if len(title) > MaxTraceTitleLen {
		return []ValidationDetail{{Path: "payload.title", Message: fmt.Sprintf("must be at most %d characters", MaxTraceTitleLen)}}
	}
	return nil
}

func validateTraceSummary(payload json.RawMessage) []ValidationDetail {
	var p TraceSummaryPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid trace.summary payload"}}
	}
	summary := strings.TrimSpace(p.Summary)
	if summary == "" {
		return []ValidationDetail{{Path: "payload.summary", Message: "required"}}
	}
	if len(summary) > MaxTraceSummaryLen {
		return []ValidationDetail{{Path: "payload.summary", Message: fmt.Sprintf("must be at most %d characters", MaxTraceSummaryLen)}}
	}
	return nil
}

func validateEnergyAdd(payload json.RawMessage) []ValidationDetail {
	var p EnergyAddPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid energy.add payload"}}
	}
	if p.Amount <= 0 {
		return []ValidationDetail{{Path: "payload.amount", Message: "must be positive"}}
	}
	return nil
}

func validateEnergyConsume(payload json.RawMessage) []ValidationDetail {
	var p EnergyConsumePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid energy.consume payload"}}
	}
	if p.Amount <= 0 {
		return []ValidationDetail{{Path: "payload.amount", Message: "must be positive"}}
	}
	return nil
}

func validateSessionInvite(payload json.RawMessage) []ValidationDetail {
	var p SessionInvitePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid session.invite payload"}}
	}
	var details []ValidationDetail
	if strings.TrimSpace(p.InviteID) == "" {
		details = append(details, ValidationDetail{Path: "payload.inviteId", Message: "required"})
	}
	if strings.TrimSpace(p.Bee) == "" {
		details = append(details, ValidationDetail{Path: "payload.bee", Message: "required"})
	}
	if strings.TrimSpace(p.Task) == "" {
		details = append(details, ValidationDetail{Path: "payload.task", Message: "required"})
	}
	if strings.TrimSpace(string(p.Status)) == "" {
		details = append(details, ValidationDetail{Path: "payload.status", Message: "required"})
	} else if !isInviteStatus(p.Status) {
		details = append(details, ValidationDetail{
			Path:    "payload.status",
			Message: "must be one of pending, accepted, cancelled, completed",
		})
	}
	return details
}

func validateBeekeeperReady(payload json.RawMessage) []ValidationDetail {
	var p BeekeeperReadyPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return []ValidationDetail{{Path: "payload", Message: "invalid beekeeper.ready payload"}}
	}
	var details []ValidationDetail
	if strings.TrimSpace(p.InviteID) == "" {
		details = append(details, ValidationDetail{Path: "payload.inviteId", Message: "required"})
	}
	if strings.TrimSpace(string(p.Action)) == "" {
		details = append(details, ValidationDetail{Path: "payload.action", Message: "required"})
	} else if !isBeekeeperAction(p.Action) {
		details = append(details, ValidationDetail{
			Path:    "payload.action",
			Message: "must be one of accept, reject, defer",
		})
	}
	return details
}

func isInviteStatus(status InviteStatus) bool {
	switch status {
	case InviteStatusPending, InviteStatusAccepted, InviteStatusCancelled, InviteStatusCompleted, InviteStatusIncomplete:
		return true
	default:
		return false
	}
}

func isBeekeeperAction(action BeekeeperAction) bool {
	switch action {
	case BeekeeperAccept, BeekeeperReject, BeekeeperDefer:
		return true
	default:
		return false
	}
}

// ToEvent validates input and builds a canonical protocol.Event.
func (in EventInput) ToEvent(defaultAgentID string) (Event, error) {
	if details := in.Validate(); len(details) > 0 {
		return Event{}, &ValidationError{Code: "schema_validation_failed", Details: details}
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = defaultAgentID
	}
	var payload any
	if err := json.Unmarshal(in.Payload, &payload); err != nil {
		return Event{}, fmt.Errorf("protocol: unmarshal payload: %w", err)
	}
	return NewEvent(in.TraceID, agentID, 0, in.Type, payload)
}
