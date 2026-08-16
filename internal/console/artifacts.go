package console

import (
	"net/http"
	"strings"

	"github.com/paseka/paseka/internal/export"
	"github.com/paseka/paseka/internal/hiveview"
)

func (a *api) handleTraceArtifacts(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ref := strings.TrimSpace(r.URL.Query().Get("ref"))
	if ref != "" {
		view, err := hiveview.GetTraceArtifactContent(a.ctx, traceID, ref)
		if err != nil {
			writeError(w, err)
			return
		}
		if strings.EqualFold(r.URL.Query().Get("format"), "html") && view.Content != "" {
			view.ContentHTML = string(export.FormatMarkdownHTML(view.Content))
		} else if view.Content != "" && strings.HasSuffix(strings.ToLower(ref), ".md") {
			view.ContentHTML = string(export.FormatMarkdownHTML(view.Content))
		}
		writeJSON(w, view)
		return
	}
	list, err := hiveview.ListTraceArtifacts(a.ctx, traceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []hiveview.ArtifactView{}
	}
	writeJSON(w, list)
}
