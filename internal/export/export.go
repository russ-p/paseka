package export

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/runs"
)

// Options configures a trace export.
type Options struct {
	TraceID   string
	OutputDir string
	Format    Format
	Include   IncludeSet
}

// TraceExportData is the view model passed to export renderers.
type TraceExportData struct {
	Slug       string
	ColonyRoot string
	ExportedAt time.Time
	Trace      hiveview.TraceDetailView
	Runs       []hiveview.RunView
	Events     []hiveview.EventFeedItem
	Include    IncludeSet
	BeeYAML    []NamedYAML
	ColonyYAML *NamedYAML
	CueYAML    []NamedYAML
}

// ExportTrace writes a self-contained trace report for one flight trail.
func ExportTrace(ctx colony.Context, opts Options) (string, error) {
	traceID := strings.TrimSpace(opts.TraceID)
	if traceID == "" {
		return "", fmt.Errorf("trace id is required")
	}

	detail, ok, err := hiveview.GetTrace(ctx, traceID)
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
	feedItems := hiveview.BuildEventFeedItems(ctx.ColonyRoot, traceID, events)

	runsView := append([]hiveview.RunView(nil), detail.Runs...)
	hiveview.SortRunsAsc(runsView)

	outDir := opts.OutputDir
	if outDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		outDir = wd
	}

	format := opts.Format
	if format == "" {
		format = FormatHTML
	}

	filename := OutputFilename(ctx.Slug, traceID, format)
	outPath := filepath.Join(outDir, filename)

	data, err := buildTraceExportData(ctx, detail, runsView, feedItems, opts.Include)
	if err != nil {
		return "", err
	}

	var content []byte
	switch format {
	case FormatHTML:
		content, err = RenderHTML(data)
	case FormatMarkdown:
		content, err = RenderMarkdown(data)
	default:
		return "", fmt.Errorf("unsupported export format %q", format)
	}
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(outPath, content, 0o644); err != nil {
		return "", err
	}

	abs, err := filepath.Abs(outPath)
	if err != nil {
		return outPath, nil
	}
	return abs, nil
}

// OutputFilename returns the default export filename for a slug, trace id, and format.
func OutputFilename(slug, traceID string, format Format) string {
	ext := "html"
	if format == FormatMarkdown {
		ext = "md"
	}
	return fmt.Sprintf("paseka-export-%s-%s.%s", sanitizeFilename(slug), sanitizeFilename(traceID), ext)
}

func buildTraceExportData(ctx colony.Context, detail hiveview.TraceDetailView, runsView []hiveview.RunView, feedItems []hiveview.EventFeedItem, include IncludeSet) (TraceExportData, error) {
	data := TraceExportData{
		Slug:       ctx.Slug,
		ColonyRoot: ctx.ColonyRoot,
		ExportedAt: time.Now().UTC(),
		Trace:      detail,
		Runs:       runsView,
		Events:     feedItems,
		Include:    include,
	}
	if include.Has(IncludeBees) {
		bees, err := loadBeeYAML(ctx.ColonyRoot, detail.Bees)
		if err != nil {
			return TraceExportData{}, err
		}
		data.BeeYAML = bees
	}
	if include.Has(IncludeColony) {
		colonyYAML, err := loadColonyYAML(ctx.ColonyRoot)
		if err != nil {
			return TraceExportData{}, err
		}
		data.ColonyYAML = colonyYAML
	}
	if include.Has(IncludeCues) {
		cueYAML, err := loadCueYAML(ctx.ColonyRoot)
		if err != nil {
			return TraceExportData{}, err
		}
		data.CueYAML = cueYAML
	}
	return data, nil
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
