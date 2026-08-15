package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/protocol"
	"github.com/paseka/paseka/internal/runs"
)

func formatUsageAggregate(u *runs.UsageAggregate) string {
	if u == nil || u.RunCountWithUsage == 0 {
		return ""
	}
	parts := []string{
		fmt.Sprintf("in %d / out %d", u.InputTokens, u.OutputTokens),
	}
	if u.CacheReadTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache read %d", u.CacheReadTokens))
	}
	if u.CacheWriteTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache write %d", u.CacheWriteTokens))
	}
	parts = append(parts, fmt.Sprintf("%d runs with usage", u.RunCountWithUsage))
	return strings.Join(parts, ", ")
}

func formatUsageTokens(u *protocol.Usage) string {
	if u == nil {
		return ""
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 && u.DurationMs == 0 {
		return ""
	}
	parts := []string{
		fmt.Sprintf("in %d / out %d", u.InputTokens, u.OutputTokens),
	}
	if u.CacheReadTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache read %d", u.CacheReadTokens))
	}
	if u.CacheWriteTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache write %d", u.CacheWriteTokens))
	}
	if u.DurationMs > 0 {
		parts = append(parts, fmt.Sprintf("adapter %s", formatDuration(time.Duration(u.DurationMs)*time.Millisecond)))
	}
	return strings.Join(parts, ", ")
}

func formatWallDuration(run hiveview.RunView) string {
	if run.FinishedAt == nil || run.StartedAt.IsZero() {
		return ""
	}
	return formatDuration(run.FinishedAt.Sub(run.StartedAt))
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d / time.Minute)
	seconds := int((d % time.Minute) / time.Second)
	if minutes < 60 {
		if seconds == 0 {
			return fmt.Sprintf("%dm", minutes)
		}
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	hours := minutes / 60
	minutes = minutes % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}
