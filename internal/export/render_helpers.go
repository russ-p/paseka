package export

import (
	"fmt"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

func formatUsageAggregate(u *runs.UsageAggregate) string {
	if u == nil || u.RunCountWithUsage == 0 {
		return ""
	}
	parts := []string{
		formatUsageHead(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens),
		fmt.Sprintf("%d runs with usage", u.RunCountWithUsage),
	}
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
		formatUsageHead(u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens),
	}
	if u.DurationMs > 0 {
		parts = append(parts, fmt.Sprintf("adapter %s", formatDuration(time.Duration(u.DurationMs)*time.Millisecond)))
	}
	return strings.Join(parts, ", ")
}

func formatUsageHead(in, out, cacheRead, cacheWrite int64) string {
	segments := []string{fmt.Sprintf("%d / %d", in, out)}
	if cacheRead > 0 {
		segments = append(segments, fmt.Sprintf("cache read %d", cacheRead))
	}
	if cacheWrite > 0 {
		segments = append(segments, fmt.Sprintf("cache write %d", cacheWrite))
	}
	if in < 1000 && out < 1000 {
		segments[0] = fmt.Sprintf("in %d / out %d", in, out)
		return strings.Join(segments, ", ")
	}
	return fmt.Sprintf("in %s / out %s (%s)", compactTokenCount(in), compactTokenCount(out), strings.Join(segments, ", "))
}

func compactTokenCount(n int64) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	k := (n + 500) / 1000
	if k >= 100 {
		k = ((k + 5) / 10) * 10
	}
	return fmt.Sprintf("%dk", k)
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
