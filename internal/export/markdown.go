package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/taskledger"
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
	writeMarkdownRuns(&buf, data)
	writeMarkdownTimeline(&buf, data.Events)
	writeMarkdownConfigSnapshots(&buf, data)

	return buf.Bytes(), nil
}

func writeMarkdownOverview(buf *bytes.Buffer, data TraceExportData) {
	buf.WriteString("## Overview\n\n")
	fmt.Fprintf(buf, "- **Runs:** %d\n", data.Trace.RunCount)
	fmt.Fprintf(buf, "- **Tasks:** %d\n", data.Trace.TaskCount)
	if data.Trace.EnergyBudget > 0 {
		fmt.Fprintf(buf, "- **Honey reserve:** %s\n", taskledger.FormatHoneyPrimary(
			data.Trace.EnergyRemaining, data.Trace.EnergyBudget, data.Trace.EnergyAdded))
		if extra := taskledger.FormatHoneySecondary(data.Trace.EnergyBudget, data.Trace.EnergyAdded); extra != "" {
			fmt.Fprintf(buf, "- %s\n", extra)
		}
	}
	fmt.Fprintf(buf, "- **Last activity:** %s\n", formatTime(data.Trace.LastActivityAt))
	if bees := strings.Join(data.Trace.Bees, ", "); bees != "" {
		fmt.Fprintf(buf, "- **Bees:** %s\n", bees)
	}
	if data.Include.Has(IncludeUsage) {
		if line := formatUsageAggregate(data.Trace.Usage); line != "" {
			fmt.Fprintf(buf, "- **Usage:** %s\n", line)
		}
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

func writeMarkdownRuns(buf *bytes.Buffer, data TraceExportData) {
	buf.WriteString("## Runs _(oldest first)_\n\n")
	if len(data.Runs) == 0 {
		buf.WriteString("_No runs in this trace._\n\n")
		return
	}
	for _, run := range data.Runs {
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
		if data.Include.Has(IncludeDurations) {
			if dur := formatWallDuration(run); dur != "" {
				fmt.Fprintf(buf, "- **Duration:** %s\n", dur)
			}
		}
		if data.Include.Has(IncludeUsage) {
			if line := formatUsageTokens(run.Usage); line != "" {
				fmt.Fprintf(buf, "- **Usage:** %s\n", line)
			}
		}
		if summary := strings.TrimSpace(run.Summary); summary != "" {
			buf.WriteString("\n")
			buf.WriteString(summary)
			buf.WriteString("\n")
		}
		buf.WriteString("\n")
	}
}

func writeMarkdownConfigSnapshots(buf *bytes.Buffer, data TraceExportData) {
	if data.Include.Has(IncludeBees) {
		buf.WriteString("## Bees\n\n")
		if len(data.BeeYAML) == 0 {
			buf.WriteString("_No bee YAML found._\n\n")
		} else {
			for _, snap := range data.BeeYAML {
				fmt.Fprintf(buf, "### %s\n\n", snap.Name)
				fmt.Fprintf(buf, "`%s`\n\n", snap.Path)
				writeYAMLFence(buf, snap.Content)
			}
		}
	}
	if data.Include.Has(IncludeColony) && data.ColonyYAML != nil {
		buf.WriteString("## Colony\n\n")
		fmt.Fprintf(buf, "`%s`\n\n", data.ColonyYAML.Path)
		if data.ColonyYAML.Missing {
			buf.WriteString("_No colony.yaml found._\n\n")
		} else {
			writeYAMLFence(buf, data.ColonyYAML.Content)
		}
	}
	if data.Include.Has(IncludeCues) {
		buf.WriteString("## Cues\n\n")
		if len(data.CueYAML) == 0 {
			buf.WriteString("_No cues found._\n\n")
		} else {
			for _, snap := range data.CueYAML {
				fmt.Fprintf(buf, "### %s\n\n", snap.Name)
				fmt.Fprintf(buf, "`%s`\n\n", snap.Path)
				writeYAMLFence(buf, snap.Content)
			}
		}
	}
	if data.Include.Has(IncludeArtifacts) {
		buf.WriteString("## Trail artifacts\n\n")
		if len(data.Artifacts) == 0 {
			buf.WriteString("_No trail artifacts in the comb._\n\n")
		} else {
			for _, art := range data.Artifacts {
				title := art.Title
				if title == "" {
					title = art.ArtifactKind
				}
				fmt.Fprintf(buf, "### %s\n\n", title)
				fmt.Fprintf(buf, "`%s`\n\n", art.Ref)
				if art.Omitted != "" {
					fmt.Fprintf(buf, "_%s_\n\n", art.Omitted)
					continue
				}
				if art.IsMarkdown {
					buf.WriteString(strings.TrimRight(art.Content, "\n"))
					buf.WriteString("\n\n")
				} else {
					writeYAMLFence(buf, art.Content)
				}
			}
		}
	}
}

func writeYAMLFence(buf *bytes.Buffer, content string) {
	fence := "```"
	if strings.Contains(content, "```") {
		fence = "~~~"
	}
	buf.WriteString(fence)
	buf.WriteString("yaml\n")
	buf.WriteString(strings.TrimRight(content, "\n"))
	buf.WriteString("\n")
	buf.WriteString(fence)
	buf.WriteString("\n\n")
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
