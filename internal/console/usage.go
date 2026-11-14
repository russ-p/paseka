package console

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/russ-p/paseka/internal/runs"
)

// handleUsage answers GET /api/usage with the colony-wide usage report.
//
// The read is an observation over retained run records: it changes no file,
// publishes no event, and needs no bus. A malformed range or an unknown zone is
// the caller's error and is answered as a plain-text 400; a missing runs
// directory is an empty report rather than a failure.
//
// The reader may answer from its short-lived scan cache, and the report names
// the scan it came from in observedAt. `?refresh=1` is the page's refresh
// control: it bypasses the cache, scans again, and returns a new observedAt, so
// a trail that finished since the last read can appear on demand.
func (a *api) handleUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	// strconv.ParseBool accepts the forms a refresh control sends; anything
	// else is not a refresh, rather than a 400 for a query detail that cannot
	// make the report wrong.
	refresh, _ := strconv.ParseBool(query.Get("refresh"))
	reader := a.usage
	if reader == nil {
		// A server is built with a reader; a caller that constructs the API
		// context directly still gets a working, uncached read.
		reader = runs.NewUsageReader(a.ctx.ColonyRoot)
	}
	report, err := reader.Read(runs.UsageQuery{
		Since:    query.Get("since"),
		Until:    query.Get("until"),
		Timezone: query.Get("timezone"),
		// Repeated params are the filters: `?adapter=a&adapter=b` is the set {a,
		// b}. The reader normalizes them and echoes the set it applied.
		Adapters: query["adapter"],
		Models:   query["model"],
		Bees:     query["bee"],
		Purposes: query["purpose"],
	}, refresh)
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
