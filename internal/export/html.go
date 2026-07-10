package export

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/console"
)

type htmlEventView struct {
	Type        string
	PayloadKind string
	CreatedAt   string
	Summary     string
	AgentID     string
	Bee         string
	TaskID      string
	Severity    string
	RawJSON     template.HTML
}

type htmlPageData struct {
	Slug            string
	ColonyRoot      string
	TraceID         string
	ExportedAt      string
	LastActivityAt  string
	RunCount        int
	TaskCount       int
	Bees            string
	EnergyBudget    int
	EnergyRemaining int
	LowEnergy       bool
	HasEnergy       bool
	WorktreePath    string
	WorktreeBranch  string
	WorktreeBaseSHA string
	WorktreeCreated string
	HasWorktree     bool
	Tasks           []console.TaskSummaryView
	Runs            []console.RunView
	Events          []htmlEventView
}

// RenderHTML builds a self-contained HTML document for one trace export.
func RenderHTML(data TraceExportData) ([]byte, error) {
	page := htmlPageData{
		Slug:           data.Slug,
		ColonyRoot:     data.ColonyRoot,
		TraceID:        data.Trace.TraceID,
		ExportedAt:     formatTime(data.ExportedAt),
		LastActivityAt: formatTime(data.Trace.LastActivityAt),
		RunCount:       data.Trace.RunCount,
		TaskCount:      data.Trace.TaskCount,
		Bees:           strings.Join(data.Trace.Bees, ", "),
		Tasks:          data.Trace.Tasks,
		Runs:           data.Runs,
	}
	if data.Trace.EnergyBudget > 0 {
		page.HasEnergy = true
		page.EnergyBudget = data.Trace.EnergyBudget
		page.EnergyRemaining = data.Trace.EnergyRemaining
		page.LowEnergy = data.Trace.LowEnergy
	}
	if data.Trace.Worktree != nil {
		page.HasWorktree = true
		page.WorktreePath = data.Trace.Worktree.Path
		page.WorktreeBranch = data.Trace.Worktree.Branch
		page.WorktreeBaseSHA = data.Trace.Worktree.BaseSHA
		page.WorktreeCreated = formatTime(data.Trace.Worktree.CreatedAt)
	}
	for _, item := range data.Events {
		page.Events = append(page.Events, htmlEventView{
			Type:        string(item.Type),
			PayloadKind: item.PayloadKind,
			CreatedAt:   formatTime(item.CreatedAt),
			Summary:     item.Summary,
			AgentID:     item.AgentID,
			Bee:         item.Bee,
			TaskID:      item.TaskID,
			Severity:    item.Severity,
			RawJSON:     template.HTML(rawEventJSON(item.Raw)), //nolint:gosec // escaped via json.Marshal
		})
	}

	funcs := template.FuncMap{
		"badgeClass":     badgeClass,
		"formatTime":     formatTime,
		"formatMarkdown": formatMarkdown,
		"lower":          strings.ToLower,
		"kindSuffix": func(kind string) string {
			if kind == "" {
				return ""
			}
			return " · " + kind
		},
	}
	tmpl, err := template.New("export").Funcs(funcs).Parse(exportHTMLTemplate)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, page); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func badgeClass(state string) string {
	s := strings.ToLower(strings.TrimSpace(state))
	switch s {
	case "active", "running", "queued", "ready", "in_progress":
		return "active"
	case "completed", "done":
		return "completed"
	case "failed", "cancelled", "stale", "stopping":
		return "failed"
	default:
		return ""
	}
}

func rawEventJSON(ev any) string {
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Sprintf("%q", err.Error())
	}
	return string(b)
}

const exportHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Paseka export 🐝 — {{ .TraceID }}</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:ital,wght@0,400;0,700;1,400;1,700&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #0f1117;
  --panel: #171a22;
  --border: #2a3040;
  --text: #e8ecf4;
  --muted: #8b95a8;
  --accent: #6ea8ff;
  --danger: #ff6b6b;
  --ok: #5fd38d;
  --warn: #f0c14b;
  --font: ui-sans-serif, system-ui, -apple-system, Segoe UI, sans-serif;
  --mono: 'JetBrains Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
}
* { box-sizing: border-box; }
body {
  margin: 0;
  font-family: var(--font);
  background: var(--bg);
  color: var(--text);
  line-height: 1.45;
}
header {
  padding: 1.25rem 1.5rem 0.75rem;
  border-bottom: 1px solid var(--border);
}
header h1 { margin: 0; font-size: 1.35rem; }
.subtitle { margin: 0.25rem 0 0; color: var(--muted); font-size: 0.95rem; }
.layout {
  display: grid;
  gap: 1rem;
  padding: 1rem 1.5rem 2rem;
  max-width: 1100px;
  margin: 0 auto;
}
.panel {
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: 10px;
  padding: 1rem 1.1rem 1.2rem;
}
.panel h2, .panel h3 { margin: 0 0 0.75rem; font-size: 1rem; }
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 0.75rem;
  margin-bottom: 1rem;
}
.stat-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.65rem 0.75rem;
  background: #12151d;
}
.stat-label {
  display: block;
  font-size: 0.78rem;
  color: var(--muted);
  margin-bottom: 0.25rem;
}
.stat-value { font-size: 1rem; font-weight: 600; }
.meta {
  display: grid;
  grid-template-columns: 8rem 1fr;
  gap: 0.35rem 0.75rem;
  margin: 0 0 1rem;
  font-size: 0.88rem;
}
.meta dt { color: var(--muted); margin: 0; }
.meta dd { margin: 0; font-family: var(--mono); word-break: break-all; }
.muted { color: var(--muted); }
.bee { font-weight: 600; color: var(--text); }
.badge {
  display: inline-block;
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  padding: 0.1rem 0.4rem;
  border-radius: 999px;
  border: 1px solid var(--border);
}
.badge.active { color: var(--ok); border-color: #2f6b4a; }
.badge.completed { color: var(--accent); }
.badge.failed, .badge.cancelled { color: var(--danger); }
.badge.ok { color: var(--ok); border-color: #2f6b4a; }
.badge.warn { color: var(--warn); border-color: #6b5a2f; }
.compact-list { list-style: none; margin: 0; padding: 0; }
.compact-item {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.5rem 0.65rem;
  margin-bottom: 0.4rem;
  background: #12151d;
}
.compact-item .top {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
}
.compact-item .id {
  font-family: var(--mono);
  font-size: 0.78rem;
  color: var(--muted);
}
.timeline-feed { list-style: none; margin: 0; padding: 0; }
.timeline-item {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.65rem 0.75rem;
  margin-bottom: 0.5rem;
  background: #12151d;
}
.timeline-top {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  font-size: 0.82rem;
}
.timeline-type { font-family: var(--mono); color: var(--accent); }
.timeline-summary { margin-top: 0.35rem; font-size: 0.9rem; }
.timeline-meta {
  margin-top: 0.25rem;
  font-size: 0.78rem;
  font-family: var(--mono);
}
.timeline-raw {
  margin: 0.5rem 0 0;
  padding: 0.5rem;
  background: #0c0e14;
  border: 1px solid var(--border);
  border-radius: 6px;
  font-family: var(--mono);
  font-size: 0.75rem;
  overflow: auto;
  max-height: 16rem;
  white-space: pre-wrap;
  word-break: break-word;
}
details summary {
  cursor: pointer;
  color: var(--accent);
  font-size: 0.78rem;
  margin-top: 0.35rem;
  user-select: none;
}
.formatted-text {
  line-height: 1.5;
  word-break: break-word;
}
.formatted-text > :first-child { margin-top: 0; }
.formatted-text > :last-child { margin-bottom: 0; }
.formatted-text p { margin: 0.35rem 0; }
.formatted-text ul, .formatted-text ol {
  margin: 0.35rem 0;
  padding-left: 1.35rem;
}
.formatted-text li { margin: 0.15rem 0; }
.formatted-text h1, .formatted-text h2, .formatted-text h3,
.formatted-text h4, .formatted-text h5, .formatted-text h6 {
  margin: 0.5rem 0 0.25rem;
  font-size: 1em;
  font-weight: 600;
}
.formatted-text h1 { font-size: 1.1em; }
.formatted-text h2 { font-size: 1.05em; }
.formatted-text blockquote {
  margin: 0.35rem 0;
  padding-left: 0.75rem;
  border-left: 3px solid var(--border);
  color: var(--muted);
}
.formatted-text pre {
  margin: 0.35rem 0;
  padding: 0.5rem;
  background: #0c0e14;
  border: 1px solid var(--border);
  border-radius: 6px;
  overflow: auto;
  font-family: var(--mono);
  font-size: 0.8rem;
  white-space: pre-wrap;
}
.formatted-text code {
  font-family: var(--mono);
  font-size: 0.85em;
  background: #0c0e14;
  padding: 0.1rem 0.25rem;
  border-radius: 4px;
}
.formatted-text pre code {
  padding: 0;
  background: transparent;
}
.formatted-text a { color: var(--accent); }
code { font-family: var(--mono); font-size: 0.85em; }
</style>
</head>
<body>
<header>
  <h1>Paseka export 🐝</h1>
  <p class="subtitle">Flight trail <span class="bee">{{ .TraceID }}</span> · project <span class="bee">{{ .Slug }}</span></p>
  <p class="subtitle">Exported {{ .ExportedAt }}</p>
</header>
<main class="layout">
  <section class="panel">
    <h2>Overview</h2>
    <div class="stat-grid">
      <div class="stat-card">
        <span class="stat-label">Runs</span>
        <span class="stat-value">{{ .RunCount }}</span>
      </div>
      <div class="stat-card">
        <span class="stat-label">Tasks</span>
        <span class="stat-value">{{ .TaskCount }}</span>
      </div>
      {{ if .HasEnergy }}
      <div class="stat-card">
        <span class="stat-label">Honey reserve</span>
        <span class="stat-value">{{ .EnergyRemaining }} / {{ .EnergyBudget }}</span>
      </div>
      {{ end }}
      <div class="stat-card">
        <span class="stat-label">Last activity</span>
        <span class="stat-value" style="font-size:0.88rem">{{ .LastActivityAt }}</span>
      </div>
      {{ if .Bees }}
      <div class="stat-card">
        <span class="stat-label">Bees</span>
        <span class="stat-value" style="font-size:0.88rem">{{ .Bees }}</span>
      </div>
      {{ end }}
    </div>
    <dl class="meta">
      <dt>Colony root</dt><dd><code>{{ .ColonyRoot }}</code></dd>
      {{ if .HasWorktree }}
      <dt>Worktree</dt><dd><code>{{ .WorktreePath }}</code></dd>
      <dt>Branch</dt><dd>{{ if .WorktreeBranch }}{{ .WorktreeBranch }}{{ else }}—{{ end }}</dd>
      <dt>Base SHA</dt><dd><code>{{ if .WorktreeBaseSHA }}{{ .WorktreeBaseSHA }}{{ else }}—{{ end }}</code></dd>
      <dt>Worktree created</dt><dd>{{ .WorktreeCreated }}</dd>
      {{ end }}
    </dl>
  </section>

  <section class="panel">
    <h2>Tasks</h2>
    {{ if .Tasks }}
    <ul class="compact-list">
      {{ range .Tasks }}
      <li class="compact-item">
        <div class="top">
          <span class="bee">{{ if .Title }}{{ .Title }}{{ else }}{{ .TaskID }}{{ end }}</span>
          <span class="badge {{ badgeClass .Status }}">{{ .Status }}</span>
        </div>
        <div class="id">{{ .TaskID }}{{ if .Bee }} · {{ .Bee }}{{ end }}</div>
      </li>
      {{ end }}
    </ul>
    {{ else }}
    <p class="muted">No tasks in this trace.</p>
    {{ end }}
  </section>

  <section class="panel">
    <h2>Runs <span class="muted" style="font-weight:400;font-size:0.85rem">(oldest first)</span></h2>
    {{ if .Runs }}
    <ul class="compact-list">
      {{ range .Runs }}
      <li class="compact-item">
        <div class="top">
          <span class="bee">{{ if .Bee }}{{ .Bee }}{{ else }}{{ .AgentID }}{{ end }}</span>
          <span class="badge {{ badgeClass .State }}">{{ .State }}</span>
        </div>
        <div class="id">{{ .AgentID }}{{ if .TaskID }} · {{ .TaskID }}{{ end }}</div>
        <div class="muted" style="font-size:0.78rem;margin-top:0.2rem">{{ formatTime .StartedAt }}{{ if .FinishedAt }} → {{ formatTime .FinishedAt }}{{ end }}</div>
        {{ if .Summary }}<div class="formatted-text" style="font-size:0.85rem;margin-top:0.25rem">{{ formatMarkdown .Summary }}</div>{{ end }}
      </li>
      {{ end }}
    </ul>
    {{ else }}
    <p class="muted">No runs in this trace.</p>
    {{ end }}
  </section>

  <section class="panel">
    <h2>Timeline <span class="muted" style="font-weight:400;font-size:0.85rem">({{ len .Events }} events, oldest first)</span></h2>
    {{ if .Events }}
    <ul class="timeline-feed">
      {{ range .Events }}
      <li class="timeline-item">
        <div class="timeline-top">
          <span class="timeline-type">{{ .Type }}{{ kindSuffix .PayloadKind }}</span>
          <span class="muted">{{ .CreatedAt }}</span>
        </div>
        <div class="timeline-summary formatted-text">{{ formatMarkdown .Summary }}</div>
        <div class="timeline-meta muted">{{ .AgentID }}{{ if .Bee }} · {{ .Bee }}{{ end }}{{ if .TaskID }} · task {{ .TaskID }}{{ end }}{{ if .Severity }} · {{ .Severity }}{{ end }}</div>
        <details>
          <summary>Show raw JSON</summary>
          <pre class="timeline-raw">{{ .RawJSON }}</pre>
        </details>
      </li>
      {{ end }}
    </ul>
    {{ else }}
    <p class="muted">No events in this trace.</p>
    {{ end }}
  </section>
</main>
</body>
</html>`
