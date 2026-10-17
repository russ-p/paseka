package console

import (
	"net/http"
	"strings"

	"github.com/russ-p/paseka/internal/hiveview"
)

// proposalPatchListResponse is the trail-scoped listing: complete records plus the
// diagnostics for anything on disk that is not one.
type proposalPatchListResponse struct {
	Patches     []hiveview.ProposalPatchView       `json:"patches"`
	Diagnostics []hiveview.ProposalPatchDiagnostic `json:"diagnostics"`
}

// handleTraceProposalPatches serves the proposal archive for one trail. It is a listing and
// a bounded content read, kept separate from the merge-gate preview endpoint so "what each
// run proposed" and "what is staged now" never render as the same thing.
func (a *api) handleTraceProposalPatches(w http.ResponseWriter, r *http.Request, traceID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agentId"))
	if agentID != "" {
		view, err := hiveview.GetTraceProposalPatchContent(a.ctx, traceID, agentID)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, view)
		return
	}

	list, diagnostics, err := hiveview.ListTraceProposalPatches(a.ctx, traceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if list == nil {
		list = []hiveview.ProposalPatchView{}
	}
	if diagnostics == nil {
		diagnostics = []hiveview.ProposalPatchDiagnostic{}
	}
	writeJSON(w, proposalPatchListResponse{Patches: list, Diagnostics: diagnostics})
}
