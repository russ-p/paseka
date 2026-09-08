package hiveview

import "github.com/russ-p/paseka/internal/cues"

// LoadStandingTraceIDs returns standing.trace ids from loaded colony cues.
// Load errors yield an empty set so trace lists still render.
func LoadStandingTraceIDs(colonyRoot string) map[string]struct{} {
	ids, err := cues.StandingTraceIDs(colonyRoot)
	if err != nil || len(ids) == 0 {
		return nil
	}
	return ids
}

// EnrichTraceStanding sets Standing when the trace id is bound to a standing cue.
func EnrichTraceStanding(standing map[string]struct{}, view *TraceSummaryView) {
	if view == nil || len(standing) == 0 {
		return
	}
	_, view.Standing = standing[view.TraceID]
}
