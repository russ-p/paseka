package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/filelock"
)

// TrailOriginFileName is the durable trail-owned origin record. It lives at the
// trace root beside the trail event log, so worktree-only cleanup cannot remove
// the evidence of where the trail came from.
const TrailOriginFileName = "origin.json"

// Origin kinds. An absent record is unknown; it is deliberately not one of
// these values, so a legacy trail can never be reported as a known non-cue
// origin merely because it has no cue.
const (
	OriginKindCue     = "cue"
	OriginKindManual  = "manual"
	OriginKindFork    = "fork"
	OriginKindOther   = "other"
	OriginKindUnknown = "unknown"
)

const trailOriginVersion = 1

// TrailOrigin records where a trail came from exactly once, at creation or
// admission.
//
// A cue-created trail names the invocation and cue that created it. Every other
// creation path records a known non-cue kind and leaves the cue fields empty
// rather than inventing a cue identity. A task's cue id is not the trail's
// origin: forks copy their source task's cue id and later tasks may carry a
// different one, so readers must consult this record.
type TrailOrigin struct {
	Version int    `json:"version"`
	TraceID string `json:"traceId"`
	Kind    string `json:"kind"`
	// Source names the ingress channel or creation path that admitted the
	// trail, for example "cli", "console", "task.create", or "trail.fork".
	Source string `json:"source,omitempty"`
	// Actor is the initiating identity when one was recorded.
	Actor string `json:"actor,omitempty"`
	// InvocationID and CueID are set only when Kind is cue. They name the
	// original ingress invocation, not the latest cue that touched the trail.
	InvocationID string `json:"invocationId,omitempty"`
	CueID        string `json:"cueId,omitempty"`
	// SourceTraceID, ForkID, and RequestID describe a fork origin. They are
	// evidence of the fork action, not of the source trail's cue.
	SourceTraceID string    `json:"sourceTraceId,omitempty"`
	ForkID        string    `json:"forkId,omitempty"`
	RequestID     string    `json:"requestId,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// TrailOriginPath returns .paseka/runs/<traceId>/origin.json.
func TrailOriginPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, TrailOriginFileName)
}

// EnsureTrailOrigin records the trail's origin exactly once.
//
// The first caller wins. An existing origin is returned unchanged, so a later
// cue on a manually created trail is an addition rather than a new origin, and
// concurrent creation converges on one record. The record is written before any
// ingress event is published, so a crash cannot leave a trail whose origin is
// later inferred from the earliest task or source timestamp.
func EnsureTrailOrigin(colonyRoot, traceID string, origin TrailOrigin) (TrailOrigin, bool, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return TrailOrigin{}, false, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return TrailOrigin{}, false, err
	}
	normalized, err := normalizeTrailOrigin(traceID, origin)
	if err != nil {
		return TrailOrigin{}, false, err
	}

	root := filepath.Dir(TrailOriginPath(colonyRoot, traceID))
	if err := os.MkdirAll(root, 0o755); err != nil {
		return TrailOrigin{}, false, fmt.Errorf("runs: mkdir trail origin dir: %w", err)
	}

	release, err := acquireTrailOriginLock(filepath.Join(root, ".origin.lock"))
	if err != nil {
		return TrailOrigin{}, false, err
	}
	defer release()

	if existing, ok, err := ReadTrailOrigin(colonyRoot, traceID); err != nil {
		return TrailOrigin{}, false, err
	} else if ok {
		return existing, false, nil
	}

	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return TrailOrigin{}, false, err
	}
	if err := writeFileAtomic(TrailOriginPath(colonyRoot, traceID), append(data, '\n'), 0o644); err != nil {
		return TrailOrigin{}, false, err
	}
	return normalized, true, nil
}

// ReadTrailOrigin loads a trail's durable origin. A missing record is not an
// error: ok is false and the caller reports an unknown origin rather than
// guessing manual or deriving one from tasks.
func ReadTrailOrigin(colonyRoot, traceID string) (TrailOrigin, bool, error) {
	if strings.TrimSpace(colonyRoot) == "" {
		return TrailOrigin{}, false, fmt.Errorf("runs: colony root is required")
	}
	if err := validateTrailRecordID("traceId", traceID); err != nil {
		return TrailOrigin{}, false, err
	}
	data, err := os.ReadFile(TrailOriginPath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return TrailOrigin{}, false, nil
		}
		return TrailOrigin{}, false, err
	}
	var origin TrailOrigin
	if err := json.Unmarshal(data, &origin); err != nil {
		return TrailOrigin{}, false, fmt.Errorf("runs: parse trail origin %q: %w", traceID, err)
	}
	if origin.TraceID == "" {
		origin.TraceID = traceID
	}
	return origin, true, nil
}

// LoadTrailOrigin is an alias for ReadTrailOrigin for readers that describe the
// operation as loading retained evidence.
func LoadTrailOrigin(colonyRoot, traceID string) (TrailOrigin, bool, error) {
	return ReadTrailOrigin(colonyRoot, traceID)
}

func normalizeTrailOrigin(traceID string, origin TrailOrigin) (TrailOrigin, error) {
	origin.TraceID = strings.TrimSpace(origin.TraceID)
	if origin.TraceID == "" {
		origin.TraceID = traceID
	}
	if origin.TraceID != traceID {
		return TrailOrigin{}, fmt.Errorf("runs: trail origin belongs to trace %q, not %q", origin.TraceID, traceID)
	}
	origin.Kind = strings.TrimSpace(origin.Kind)
	switch origin.Kind {
	case OriginKindCue:
		if strings.TrimSpace(origin.InvocationID) == "" {
			return TrailOrigin{}, fmt.Errorf("runs: cue trail origin requires an invocation id")
		}
		if strings.TrimSpace(origin.CueID) == "" {
			return TrailOrigin{}, fmt.Errorf("runs: cue trail origin requires a cue id")
		}
	case OriginKindManual, OriginKindFork, OriginKindOther:
		if strings.TrimSpace(origin.CueID) != "" || strings.TrimSpace(origin.InvocationID) != "" {
			return TrailOrigin{}, fmt.Errorf("runs: %s trail origin must not invent a cue identity", origin.Kind)
		}
	case OriginKindUnknown, "":
		return TrailOrigin{}, fmt.Errorf("runs: trail origin kind is required")
	default:
		return TrailOrigin{}, fmt.Errorf("runs: unsupported trail origin kind %q", origin.Kind)
	}
	if origin.Version == 0 {
		origin.Version = trailOriginVersion
	}
	if origin.CreatedAt.IsZero() {
		origin.CreatedAt = time.Now().UTC()
	}
	return origin, nil
}

func acquireTrailOriginLock(path string) (func(), error) {
	lock, err := filelock.Acquire(path, filelock.Options{
		Budget:     5 * time.Second,
		StaleAfter: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return lock.Release, nil
}
