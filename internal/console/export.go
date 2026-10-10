package console

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/russ-p/paseka/internal/export"
)

// handleTraceExport renders one trail as a downloadable report. It reuses
// `internal/export` — the same renderer `paseka export` writes to a file — so the
// console never grows a second renderer to keep in sync. The body is the report
// itself rather than JSON, so `format` decides the Content-Type and the server's
// own OutputFilename names the attachment.
func (a *api) handleTraceExport(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	format, err := export.ParseFormat(r.URL.Query().Get("format"))
	if err != nil {
		writeError(w, err)
		return
	}
	include, err := export.ParseInclude(r.URL.Query()["include"])
	if err != nil {
		writeError(w, err)
		return
	}

	content, err := export.RenderTrace(a.ctx, export.Options{
		TraceID: traceID,
		Format:  format,
		Include: include,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	contentType := "text/html; charset=utf-8"
	if format == export.FormatMarkdown {
		contentType = "text/markdown; charset=utf-8"
	}
	filename := export.OutputFilename(a.ctx.Slug, traceID, format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
