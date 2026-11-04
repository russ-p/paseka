package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/russ-p/paseka/internal/redact"
)

// Stable omit reasons for provider transcript resolution. Every consumer shares
// this set so "missing", "unreadable", and "too big" read the same everywhere.
// A failed read is an omission, never an error.
const (
	SessionLogOmittedNoProviderSessionID = "no providerSessionId"
	SessionLogOmittedUnsupported         = "unsupported"
	SessionLogOmittedNotImplemented      = "not implemented"
	SessionLogOmittedStoreNotFound       = "store not found"
	SessionLogOmittedParseError          = "parse error"
	SessionLogOmittedResolveError        = "resolve error"
	SessionLogOmittedTimeout             = "timeout"
)

// Turn roles. A transcript carries only these three.
const (
	TurnRoleUser      = "user"
	TurnRoleAssistant = "assistant"
	TurnRoleSystem    = "system"
)

// Tool statuses. Status is always present on a tool part; a provider that
// records none yields ToolStatusUnknown rather than an empty field.
const (
	ToolStatusUnknown = "unknown"
	ToolStatusOK      = "ok"
	ToolStatusError   = "error"
)

// Default per-part caps, measured in runes. They are applied by every adapter
// through NormalizeTranscript so truncation is uniform.
const (
	DefaultMaxTextRunes       = 8000
	DefaultMaxReasoningRunes  = 16000
	DefaultMaxToolInputRunes  = 2000
	DefaultMaxToolOutputRunes = 16000
)

// TranscriptRequest is the read-time lookup for a provider-native session. The
// association fields mirror the run; Include, Limits, and Window say how much
// of the conversation the caller wants.
type TranscriptRequest struct {
	ProviderSessionID string
	Adapter           string
	Workspace         string
	ColonyRoot        string
	RunDir            string
	AgentID           string
	Include           IncludeMask
	Limits            TranscriptLimits
	Window            TranscriptWindow
}

// ToolCall is one Agent log row (name, short args, optional call id). It is a
// projection over a Transcript, not a resolver result type.
type ToolCall struct {
	Name      string
	CallID    string
	Args      string
	Timestamp time.Time
}

// Transcript is a provider-ordered, normalized conversation. Omitted is a
// stable reason when the session could not be resolved; HasMore and NextCursor
// report the page request honestly.
type Transcript struct {
	Turns      []Turn
	Omitted    string
	Truncated  bool
	HasMore    bool
	NextCursor string
}

// Turn is one ordered conversation turn. Timestamp is the provider's, and is
// zero when the provider records none.
type Turn struct {
	Role      string
	Timestamp time.Time
	Parts     []Part
}

// PartKind identifies the payload a Part carries.
type PartKind string

const (
	PartText      PartKind = "text"
	PartReasoning PartKind = "reasoning"
	PartTool      PartKind = "tool"
)

// Part is one typed piece of a turn. Tool identity is always present; input and
// output are filled only when the request mask asks for them. Truncated marks a
// payload the shared cap cut, so a renderer can say the text is a prefix.
type Part struct {
	Kind      PartKind
	Text      string
	Reasoning string
	Tool      *ToolPart
	Truncated bool
}

// ToolPart is a tool call with always-present identity and status, plus payloads
// the caller may or may not have asked for.
type ToolPart struct {
	Name   string
	CallID string
	Status string
	Input  string
	Output string
}

// IncludeMask selects which payloads the caller wants. Tool identity (name, call
// id, status) is always present because it is tiny and every view uses it.
type IncludeMask struct {
	Text       bool
	Reasoning  bool
	ToolInput  bool
	ToolOutput bool
	System     bool
}

// Named presets. A consumer picks one; it does not assemble a mask by hand.
func PresetToolCall() IncludeMask {
	return IncludeMask{ToolInput: true}
}

func PresetDialogue() IncludeMask {
	return IncludeMask{Text: true}
}

func PresetDialogueWithReasoning() IncludeMask {
	return IncludeMask{Text: true, Reasoning: true}
}

func PresetFull() IncludeMask {
	return IncludeMask{Text: true, Reasoning: true, ToolInput: true, ToolOutput: true, System: true}
}

// TranscriptLimits caps each part in runes. A zero field takes its pinned
// default, so a caller that only cares about the preset still gets sane caps.
type TranscriptLimits struct {
	Text       int
	Reasoning  int
	ToolInput  int
	ToolOutput int
}

// DefaultTranscriptLimits returns the measured defaults for a full view.
func DefaultTranscriptLimits() TranscriptLimits {
	return TranscriptLimits{
		Text:       DefaultMaxTextRunes,
		Reasoning:  DefaultMaxReasoningRunes,
		ToolInput:  DefaultMaxToolInputRunes,
		ToolOutput: DefaultMaxToolOutputRunes,
	}
}

