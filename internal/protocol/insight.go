package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InsightKind identifies INSIGHT payload variants.
type InsightKind string

const (
	InsightRunSummary     InsightKind = "run.summary"
	InsightReviewNote     InsightKind = "review.note"
	InsightContextNote    InsightKind = "context.note"
	InsightHumanFeedback  InsightKind = "human.feedback"
	InsightTraceTitle     InsightKind = "trace.title"
	InsightTraceSummary   InsightKind = "trace.summary"
	InsightWorktreeBranch InsightKind = "worktree.branch"
	InsightPRBody         InsightKind = "pr.body"
)

// Review note severities form a closed vocabulary so a human threshold such as
// "medium or worse" means the same thing whichever bee wrote the note.
const (
	ReviewSeverityInfo     = "info"
	ReviewSeverityLow      = "low"
	ReviewSeverityMedium   = "medium"
	ReviewSeverityHigh     = "high"
	ReviewSeverityCritical = "critical"
)

// ReviewSeverityUnspecified classifies a review note with no severity: absent
// or whitespace-only. It is not a stored value.
const ReviewSeverityUnspecified = "unspecified"

// ReviewSeverityUnknown classifies a historical non-empty severity outside the
// closed vocabulary. Readers report it; they never rewrite the payload.
const ReviewSeverityUnknown = "unknown"

// ReviewSeverityAllowedValues is the human-readable vocabulary used in
// validation details and prompt guidance.
const ReviewSeverityAllowedValues = "info, low, medium, high, critical"

// ReviewSeverityValues returns the closed vocabulary in ascending order.
func ReviewSeverityValues() []string {
	return []string{ReviewSeverityInfo, ReviewSeverityLow, ReviewSeverityMedium, ReviewSeverityHigh, ReviewSeverityCritical}
}

// Feedback verdicts form a closed vocabulary so a reader can tell a rejection
// from observational feedback without inferring intent from prose.
const (
	FeedbackVerdictPositive = "positive"
	FeedbackVerdictNegative = "negative"
	FeedbackVerdictNeutral  = "neutral"
)

// FeedbackVerdictUnspecified classifies feedback with no verdict: absent or
// whitespace-only. It is not a stored value.
const FeedbackVerdictUnspecified = "unspecified"

// FeedbackVerdictUnknown classifies a historical non-empty verdict outside the
// closed vocabulary. Readers report it; they never rewrite the payload.
const FeedbackVerdictUnknown = "unknown"

// FeedbackVerdictAllowedValues is the human-readable vocabulary used in
// validation details and prompt guidance.
const FeedbackVerdictAllowedValues = "positive, negative, neutral"

// FeedbackVerdictValues returns the closed vocabulary.
func FeedbackVerdictValues() []string {
	return []string{FeedbackVerdictPositive, FeedbackVerdictNegative, FeedbackVerdictNeutral}
}

// MaxTraceTitleLen is the maximum length of payload.title on INSIGHT/trace.title.
const MaxTraceTitleLen = 120

// MaxWorktreeBranchLen is the maximum length of payload.branch on INSIGHT/worktree.branch.
const MaxWorktreeBranchLen = 120

// MaxTraceSummaryLen is the maximum length of payload.summary on INSIGHT/trace.summary.
const MaxTraceSummaryLen = 800

// MaxPRBodyLen is the maximum length of payload.body on INSIGHT/pr.body.
const MaxPRBodyLen = 8000

// NarrativeInsightPayload is the shared shape for narrative INSIGHT events.
type NarrativeInsightPayload struct {
	Kind     InsightKind `json:"kind"`
	Summary  string      `json:"summary,omitempty"`
	TaskID   string      `json:"taskId,omitempty"`
	Severity string      `json:"severity,omitempty"`
}

// HumanFeedbackPayload is emitted as INSIGHT with payload.kind=human.feedback.
// Verdict is optional; absence keeps the legacy verdict-less behaviour.
type HumanFeedbackPayload struct {
	Kind    InsightKind `json:"kind"`
	TaskID  string      `json:"taskId"`
	Message string      `json:"message"`
	Ref     string      `json:"ref,omitempty"`
	Verdict string      `json:"verdict,omitempty"`
}

// TraceTitlePayload is emitted as INSIGHT with payload.kind=trace.title.
type TraceTitlePayload struct {
	Kind  InsightKind `json:"kind"`
	Title string      `json:"title"`
}

