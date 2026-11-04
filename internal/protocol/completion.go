package protocol

import (
	"strings"
	"time"
)

// SignalTrailWorkCompleted is the platform milestone that announces the
// trail's accepted working set — ordinary work plus every closing round — and
// its mandatory checks have passed. It is not final human acceptance and not a
// delivery outcome: the merge gate, approval, PR publication, and merge stay
// separate observations.
const SignalTrailWorkCompleted = "trail.work.completed"

// TrailWorkCompletedPayload is emitted as SIGNAL with
// payload.kind=trail.work.completed.
//
// CompletionID is the durable identity of one completion episode; bus delivery
// is at least once, so consumers deduplicate by it. WorkRevision names the
// accepted work the completion describes, which lets a reader tell an older
// completion from the trail's current work. ResultRef and MetricsRef are
// colony-root-relative references to immutable trail-owned evidence captured
// before this event was published.
type TrailWorkCompletedPayload struct {
	Kind         string    `json:"kind"`
	CompletionID string    `json:"completionId"`
	WorkRevision int       `json:"workRevision"`
	CompletedAt  time.Time `json:"completedAt"`
	ResultRef    string    `json:"resultRef"`
	MetricsRef   string    `json:"metricsRef"`
}

// IsTrailWorkCompletion reports whether kind is the work-completion milestone.
func IsTrailWorkCompletion(kind string) bool {
	return strings.TrimSpace(kind) == SignalTrailWorkCompleted
}
