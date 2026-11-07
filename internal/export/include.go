package export

import (
	"fmt"
	"strings"
)

// IncludeKind selects optional payload slices for a trace export.
type IncludeKind string

const (
	IncludeUsage      IncludeKind = "usage"
	IncludeDurations  IncludeKind = "durations"
	IncludeBees       IncludeKind = "bees"
	IncludeColony     IncludeKind = "colony"
	IncludeCues       IncludeKind = "cues"
	IncludeArtifacts  IncludeKind = "artifacts"
	IncludeAgentLogs  IncludeKind = "agent-logs"
	IncludeTranscript IncludeKind = "transcript"
	// IncludeReasoning is a conversation view with thinking blocks. It implies
	// IncludeTranscript: a bare reasoning token that changed nothing would be a
	// trap, so the set always carries both.
	IncludeReasoning IncludeKind = "reasoning"
	IncludePatches   IncludeKind = "patches"
	IncludeConfig    IncludeKind = "config"
	// IncludeSkills adds the per-run offered-skills section. The set is resolved
	// from each run's own pinned configuration snapshot, so a later store edit
	// or removal does not change what an old export says.
	IncludeSkills IncludeKind = "skills"
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

// HasConversation reports whether the per-run Conversation section is requested.
// reasoning is a conversation view, so it implies transcript.
func (s IncludeSet) HasConversation() bool {
	return s.Has(IncludeTranscript) || s.Has(IncludeReasoning)
}

// HasReasoning reports whether the Conversation section should carry thinking
// blocks.
func (s IncludeSet) HasReasoning() bool {
	return s.Has(IncludeReasoning)
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
				return nil, fmt.Errorf("invalid --include %q (supported: usage, durations, bees, colony, cues, artifacts, agent-logs, transcript, reasoning, patches, config, skills)", part)
			}
			if _, ok := seen[kind]; ok {
				continue
			}
			seen[kind] = struct{}{}
			out = append(out, kind)
		}
	}
	return normalizeInclude(out), nil
}

// normalizeInclude applies the include family's implications in canonical order:
// reasoning is a conversation view with thinking blocks, so it always brings
// transcript along, and spelling both is the same set as spelling one.
func normalizeInclude(in IncludeSet) IncludeSet {
	if !in.Has(IncludeReasoning) {
		return in
	}
	// Keep first-appearance order for every other token, then make the pair
	// canonical: transcript, reasoning.
	out := make(IncludeSet, 0, len(in)+1)
	hasTranscript := false
	for _, kind := range in {
		switch kind {
		case IncludeReasoning:
			continue
		case IncludeTranscript:
			hasTranscript = true
		}
		out = append(out, kind)
	}
	if !hasTranscript {
		out = append(out, IncludeTranscript)
	}
	return append(out, IncludeReasoning)
}

func (k IncludeKind) valid() bool {
	switch k {
	case IncludeUsage, IncludeDurations, IncludeBees, IncludeColony, IncludeCues, IncludeArtifacts, IncludeAgentLogs, IncludeTranscript, IncludeReasoning, IncludePatches, IncludeConfig, IncludeSkills:
		return true
	default:
		return false
	}
}