// TraceSummaryPayload is emitted as INSIGHT with payload.kind=trace.summary.
type TraceSummaryPayload struct {
	Kind    InsightKind `json:"kind"`
	Summary string      `json:"summary"`
}

// WorktreeBranchPayload is emitted as INSIGHT with payload.kind=worktree.branch.
type WorktreeBranchPayload struct {
	Kind   InsightKind `json:"kind"`
	Branch string      `json:"branch"`
}

// PRBodyPayload is emitted as INSIGHT with payload.kind=pr.body.
type PRBodyPayload struct {
	Kind InsightKind `json:"kind"`
	Body string      `json:"body"`
}

// NormalizeReviewSeverity trims and lowercases raw and reports whether the
// result belongs to the closed review.note vocabulary. An absent or
// whitespace-only value returns ("", false): it is valid but says nothing.
func NormalizeReviewSeverity(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case ReviewSeverityInfo, ReviewSeverityLow, ReviewSeverityMedium, ReviewSeverityHigh, ReviewSeverityCritical:
		return value, true
	default:
		return value, false
	}
}

// ClassifyReviewSeverity maps a stored severity onto the closed vocabulary. A
// value outside the vocabulary is reported as unknown; it is never rewritten,
// so a historical payload keeps whatever it recorded.
func ClassifyReviewSeverity(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ReviewSeverityUnspecified
	}
	if value, ok := NormalizeReviewSeverity(raw); ok {
		return value
	}
	return ReviewSeverityUnknown
}

// NormalizeReviewNotePayload canonicalizes a review.note payload at
// construction time, so the persisted and published event carries the trimmed,
// lowercased severity rather than the prompt's spelling. Other insight kinds
// keep their payload untouched: only review.note has a closed vocabulary.
//
// payload is the generic JSON object produced by decoding an event input.
func NormalizeReviewNotePayload(payload any) any {
	obj, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	raw, ok := obj["severity"]
	if !ok {
		return payload
	}
	text, ok := raw.(string)
	if !ok {
		return payload
	}
	value, valid := NormalizeReviewSeverity(text)
	if valid {
		obj["severity"] = value
		return payload
	}
	if value == "" {
		// A whitespace-only severity is valid and means the same as absent.
		delete(obj, "severity")
	}
	// A historical unknown value stays as written for readers to classify.
	return payload
}

// NormalizeFeedbackVerdict trims and lowercases raw and reports whether the
// result belongs to the closed human.feedback verdict vocabulary. An absent or
// whitespace-only value returns ("", false): it is valid but says nothing.
func NormalizeFeedbackVerdict(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case FeedbackVerdictPositive, FeedbackVerdictNegative, FeedbackVerdictNeutral:
		return value, true
	default:
		return value, false
	}
}

// ClassifyFeedbackVerdict maps a stored verdict onto the closed vocabulary. A
// value outside the vocabulary is reported as unknown; it is never rewritten,
// so a historical payload keeps whatever it recorded.
func ClassifyFeedbackVerdict(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return FeedbackVerdictUnspecified
	}
	if value, ok := NormalizeFeedbackVerdict(raw); ok {
		return value
	}
	return FeedbackVerdictUnknown
}

// NormalizeHumanFeedbackPayload canonicalizes a human.feedback payload at
// construction time, so the persisted and published event carries the trimmed,
// lowercased verdict rather than the writer's spelling. Other insight kinds
// keep their payload untouched: only human.feedback has a verdict vocabulary.
//
// payload is the generic JSON object produced by decoding an event input.
func NormalizeHumanFeedbackPayload(payload any) any {
	obj, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	raw, ok := obj["verdict"]
	if !ok {
		return payload
	}
	text, ok := raw.(string)
	if !ok {
		return payload
	}
	value, valid := NormalizeFeedbackVerdict(text)
	if valid {
		obj["verdict"] = value
		return payload
	}
	if value == "" {
		// A whitespace-only verdict is valid and means the same as absent.
		delete(obj, "verdict")
	}
	// A historical unknown value stays as written for readers to classify.
	return payload
}

// IsPromptMemoryInsightKind reports whether an INSIGHT kind should be projected into prompt memory.
func IsPromptMemoryInsightKind(kind string) bool {
	switch InsightKind(strings.TrimSpace(kind)) {
	case InsightRunSummary, InsightReviewNote, InsightContextNote, InsightHumanFeedback:
		return true
	default:
		return false
	}
}

