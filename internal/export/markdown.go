package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/hiveview"
)

// RenderMarkdown builds a Markdown document for one trace export.
func RenderMarkdown(data TraceExportData) ([]byte, error) {
	var buf bytes.Buffer

	traceID := data.Trace.TraceID
	fmt.Fprintf(&buf, "# Paseka export — %s\n\n", traceID)
	fmt.Fprintf(&buf, "Flight trail **%s** · project **%s**\n\n", traceID, data.Slug)
	fmt.Fprintf(&buf, "Exported %s\n\n", formatTime(data.ExportedAt))

	writeMarkdownOverview(&buf, data)
	writeMarkdownTasks(&buf, data.Trace.Tasks)
	writeMarkdownRuns(&buf, data.Runs)
	writeMarkdownTimeline(&buf, data.Events)

	return buf.Bytes(), nil
}

func writeMarkdownOverview(buf *bytes.Buffer, data TraceExportData) {
	buf.WriteString("## Overview\n\n")
	fmt.Fprintf(buf, "- **Runs:** %d\n", data.Trace.RunCount)
	fmt.Fprintf(buf, "- **Tasks:** %d\n", data.Trace.TaskCount)
	if data.Trace.EnergyBudget > 0 {
		fmt.Fprintf(buf, "- **Honey reserve:** %d / %d\n", data.Trace.EnergyRemaining, data.Trace.EnergyBudget)
	}
	fmt.Fprintf(buf, "- **Last activity:** %s\n", formatTime(data.Trace.LastActivityAt))
	if bees := strings.Join(data.Trace.Bees, ", "); bees != "" {
		fmt.Fprintf(buf, "- **Bees:** %s\n", bees)
	}
	fmt.Fprintf(buf, "- **Colony root:** `%s`\n", data.ColonyRoot)
	if wt := data.Trace.Worktree; wt != nil {
		fmt.Fprintf(buf, "- **Worktree:** `%s`\n", wt.Path)
		if wt.Branch != "" {
			fmt.Fprintf(buf, "- **Branch:** %s\n", wt.Branch)
		}
		if wt.BaseSHA != "" {
			fmt.Fprintf(buf, "- **Base SHA:** `%s`\n", wt.BaseSHA)
		}
		fmt.Fprintf(buf, "- **Worktree created:** %s\n", formatTime(wt.CreatedAt))
	}
	buf.WriteString("\n")
}

func writeMarkdownTasks(buf *bytes.Buffer, tasks []hiveview.TaskSummaryView) {
	buf.WriteString("## Tasks\n\n")
	if len(tasks) == 0 {
		buf.WriteString("_No tasks in this trace._\n\n")
		return
	}
	for _, task := range tasks {
		title := task.Title
		if title == "" {
			title = task.TaskID
		}
		line := fmt.Sprintf("- **%s** (`%s`)", title, task.TaskID)
		if task.Bee != "" {
			line += fmt.Sprintf(" · %s", task.Bee)
		}
		line += fmt.Sprintf(" — %s", task.Status)
		buf.WriteString(line + "\n")
	}
	buf.WriteString("\n")
}

func writeMarkdownRuns(buf *bytes.Buffer, runs []hiveview.RunView) {
	buf.WriteString("## Runs _(oldest first)_\n\n")
	if len(runs) == 0 {
		buf.WriteString("_No runs in this trace._\n\n")
		return
	}
	for _, run := range runs {
		label := run.Bee
		if label == "" {
			label = run.AgentID
		}
		fmt.Fprintf(buf, "### %s — %s\n\n", label, run.State)
		fmt.Fprintf(buf, "- **Agent:** `%s`\n", run.AgentID)
		if run.TaskID != "" {
			fmt.Fprintf(buf, "- **Task:** `%s`\n", run.TaskID)
		}
		timeRange := formatTime(run.StartedAt)
		if run.FinishedAt != nil {
			timeRange += " → " + formatTime(*run.FinishedAt)
		}
		fmt.Fprintf(buf, "- **Time:** %s\n", timeRange)
		if summary := strings.TrimSpace(run.Summary); summary != "" {
			buf.WriteString("\n")
			buf.WriteString(summary)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
}

func writeMarkdownTimeline(buf *bytes.Buffer, events []hiveview.EventFeedItem) {
	fmt.Fprintf(buf, "## Timeline _(%d events, oldest first)_\n\n", len(events))
	if len(events) == 0 {
		buf.WriteString("_No events in this trace._\n\n")
		return
	}
	for _, item := range events {
		kind := string(item.Type)
		if item.PayloadKind != "" {
			kind += " · " + item.PayloadKind
		}
		fmt.Fprintf(buf, "### %s\n\n", kind)
		fmt.Fprintf(buf, "- **Time:** %s\n", formatTime(item.CreatedAt))
		fmt.Fprintf(buf, "- **Agent:** `%s`", item.AgentID)
		if item.Bee != "" {
			fmt.Fprintf(buf, " · %s", item.Bee)
		}
		buf.WriteString("\n")
		if item.TaskID != "" {
			fmt.Fprintf(buf, "- **Task:** `%s`\n", item.TaskID)
		}
		if item.Severity != "" {
			fmt.Fprintf(buf, "- **Severity:** %s\n", item.Severity)
		}
		if summary := strings.TrimSpace(item.Summary); summary != "" {
			buf.WriteString("\n")
			buf.WriteString(summary)
			buf.WriteString("\n")
		}
		buf.WriteString("\n```json\n")
		buf.WriteString(rawEventJSON(item.Raw))
		buf.WriteString("\n```\n\n")
	}
}
