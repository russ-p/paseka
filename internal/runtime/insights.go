package runtime

import (
	"strings"

	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

// GatherPromptInsights loads prior narrative INSIGHT events for a trace and projects them into prompt strings.
func GatherPromptInsights(colonyRoot, traceID, taskID string) ([]string, error) {
	events, err := runs.ReadTraceEvents(colonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	return protocol.ProjectInsights(events, protocol.DefaultInsightProjectionOptions(taskID)), nil
}

// MergeInsights combines manually supplied insights with projected ones, preserving order and deduplicating.
func MergeInsights(manual, projected []string) []string {
	seen := make(map[string]struct{})
	var out []string
	appendUnique := func(items []string) {
		for _, s := range items {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	appendUnique(manual)
	appendUnique(projected)
	return out
}
