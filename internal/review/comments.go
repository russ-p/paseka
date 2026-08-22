package review

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/bus"
	"github.com/paseka/paseka/internal/taskledger"
)

const (
	// ReviewCommentsCombRel is the stable trail-comb basename for annotated review packets.
	ReviewCommentsCombRel = "review-comments.md"

	reviewCommentsFeedbackDefault = "Review comments written to comb"
)

// ReviewComment is one line-anchored note in an annotated review packet.
type ReviewComment struct {
	Path      string `json:"path"`
	Side      string `json:"side"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine,omitempty"`
	Snippet   string `json:"snippet,omitempty"`
	Body      string `json:"body"`
}

// CommentsPacket is the server-rendered annotated review payload.
type CommentsPacket struct {
	HeadSHA  string
	Summary  string
	Comments []ReviewComment
}

// AnnotatedReviewInput describes a comb-backed annotated review submit.
type AnnotatedReviewInput struct {
	TraceID  string
	TaskID   string
	AgentID  string
	Producer string
	Packet   CommentsPacket
}

// AnnotatedReviewResult is the outcome of an annotated review submit.
type AnnotatedReviewResult struct {
	CombRef      string
	ReworkTaskID string
}

// DeliverCommentsInput copies an existing Markdown packet into the trail comb.
type DeliverCommentsInput struct {
	TraceID  string
	TaskID   string
	AgentID  string
	Producer string
	Content  []byte
	Feedback string
}

// ValidateCommentsPacket ensures an annotated review has content and well-formed comments.
func ValidateCommentsPacket(pkt CommentsPacket) error {
	summary := strings.TrimSpace(pkt.Summary)
	if len(pkt.Comments) == 0 && summary == "" {
		return fmt.Errorf("review comments: at least one comment or an overall summary is required")
	}
	for i, c := range pkt.Comments {
		if err := validateReviewComment(c); err != nil {
			return fmt.Errorf("review comments[%d]: %w", i, err)
		}
	}
	return nil
}

func validateReviewComment(c ReviewComment) error {
	if strings.TrimSpace(c.Path) == "" {
		return fmt.Errorf("path is required")
	}
	side := strings.TrimSpace(strings.ToLower(c.Side))
	if side != "new" && side != "old" {
		return fmt.Errorf("side must be new or old")
	}
	if c.StartLine < 1 {
		return fmt.Errorf("startLine must be >= 1")
	}
	end := c.EndLine
	if end == 0 {
		end = c.StartLine
	}
	if end < c.StartLine {
		return fmt.Errorf("endLine must be >= startLine")
	}
	if strings.TrimSpace(c.Body) == "" {
		return fmt.Errorf("body is required")
	}
	return nil
}

// RenderCommentsMarkdown renders a stable Markdown packet for the trail comb.
func RenderCommentsMarkdown(pkt CommentsPacket) []byte {
	var b strings.Builder
	b.WriteString("# Review comments\n\n")
	if sha := strings.TrimSpace(pkt.HeadSHA); sha != "" {
		b.WriteString("headSha: `")
		b.WriteString(sha)
		b.WriteString("`\n\n")
	}
	if summary := strings.TrimSpace(pkt.Summary); summary != "" {
		b.WriteString(summary)
		b.WriteString("\n\n")
	}
	byPath := map[string][]ReviewComment{}
	pathOrder := make([]string, 0, len(pkt.Comments))
	for _, c := range pkt.Comments {
		if _, ok := byPath[c.Path]; !ok {
			pathOrder = append(pathOrder, c.Path)
		}
		byPath[c.Path] = append(byPath[c.Path], c)
	}
	for _, path := range pathOrder {
		b.WriteString("## `")
		b.WriteString(path)
		b.WriteString("`\n\n")
		for _, c := range byPath[path] {
			end := c.EndLine
			if end == 0 {
				end = c.StartLine
			}
			lineLabel := fmt.Sprintf("L%d", c.StartLine)
			if end != c.StartLine {
				lineLabel = fmt.Sprintf("L%d–%d", c.StartLine, end)
			}
			b.WriteString("- **")
			b.WriteString(strings.ToLower(strings.TrimSpace(c.Side)))
			b.WriteString("** ")
			b.WriteString(lineLabel)
			b.WriteString("\n")
			if snippet := strings.TrimSpace(c.Snippet); snippet != "" {
				b.WriteString("  ```\n  ")
				b.WriteString(strings.ReplaceAll(snippet, "\n", "\n  "))
				b.WriteString("\n  ```\n")
			}
			b.WriteString("  ")
			b.WriteString(strings.TrimSpace(c.Body))
			b.WriteString("\n\n")
		}
	}
	return []byte(b.String())
}

// ShortFeedbackMessage returns the human.feedback message for an annotated review.
func ShortFeedbackMessage(summary string) string {
	if s := strings.TrimSpace(summary); s != "" {
		return s
	}
	return reviewCommentsFeedbackDefault
}

