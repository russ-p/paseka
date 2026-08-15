package export

import (
	"fmt"
	"strings"
)

// IncludeKind selects optional payload slices for a trace export.
type IncludeKind string

const (
	IncludeUsage     IncludeKind = "usage"
	IncludeDurations IncludeKind = "durations"
	IncludeBees      IncludeKind = "bees"
	IncludeColony    IncludeKind = "colony"
	IncludeCues      IncludeKind = "cues"
)

// IncludeSet is a normalized, deduplicated list of include tokens.
type IncludeSet []IncludeKind

// Has reports whether kind is enabled.
func (s IncludeSet) Has(kind IncludeKind) bool {
	for _, k := range s {
		if k == kind {
			return true
		}
	}
	return false
}

// ParseInclude normalizes cobra StringSlice values (repeatable and comma-separated).
func ParseInclude(flags []string) (IncludeSet, error) {
	seen := make(map[IncludeKind]struct{})
	var out IncludeSet
	for _, raw := range flags {
		for _, part := range strings.Split(raw, ",") {
			part = strings.ToLower(strings.TrimSpace(part))
			if part == "" {
				continue
			}
			kind := IncludeKind(part)
			if !kind.valid() {
				return nil, fmt.Errorf("invalid --include %q (supported: usage, durations, bees, colony, cues)", part)
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			out = append(out, kind)
		}
	}
	return out, nil
}

func (k IncludeKind) valid() bool {
	switch k {
	case IncludeUsage, IncludeDurations, IncludeBees, IncludeColony, IncludeCues:
		return true
	default:
		return false
	}
}
