package colony

import (
	"time"

	"github.com/paseka/paseka/internal/ids"
)

const traceIDPrefix = "trace-"

// NewTraceID returns a lexicographically sortable trace identifier.
// Layout: trace- + 16 lowercase hex chars (48-bit UTC ms + 16-bit random).
func NewTraceID() (string, error) {
	return ids.Prefixed(traceIDPrefix)
}

// ParseTraceIDTime extracts the UTC timestamp from an auto-generated mini-ULID trace id.
// Returns false for ids that do not match the expected format.
func ParseTraceIDTime(id string) (time.Time, bool) {
	return ids.ParsePrefixedTime(traceIDPrefix, id)
}
