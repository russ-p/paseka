package console

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteCueErrorStandingClientErrors(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{fmt.Errorf(`cue "daily-triage": trace id "other" does not match standing.trace "trail-daily-triage"`), http.StatusBadRequest},
		{fmt.Errorf(`cue "daily-triage": energy_budget is forbidden when standing is set`), http.StatusBadRequest},
		{fmt.Errorf(`cue "daily-triage": standing.stipend must be a positive integer`), http.StatusBadRequest},
		{fmt.Errorf(`cue "daily-triage": standing.trace "trail.daily" is not a legal trace id`), http.StatusBadRequest},
		{fmt.Errorf(`cue "beta": standing.trace "trail-daily-triage" is already declared by cue "alpha"`), http.StatusBadRequest},
		{fmt.Errorf(`cue "daily": standing: bee "watch": worktree must be false`), http.StatusBadRequest},
		{fmt.Errorf(`cue "missing": not found at .paseka/cues/missing.yaml`), http.StatusNotFound},
		{fmt.Errorf("nats url not configured (cue run requires NATS)"), http.StatusServiceUnavailable},
		{fmt.Errorf("ledger snapshot: connection reset"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		writeCueError(rec, tc.err)
		if rec.Code != tc.want {
			t.Errorf("%v: status = %d, want %d", tc.err, rec.Code, tc.want)
		}
	}
}
