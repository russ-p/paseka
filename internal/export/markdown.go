package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/taskledger"
)

// RenderMarkdown builds a Markdown document for one trace export.
func RenderMarkdown(data TraceExportData) ([]byte, error) {
	var buf bytes.Buffer

	traceID := data.Trace.TraceID
	fmt.Fprintf(&buf, "# Paseka export — %s\n\n", traceID)
	fmt.Fprintf(&buf, "Flight trail **%s** · project **%s**\n\n", traceID, data.Slug)
	fmt.Fprintf(&buf, "Exported %s\n\n", formatTime(data.ExportedAt))

	writeMarkdownOverview(&buf, data)
	writeMarkdownTrailEvidence(&buf, data.Trace)
	writeMarkdownTasks(&buf, data.Trace.Tasks)
	writeMarkdownRuns(&buf, data)
	writeMarkdownTimeline(&buf, data.Events)
	writeMarkdownConfigSnapshots(&buf, data)
	if data.Include.Has(IncludeConfig) {
		writeMarkdownConfiguration(&buf, data.Config)
	}
	if data.Include.Has(IncludePatches) {
		writeMarkdownPatches(&buf, data)
	}

	return buf.Bytes(), nil
}

func writeMarkdownOverview(buf *bytes.Buffer, data TraceExportData) {
	buf.WriteString("## Overview\n\n")
	fmt.Fprintf(buf, "- **Runs:** %d\n", data.Trace.RunCount)
	fmt.Fprintf(buf, "- **Tasks:** %d\n", data.Trace.TaskCount)
	if data.Trace.EnergyBudget > 0 {
		fmt.Fprintf(buf, "- **Honey reserve:** %s\n", taskledger.FormatHoneyRemaining(
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

func writeMarkdownTrailEvidence(buf *bytes.Buffer, trace hiveview.TraceDetailView) {
	buf.WriteString("## Trail evidence\n\n")
	fmt.Fprintf(buf, "- **Admission state:** %s\n", trace.AdmissionState)
	if trace.Config != nil {
		fmt.Fprintf(buf, "- **Configuration policy:** %s\n", trace.ConfigPolicy)
		fmt.Fprintf(buf, "- **Revision:** %d\n", trace.ConfigRevision)
		fmt.Fprintf(buf, "- **Initial ConfigRef:** `%s`\n", trace.InitialConfigRef)
		fmt.Fprintf(buf, "- **Active ConfigRef:** `%s`\n", trace.ActiveConfigRef)
		if len(trace.Config.Snapshots) == 0 {
			buf.WriteString("- **Snapshots:** none retained\n")
		} else {
			buf.WriteString("- **Snapshots:**\n")
			for _, snap := range trace.Config.Snapshots {
				fmt.Fprintf(buf, "  - `%s` (%d files, retained at `%s`)\n", snap.ConfigRef, snap.Files, snap.Path)
			}
		}
		if trace.Config.Pending != nil {
			fmt.Fprintf(buf, "- **Pending change:** %s · source %s · expected revision %d\n",
				trace.Config.Pending.ChangeID, trace.Config.Pending.Source, trace.Config.Pending.ExpectedRevision)
		}
	} else {
		buf.WriteString("- **Configuration:** not captured (legacy trail); configuration evidence is unavailable\n")
	}
	if trace.Lineage != nil {
		lineage := trace.Lineage
		fmt.Fprintf(buf, "- **Fork of:** `%s`\n", lineage.SourceTrace)
		fmt.Fprintf(buf, "- **Fork id:** `%s`\n", lineage.ForkID)
		fmt.Fprintf(buf, "- **Source task:** `%s`\n", lineage.SourceTask)
		if lineage.SourceAttempt != "" {
			fmt.Fprintf(buf, "- **Source attempt:** `%s`\n", lineage.SourceAttempt)
		}
		fmt.Fprintf(buf, "- **Source ConfigRef:** `%s` (revision %d)\n", lineage.SourceConfigRef, lineage.SourceConfigRevision)
		fmt.Fprintf(buf, "- **Base SHA:** `%s`\n", lineage.BaseSHA)
		if lineage.BaseOverridden {
			fmt.Fprintf(buf, "- **Source base SHA:** `%s` (base overridden)\n", lineage.SourceBaseSHA)
		}
		fmt.Fprintf(buf, "- **Delta:** %s\n", lineage.Delta.Form)
		if lineage.Branch != "" {
			fmt.Fprintf(buf, "- **Branch:** `%s`\n", lineage.Branch)
		}
	} else if trace.ConfigPolicy == "pinned" {
		buf.WriteString("- **Fork lineage:** missing for this pinned trail; source comparison is unavailable\n")
	}
	if len(trace.Evidence) > 0 {
		buf.WriteString("\n### Missing evidence\n\n")
		for _, note := range trace.Evidence {
			fmt.Fprintf(buf, "- %s\n", note)
		}
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
		if run.Model != "" {
			fmt.Fprintf(buf, "- **Model:** `%s`\n", run.Model)
		}
		if run.Profile != "" {
			fmt.Fprintf(buf, "- **Profile:** `%s`\n", run.Profile)
		}
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
		if data.Include.Has(IncludeAgentLogs) {
			writeMarkdownAgentLog(buf, agentLogByAgentID(data.AgentLogs, run.AgentID))
		}
		buf.WriteString("\n")
	}
}

func writeMarkdownAgentLog(buf *bytes.Buffer, log AgentLogExport) {
	buf.WriteString("\n#### Agent log\n\n")
	if log.Omitted != "" {
		fmt.Fprintf(buf, "_omitted: %s_\n", log.Omitted)
		return
	}
	if len(log.ToolCalls) == 0 {
		buf.WriteString("_No tool calls._\n")
		return
	}
	buf.WriteString("| Name | Args | Call id |\n| --- | --- | --- |\n")
	for _, call := range log.ToolCalls {
		fmt.Fprintf(buf, "| %s | %s | %s |\n",
			escapeMarkdownTableCell(call.Name),
			escapeMarkdownTableCell(call.Args),
			escapeMarkdownTableCell(call.CallID))
	}
	if log.Truncated {
		buf.WriteString("\n_Showing first 100 tool calls._\n")
	}
}

func escapeMarkdownTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
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

func writeMarkdownPatches(buf *bytes.Buffer, data TraceExportData) {
	buf.WriteString("## Proposal patches\n\n")
	if len(data.Patches) == 0 {
		buf.WriteString("_No proposal patches archived for this trail._\n\n")
		writeMarkdownPatchDiagnostics(buf, data.PatchDiagnostics)
		return
	}
	for _, patch := range data.Patches {
		fmt.Fprintf(buf, "### %s · %s\n\n", patch.AgentID, patchPublicationLabel(patch.Publication))
		fmt.Fprintf(buf, "`%s`\n\n", patch.Ref)
		fmt.Fprintf(buf, "- Bee: **%s** · mode `%s` · kind `%s` · workspace `%s`\n", patch.Bee, patch.Mode, patch.Kind, patch.Workspace)
		if patch.TaskID != "" {
			fmt.Fprintf(buf, "- Task: `%s`\n", patch.TaskID)
		}
		if patch.BaseSha != "" {
			fmt.Fprintf(buf, "- Base SHA: `%s`\n", patch.BaseSha)
		}
		if patch.WorktreePath != "" {
			fmt.Fprintf(buf, "- Worktree: `%s`\n", patch.WorktreePath)
		}
		fmt.Fprintf(buf, "- Captured %s · %s", patch.CapturedAt.UTC().Format("2006-01-02 15:04:05 UTC"), patchSizeLabel(patch.Bytes, patch.FileCount))
		if patch.PublishError != "" {
			fmt.Fprintf(buf, " · last publish error: %s", patch.PublishError)
		}
		buf.WriteString("\n")
		if patch.Summary != "" {
			fmt.Fprintf(buf, "- Summary: %s\n", patch.Summary)
		}
		buf.WriteString("\n")
		if patch.Omitted != "" {
			fmt.Fprintf(buf, "_%s_\n\n", patch.Omitted)
			continue
		}
		writeDiffFence(buf, patch.Content)
	}
	writeMarkdownPatchDiagnostics(buf, data.PatchDiagnostics)
	buf.WriteString("_Proposal patches archive what the diff collector returned when a run finished._")
	buf.WriteString(" They are not a strict per-run delta, not a workspace backup,")
	buf.WriteString(" and not proof of the code a Guard approved._\n\n")
}

func writeMarkdownPatchDiagnostics(buf *bytes.Buffer, diagnostics []PatchDiagnosticExport) {
	if len(diagnostics) == 0 {
		return
	}
	buf.WriteString("### Incomplete patch records\n\n")
	for _, d := range diagnostics {
		fmt.Fprintf(buf, "- `%s`: %s\n", d.AgentID, d.Reason)
	}
	buf.WriteString("\n")
}

// writeDiffFence writes a patch body in a fence that the body cannot terminate itself.
func writeDiffFence(buf *bytes.Buffer, content string) {
	fence := "```"
	if strings.Contains(content, "```") {
		fence = "~~~"
	}
	buf.WriteString(fence)
	buf.WriteString("diff\n")
	buf.WriteString(strings.TrimRight(content, "\n"))
	buf.WriteString("\n")
	buf.WriteString(fence)
	buf.WriteString("\n\n")
}

func patchPublicationLabel(state string) string {
	switch state {
	case "published":
		return "published"
	case "unconfirmed":
		return "unconfirmed — a broker attempt was made and its outcome is unknown"
	case "unpublished":
		return "unpublished — no broker was asked"
	default:
		return state
	}
}

func patchSizeLabel(bytes int64, fileCount *int) string {
	files := "unknown file count"
	if fileCount != nil {
		files = fmt.Sprintf("%d file(s)", *fileCount)
	}
	return fmt.Sprintf("%s, %d bytes", files, bytes)
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

func writeMarkdownConfiguration(buf *bytes.Buffer, config *ConfigExport) {
	buf.WriteString("## Configuration\n\n")
	if config == nil {
		buf.WriteString("_Configuration evidence was not requested._\n\n")
		return
	}
	if config.Missing != "" {
		fmt.Fprintf(buf, "_%s_\n\n", config.Missing)
		return
	}
	fmt.Fprintf(buf, "- **Policy:** %s\n", config.Policy)
	fmt.Fprintf(buf, "- **Revision:** %d\n", config.Revision)
	fmt.Fprintf(buf, "- **Initial ConfigRef:** `%s`\n", config.InitialConfigRef)
	fmt.Fprintf(buf, "- **Active ConfigRef:** `%s`\n", config.ActiveConfigRef)
	fmt.Fprintf(buf, "- **Retained versions:** %d\n", len(config.Snapshots))
	if omitted := configOmittedCount(config); omitted > 0 {
		fmt.Fprintf(buf, "- **Omitted files:** %d (contents deliberately not exported)\n", omitted)
	}
	buf.WriteString("\n")
	for _, snapshot := range config.Snapshots {
		roles := strings.Join(snapshot.Roles, ", ")
		if roles == "" {
			roles = "retained"
		}
		fmt.Fprintf(buf, "### `%s` (%s)\n\n", snapshot.ConfigRef, roles)
		if snapshot.Missing != "" {
			fmt.Fprintf(buf, "_unavailable: %s_\n\n", snapshot.Missing)
			continue
		}
		fmt.Fprintf(buf, "%d file(s)", snapshot.FileCount)
		if snapshot.Omitted > 0 {
			fmt.Fprintf(buf, " · %d omitted", snapshot.Omitted)
		}
		buf.WriteString("\n\n")
		for _, file := range snapshot.Files {
			fmt.Fprintf(buf, "#### `%s`\n\n", file.Path)
			fmt.Fprintf(buf, "%d bytes · mode %o", file.Bytes, file.Mode)
			if file.Private {
				buf.WriteString(" · private")
			}
			buf.WriteString("\n\n")
			if file.Omitted != "" {
				fmt.Fprintf(buf, "_omitted: %s_\n\n", file.Omitted)
				continue
			}
			writeYAMLFence(buf, file.Content)
		}
	}
	omissions := configOmissions(config)
	if len(omissions) > 0 {
		buf.WriteString("### Export omissions\n\n")
		for _, note := range omissions {
			fmt.Fprintf(buf, "- %s\n", note)
		}
		buf.WriteString("\n")
	}
}
