package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

// InsightKind identifies INSIGHT payload variants.
type InsightKind string

const (
	InsightRunSummary    InsightKind = "run.summary"
	InsightReviewNote    InsightKind = "review.note"
	InsightContextNote   InsightKind = "context.note"
	InsightHumanFeedback InsightKind = "human.feedback"
	InsightTraceTitle    InsightKind = "trace.title"
)

// MaxTraceTitleLen is the maximum length of payload.title on INSIGHT/trace.title.
const MaxTraceTitleLen = 120

// NarrativeInsightPayload is the shared shape for narrative INSIGHT events.
type NarrativeInsightPayload struct {
	Kind     InsightKind `json:"kind"`
	Summary  string      `json:"summary,omitempty"`
	TaskID   string      `json:"taskId,omitempty"`
	Severity string      `json:"severity,omitempty"`
}

// HumanFeedbackPayload is emitted as INSIGHT with payload.kind=human.feedback.
type HumanFeedbackPayload struct {
	Kind    InsightKind `json:"kind"`
	TaskID  string      `json:"taskId"`
	Message string      `json:"message"`
}

// TraceTitlePayload is emitted as INSIGHT with payload.kind=trace.title.
type TraceTitlePayload struct {
	Kind  InsightKind `json:"kind"`
	Title string      `json:"title"`
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
		if p.Severity != "" {
			label = fmt.Sprintf("Review note (%s)", p.Severity)
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
		return formatInsightLine("Beekeeper feedback", "", p.Message), true
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
