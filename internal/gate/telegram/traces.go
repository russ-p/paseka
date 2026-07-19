package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/console"
)

const defaultTracesLimit = 20

func (h *Handler) sendTraces(chatID int64) {
	list, err := console.ListTraces(h.Colony, defaultTracesLimit)
	if err != nil {
		h.sendPlain(chatID, 0, "traces unavailable: "+err.Error())
		return
	}
	h.sendPlain(chatID, 0, FormatTracesList(h.Config, list, time.Now()))
}

// FormatTracesList renders the /traces response body.
func FormatTracesList(cfg Config, traces []console.TraceSummaryView, now time.Time) string {
	if len(traces) == 0 {
		return "No traces yet."
	}
	lines := []string{"Recent traces", ""}
	for _, trace := range traces {
		lines = append(lines, formatTraceListLine(cfg, trace, now))
	}
	return strings.Join(lines, "\n")
}

func formatTraceListLine(cfg Config, trace console.TraceSummaryView, now time.Time) string {
	parts := []string{trace.TraceID, formatShortActivityTime(trace.LastActivityAt, now)}
	if hint := formatTraceStatusHint(trace); hint != "" {
		parts = append(parts, hint)
	}
	line := strings.Join(parts, " · ")
	if link := TraceConsoleURL(cfg, trace.TraceID); link != "" {
		line += "\n" + link
	}
	return line
}

func formatTraceStatusHint(trace console.TraceSummaryView) string {
	if trace.HasActive {
		return "active"
	}
	if trace.HasFailures {
		return "failed"
	}
	if trace.RunCount > 0 {
		if trace.RunCount == 1 {
			return "1 run"
		}
		return fmt.Sprintf("%d runs", trace.RunCount)
	}
	return ""
}

func formatShortActivityTime(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "—"
	}
	elapsed := now.Sub(t)
	if elapsed < 0 {
		elapsed = 0
	}
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
	default:
		return t.UTC().Format("Jan 2")
	}
}

// TraceConsoleURL returns a Queen Console deep-link for one trace when configured.
func TraceConsoleURL(cfg Config, traceID string) string {
	base := strings.TrimRight(strings.TrimSpace(cfg.ConsoleBaseURL), "/")
	traceID = strings.TrimSpace(traceID)
	if base == "" || traceID == "" {
		return ""
	}
	return base + "/#traces/" + traceID
}
