package review

import (
	"fmt"
	"strings"
)

// MergeMessageParts is the subject and optional body for a merge commit.
type MergeMessageParts struct {
	Subject string
	Body    string
}

// FormatMessage returns the git merge commit message (subject, or subject + blank line + body).
func (p MergeMessageParts) FormatMessage() string {
	subject := strings.TrimSpace(p.Subject)
	body := strings.TrimSpace(p.Body)
	if body == "" {
		return subject
	}
	return subject + "\n\n" + body
}

// ComposeMergeMessage builds merge commit subject/body for merge-on-approve.
// Subject comes from trimmed mergeMessage when non-empty, else the default trace subject.
// Body comes from traceSummary when mergeMessage does not already include a non-empty body.
func ComposeMergeMessage(traceID, mergeMessage, traceSummary string) MergeMessageParts {
	subject, hitlBody := splitMergeMessage(strings.TrimSpace(mergeMessage))
	if subject == "" {
		subject = defaultMergeSubject(traceID)
	}

	body := strings.TrimSpace(hitlBody)
	if body == "" {
		body = strings.TrimSpace(traceSummary)
	}
	return MergeMessageParts{Subject: subject, Body: body}
}

func defaultMergeSubject(traceID string) string {
	return fmt.Sprintf("paseka: merge trace %s", traceID)
}

// splitMergeMessage separates an optional HITL merge message into subject and body.
// The first line (or first paragraph before a blank line) is the subject; any non-empty
// trimmed remainder is treated as an explicit body that overrides trace.summary.
func splitMergeMessage(raw string) (subject, body string) {
	if raw == "" {
		return "", ""
	}
	if before, after, ok := strings.Cut(raw, "\n\n"); ok {
		subject = strings.TrimSpace(before)
		body = strings.TrimSpace(after)
		return subject, body
	}
	lines := strings.Split(raw, "\n")
	subject = strings.TrimSpace(lines[0])
	if len(lines) == 1 {
		return subject, ""
	}
	body = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	return subject, body
}