// InsightProjectionOptions controls how INSIGHT events are projected into prompt strings.
type InsightProjectionOptions struct {
	TaskID      string
	MaxInsights int
	MaxChars    int
}

// DefaultInsightProjectionOptions returns sensible defaults for prompt memory.
func DefaultInsightProjectionOptions(taskID string) InsightProjectionOptions {
	return InsightProjectionOptions{
		TaskID:      taskID,
		MaxInsights: 5,
		MaxChars:    500,
	}
}

// ProjectInsights converts prior INSIGHT events into human-readable prompt memory strings.
func ProjectInsights(events []Event, opts InsightProjectionOptions) []string {
	if opts.MaxInsights <= 0 {
		opts.MaxInsights = 5
	}
	if opts.MaxChars <= 0 {
		opts.MaxChars = 500
	}

	taskScoped := collectInsightStrings(events, opts, true)
	traceScoped := collectInsightStrings(events, opts, false)

	selected := make([]string, 0, opts.MaxInsights)
	seen := make(map[string]struct{})

	for _, s := range taskScoped {
		if len(selected) >= opts.MaxInsights {
			break
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		selected = append(selected, s)
	}
	for _, s := range traceScoped {
		if len(selected) >= opts.MaxInsights {
			break
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		selected = append(selected, s)
	}
	return selected
}

func collectInsightStrings(events []Event, opts InsightProjectionOptions, taskScoped bool) []string {
	var out []string
	for _, ev := range events {
		if ev.Type != EventInsight {
			continue
		}
		kind := PayloadKind(ev.Payload)
		if !IsPromptMemoryInsightKind(kind) {
			continue
		}
		payloadTaskID := insightPayloadTaskID(ev.Payload)
		if taskScoped {
			if opts.TaskID == "" || payloadTaskID != opts.TaskID {
				continue
			}
		} else if payloadTaskID != "" {
			continue
		}
		line, ok := RenderInsightForPrompt(ev)
		if !ok {
			continue
		}
		out = append(out, truncateInsightLine(line, opts.MaxChars))
	}
	return out
}

func insightPayloadTaskID(payload json.RawMessage) string {
	var meta struct {
		TaskID string `json:"taskId"`
	}
	_ = json.Unmarshal(payload, &meta)
	return strings.TrimSpace(meta.TaskID)
}

// RenderInsightForPrompt formats one INSIGHT event as a prompt-memory string.
func RenderInsightForPrompt(ev Event) (string, bool) {
	if ev.Type != EventInsight {
		return "", false
	}
	kind := PayloadKind(ev.Payload)
	switch InsightKind(kind) {
	case InsightRunSummary:
		var p NarrativeInsightPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Summary) == "" {
			return "", false
		}
		return formatInsightLine("Summary", ev.BeeLabel(), p.Summary), true
	case InsightReviewNote:
		var p NarrativeInsightPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Summary) == "" {
			return "", false
		}
		label := "Review note"
		if severity := ClassifyReviewSeverity(p.Severity); severity != ReviewSeverityUnspecified {
			label = fmt.Sprintf("Review note (%s)", severity)
		}
		return formatInsightLine(label, ev.BeeLabel(), p.Summary), true
	case InsightContextNote:
		var p NarrativeInsightPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Summary) == "" {
			return "", false
		}
		return formatInsightLine("Context", ev.BeeLabel(), p.Summary), true
	case InsightHumanFeedback:
		var p HumanFeedbackPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil || strings.TrimSpace(p.Message) == "" {
			return "", false
		}
		label := "Beekeeper feedback"
		if verdict := ClassifyFeedbackVerdict(p.Verdict); verdict != FeedbackVerdictUnspecified {
			label = fmt.Sprintf("Beekeeper feedback (%s)", verdict)
		}
		return formatInsightLine(label, "", p.Message), true
	default:
		return "", false
	}
}

func (ev Event) BeeLabel() string {
	// AgentID is the run id; bee role is not on Event today — keep prefix generic.
	if strings.TrimSpace(ev.AgentID) != "" {
		return ev.AgentID
	}
	return "agent"
}

func formatInsightLine(label, source, text string) string {
	text = strings.TrimSpace(text)
	if label == "" {
		return text
	}
	if source != "" {
		return fmt.Sprintf("%s (%s): %s", label, source, text)
	}
	return fmt.Sprintf("%s: %s", label, text)
}

func truncateInsightLine(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