// SubmitAnnotatedReview writes the comb packet, announces it, then publishes short human.feedback.
// On a final merge gate it also plans a rework task (Slice C).
func SubmitAnnotatedReview(ctx context.Context, pub bus.Publisher, colonyRoot string, ledger taskledger.Ledger, in AnnotatedReviewInput) (AnnotatedReviewResult, error) {
	if err := ValidateCommentsPacket(in.Packet); err != nil {
		return AnnotatedReviewResult{}, err
	}
	return deliverAnnotatedReview(ctx, pub, colonyRoot, ledger, DeliverCommentsInput{
		TraceID:  in.TraceID,
		TaskID:   in.TaskID,
		AgentID:  in.AgentID,
		Producer: in.Producer,
		Content:  RenderCommentsMarkdown(in.Packet),
		Feedback: ShortFeedbackMessage(in.Packet.Summary),
	})
}

// DeliverReviewCommentsFile copies Markdown into the trail comb, announces it, and
// publishes short human.feedback. On a final merge gate it also plans rework.
func DeliverReviewCommentsFile(ctx context.Context, pub bus.Publisher, colonyRoot string, ledger taskledger.Ledger, in DeliverCommentsInput) (AnnotatedReviewResult, error) {
	if len(bytes.TrimSpace(in.Content)) == 0 {
		return AnnotatedReviewResult{}, fmt.Errorf("review comments: file is empty")
	}
	return deliverAnnotatedReview(ctx, pub, colonyRoot, ledger, in)
}

func deliverAnnotatedReview(ctx context.Context, pub bus.Publisher, colonyRoot string, ledger taskledger.Ledger, in DeliverCommentsInput) (AnnotatedReviewResult, error) {
	if err := EnsureRejectable(ledger, in.TraceID, in.TaskID); err != nil {
		return AnnotatedReviewResult{}, err
	}
	if pub == nil {
		return AnnotatedReviewResult{}, fmt.Errorf("nats client is required")
	}
	snap, err := ledger.Snapshot(in.TraceID)
	if err != nil {
		return AnnotatedReviewResult{}, err
	}
	task := snap.Tasks[in.TaskID]
	isFinal := taskledger.IsFinalReviewTask(task)
	var source taskledger.TaskSnapshot
	if isFinal {
		source, err = EnsureRequestChanges(snap)
		if err != nil {
			return AnnotatedReviewResult{}, err
		}
	}
	producer := strings.TrimSpace(in.Producer)
	if producer == "" {
		producer = artifacts.ProducerConsole
	}
	combRef, err := artifacts.WriteFile(colonyRoot, in.TraceID, ReviewCommentsCombRel, in.Content)
	if err != nil {
		return AnnotatedReviewResult{}, err
	}
	item, err := artifacts.ItemFromFile(colonyRoot, in.TraceID, ReviewCommentsCombRel)
	if err != nil {
		return AnnotatedReviewResult{}, err
	}
	item.Ref = combRef
	payload, err := artifacts.BuildWrittenPayload([]artifacts.Item{item})
	if err != nil {
		return AnnotatedReviewResult{}, err
	}
	if err := artifacts.PublishWritten(ctx, pub, colonyRoot, "", in.TraceID, producer, payload); err != nil {
		return AnnotatedReviewResult{}, err
	}
	agentID := strings.TrimSpace(in.AgentID)
	if agentID == "" {
		agentID = "human"
	}
	if err := Reject(ctx, pub, ledger, RejectInput{
		TraceID:  in.TraceID,
		TaskID:   in.TaskID,
		Feedback: ShortFeedbackMessage(in.Feedback),
		AgentID:  agentID,
		Ref:      combRef,
	}); err != nil {
		return AnnotatedReviewResult{}, err
	}
	result := AnnotatedReviewResult{CombRef: combRef}
	if isFinal {
		reworkID, err := PlanReworkTask(ctx, pub, source, in.TraceID, agentID, colonyRoot)
		if err != nil {
			return AnnotatedReviewResult{}, err
		}
		result.ReworkTaskID = reworkID
	}
	return result, nil
}

// RejectResponseMessage returns operator-facing copy after a reject or annotated submit.
func RejectResponseMessage(isFinal, annotated bool, reworkTaskID string) string {
	if annotated {
		if isFinal {
			if id := strings.TrimSpace(reworkTaskID); id != "" {
				return "Request changes sent. Rework task " + id + " planned; merge gate remains open — approve when ready."
			}
			return "Review comments saved to comb. Merge gate remains open — approve when ready."
		}
		return "Review comments saved to comb. For review: required tasks the runtime will return the task to ready."
	}
	if isFinal {
		return "Feedback published. Merge gate remains open."
	}
	return "Feedback published. For review: required tasks the runtime will return the task to ready."
}