func (l TranscriptLimits) withDefaults() TranscriptLimits {
	if l.Text <= 0 {
		l.Text = DefaultMaxTextRunes
	}
	if l.Reasoning <= 0 {
		l.Reasoning = DefaultMaxReasoningRunes
	}
	if l.ToolInput <= 0 {
		l.ToolInput = DefaultMaxToolInputRunes
	}
	if l.ToolOutput <= 0 {
		l.ToolOutput = DefaultMaxToolOutputRunes
	}
	return l
}

// TranscriptWindow asks for a page of turns. A zero window means the whole
// transcript. Cursor is the opaque value from a previous NextCursor and is used
// when Offset is zero.
type TranscriptWindow struct {
	Offset int
	Limit  int
	Cursor string
}

// ToolResult is a tool result the provider stored separately from its call. The
// reader folds it into the matching tool part by call id, so consumers never see
// that provider shape.
type ToolResult struct {
	CallID string
	Output string
	Status string
}

// TranscriptResolver is an optional adapter capability: resolve a provider-native
// session into a normalized transcript. Resolution is read-only and may be
// bounded by the caller's context deadline.
type TranscriptResolver interface {
	ResolveTranscript(ctx context.Context, req TranscriptRequest) (Transcript, error)
}

// ResolveTranscript resolves req through resolver under ctx, turning errors and a
// deadline into stable omit reasons. It returns when ctx is done even if the
// resolver blocks, so a wedged provider cannot stall an export or a handler.
func ResolveTranscript(ctx context.Context, resolver TranscriptResolver, req TranscriptRequest) Transcript {
	if resolver == nil {
		return Transcript{Omitted: SessionLogOmittedUnsupported}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return Transcript{Omitted: SessionLogOmittedTimeout}
	}

	done := make(chan Transcript, 1)
	go func() {
		transcript, err := resolver.ResolveTranscript(ctx, req)
		if err != nil {
			done <- Transcript{Omitted: SessionLogOmittedResolveError}
			return
		}
		done <- transcript
	}()

	select {
	case transcript := <-done:
		return transcript
	case <-ctx.Done():
		return Transcript{Omitted: SessionLogOmittedTimeout}
	}
}

// NormalizeTranscript applies the request mask, redacts obvious secrets, the
// per-part caps, and the page window to a provider-ordered transcript. Redaction
// runs before truncation so a secret straddling a cap is replaced rather than
// emitted as a partial value. Adapters call it once before returning so every
// consumer sees the same masking, redaction, and truncation semantics.
func NormalizeTranscript(t Transcript, include IncludeMask, limits TranscriptLimits, window TranscriptWindow) Transcript {
	out := Transcript{Omitted: t.Omitted, Truncated: t.Truncated, Turns: []Turn{}}
	if t.Omitted != "" {
		return out
	}
	limits = limits.withDefaults()

	for _, turn := range t.Turns {
		role := strings.ToLower(strings.TrimSpace(turn.Role))
		switch role {
		case TurnRoleUser, TurnRoleAssistant:
		case TurnRoleSystem:
			if !include.System {
				continue
			}
		default:
			continue
		}

		next := Turn{Role: role, Timestamp: turn.Timestamp}
		for _, part := range turn.Parts {
			switch part.Kind {
			case PartText:
				if !include.Text || part.Text == "" {
					continue
				}
				text, cut := TruncateRunes(redact.Text(part.Text), limits.Text)
				out.Truncated = out.Truncated || cut
				next.Parts = append(next.Parts, Part{Kind: PartText, Text: text, Truncated: cut})
			case PartReasoning:
				if !include.Reasoning || part.Reasoning == "" {
					continue
				}
				reasoning, cut := TruncateRunes(redact.Text(part.Reasoning), limits.Reasoning)
				out.Truncated = out.Truncated || cut
				next.Parts = append(next.Parts, Part{Kind: PartReasoning, Reasoning: reasoning, Truncated: cut})
			case PartTool:
				if part.Tool == nil {
					continue
				}
				tool := *part.Tool
				if tool.Status == "" {
					tool.Status = ToolStatusUnknown
				}
				if !include.ToolInput {
					tool.Input = ""
				} else {
					input, cut := TruncateRunes(redact.Text(tool.Input), limits.ToolInput)
					out.Truncated = out.Truncated || cut
					tool.Input = input
				}
				if !include.ToolOutput {
					tool.Output = ""
				} else {
					output, cut := TruncateRunes(redact.Text(tool.Output), limits.ToolOutput)
					out.Truncated = out.Truncated || cut
					tool.Output = output
				}
				next.Parts = append(next.Parts, Part{Kind: PartTool, Tool: &tool})
			}
		}
		if len(next.Parts) == 0 {
			continue
		}
		out.Turns = append(out.Turns, next)
	}

	// A zero window means the whole transcript. A provider that already paged
	// reports its own signal through the result and is left alone.
	if window.Limit <= 0 {
		out.HasMore = t.HasMore
		out.NextCursor = t.NextCursor
		return out
	}

	visible := len(out.Turns)
	start, end := window.bounds(visible)
	out.Turns = out.Turns[start:end]
	if end < visible {
		out.HasMore = true
		out.NextCursor = encodeTurnCursor(end)
	}
	return out
}

