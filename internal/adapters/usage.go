package adapters

import "github.com/russ-p/paseka/internal/protocol"

// AddUsage folds next into sum, allocating on first use. A nil next is ignored,
// so callers can add optional provider usage without branching.
func AddUsage(sum, next *protocol.Usage) *protocol.Usage {
	if next == nil {
		return sum
	}
	if sum == nil {
		cp := *next
		return &cp
	}
	sum.InputTokens += next.InputTokens
	sum.OutputTokens += next.OutputTokens
	sum.CacheReadTokens += next.CacheReadTokens
	sum.CacheWriteTokens += next.CacheWriteTokens
	sum.DurationMs += next.DurationMs
	if sum.Source == "" {
		sum.Source = next.Source
	}
	return sum
}
