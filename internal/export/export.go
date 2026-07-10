package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/runs"
)

// Options configures a trace HTML export.
type Options struct {
	TraceID   string
	OutputDir string
}

// TraceExportData is the view model passed to the HTML renderer.
type TraceExportData struct {
	Slug       string
	ColonyRoot string
	ExportedAt time.Time
	Trace      console.TraceDetailView
	Runs       []console.RunView
	Events     []console.EventFeedItem
}

// ExportTrace writes a self-contained HTML report for one flight trail.
func ExportTrace(ctx colony.Context, opts Options) (string, error) {
	traceID := strings.TrimSpace(opts.TraceID)
	if traceID == "" {
		return "", fmt.Errorf("trace id is required")
	}

	detail, ok, err := console.GetTrace(ctx, traceID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("trace %q not found", traceID)
	}

	events, err := runs.ReadTraceEvents(ctx.ColonyRoot, traceID)
	if err != nil {
		return "", err
	}
	feedItems := console.BuildEventFeedItems(ctx.ColonyRoot, traceID, events)

	runsView := append([]console.RunView(nil), detail.Runs...)
	console.SortRunsAsc(runsView)

	outDir := opts.OutputDir
	if outDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		outDir = wd
	}

	filename := OutputFilename(ctx.Slug, traceID)
	outPath := filepath.Join(outDir, filename)

	data := TraceExportData{
		Slug:       ctx.Slug,
		ColonyRoot: ctx.ColonyRoot,
		ExportedAt: time.Now().UTC(),
		Trace:      detail,
		Runs:       runsView,
		Events:     feedItems,
	}

	html, err := RenderHTML(data)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(outPath, html, 0o644); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(outPath)
	if err != nil {
		return outPath, nil
	}
	return abs, nil
}

// OutputFilename returns the default export filename for a slug and trace id.
func OutputFilename(slug, traceID string) string {
	return fmt.Sprintf("paseka-export-%s-%s.html", sanitizeFilename(slug), sanitizeFilename(traceID))
}

func sanitizeFilename(s string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		":", "-",
		"*", "-",
		"?", "-",
		"\"", "-",
		"<", "-",
		">", "-",
		"|", "-",
		" ", "-",
	)
	return replacer.Replace(strings.TrimSpace(s))
}
