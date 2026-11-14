package console

import (
	"errors"
	"net/http"

	"github.com/russ-p/paseka/internal/runs"
)

// handleUsage answers GET /api/usage with the colony-wide usage report.
//
// The read is an observation over retained run records: it changes no file,
// publishes no event, and needs no bus. A malformed range or an unknown zone is
// the caller's error and is answered as a plain-text 400; a missing runs
// directory is an empty report rather than a failure.
func (a *api) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	report, err := runs.ReadUsage(a.ctx.ColonyRoot, runs.UsageQuery{
		Since:    query.Get("since"),
		Until:    query.Get("until"),
		Timezone: query.Get("timezone"),
	})
	if err != nil {
		var usageErr *runs.UsageError
		if errors.As(err, &usageErr) {
			http.Error(w, usageErr.Reason, http.StatusBadRequest)
			return
		}
		writeError(w, err)
		return
	}
	writeJSON(w, report)
}