func (w TranscriptWindow) bounds(total int) (int, int) {
	start := w.Offset
	if cursor, ok := decodeTurnCursor(w.Cursor); ok && start == 0 {
		start = cursor
	}
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := total
	if w.Limit > 0 && start+w.Limit < end {
		end = start + w.Limit
	}
	return start, end
}

// turnCursorPrefix marks the opaque offset cursor NormalizeTranscript hands back
// with a page that was cut.
const turnCursorPrefix = "turn:"

func encodeTurnCursor(offset int) string {
	return turnCursorPrefix + strconv.Itoa(offset)
}

func decodeTurnCursor(cursor string) (int, bool) {
	cursor = strings.TrimSpace(cursor)
	if !strings.HasPrefix(cursor, turnCursorPrefix) {
		return 0, false
	}
	offset, err := strconv.Atoi(strings.TrimPrefix(cursor, turnCursorPrefix))
	if err != nil || offset < 0 {
		return 0, false
	}
	return offset, true
}

// ProjectToolCalls flattens a transcript into Agent log rows in provider order.
// It reads only tool parts and performs no IO, so it is the shared tool-call
// projection rather than a reader variant.
func ProjectToolCalls(t Transcript) []ToolCall {
	calls := make([]ToolCall, 0)
	for _, turn := range t.Turns {
		for _, part := range turn.Parts {
			if part.Kind != PartTool || part.Tool == nil {
				continue
			}
			calls = append(calls, ToolCall{
				Name:      part.Tool.Name,
				CallID:    part.Tool.CallID,
				Args:      summarizeToolInput(part.Tool.Input),
				Timestamp: turn.Timestamp,
			})
		}
	}
	return calls
}

// AttachToolResults folds separately stored tool results into the tool parts that
// carry the same call id. Unmatched results are dropped and results never become
// turns of their own, so consumers see one shape regardless of the provider.
func AttachToolResults(turns []Turn, results []ToolResult) {
	if len(results) == 0 {
		return
	}
	byCallID := make(map[string]*ToolPart, len(results))
	for i := range turns {
		for j := range turns[i].Parts {
			tool := turns[i].Parts[j].Tool
			if turns[i].Parts[j].Kind == PartTool && tool != nil && tool.CallID != "" {
				byCallID[tool.CallID] = tool
			}
		}
	}
	for _, result := range results {
		tool, ok := byCallID[strings.TrimSpace(result.CallID)]
		if !ok {
			continue
		}
		tool.Output = result.Output
		if result.Status != "" {
			tool.Status = result.Status
		}
	}
}

// TruncateRunes caps s at limit runes, appending an ellipsis when it cuts, and
// reports whether it truncated. A limit <= 0 means no cap.
func TruncateRunes(s string, limit int) (string, bool) {
	if limit <= 0 || s == "" {
		return s, false
	}
	if utf8.RuneCountInString(s) <= limit {
		return s, false
	}
	return string([]rune(s)[:limit]) + "…", true
}

// summarizeToolInput reduces a tool input to the shipped Agent log summary: the
// path, url, or command when the input carries one, the compact JSON otherwise.
func summarizeToolInput(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		for _, key := range []string{"path", "url", "command"} {
			s, _ := obj[key].(string)
			if strings.TrimSpace(s) != "" {
				return s
			}
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return raw
		}
		return string(b)
	}
	// A capped tool input is a JSON prefix. Salvage the first summary key so a
	// huge argument still projects to the same short row the shipped reader gave.
	if value, ok := leadingStringField(raw, "path", "url", "command"); ok {
		return value
	}
	return raw
}

// leadingStringField extracts the first of keys from a possibly truncated JSON
// object prefix.
func leadingStringField(raw string, keys ...string) (string, bool) {
	for _, key := range keys {
		marker := fmt.Sprintf("%q", key)
		idx := strings.Index(raw, marker)
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(raw[idx+len(marker):], " \t\r\n")
		if !strings.HasPrefix(rest, ":") {
			continue
		}
		rest = strings.TrimLeft(rest[1:], " \t\r\n")
		if !strings.HasPrefix(rest, `"`) {
			continue
		}
		var value string
		if err := json.NewDecoder(strings.NewReader(rest)).Decode(&value); err == nil && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}
