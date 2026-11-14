package runs

import (
	"strings"
	"sync"
	"time"
)

// UsageCacheTTL is how long a UsageReader may answer from the report it already
// parsed. The Usage page is history rather than live telemetry, so a range or
// filter change that lands inside this window costs nothing, while a newly
// finished run still appears on an explicit refresh.
const UsageCacheTTL = 30 * time.Second

// UsageReader reads colony-wide usage reports and may reuse the report it just
// parsed for a short interval, so a history page does not walk every run
// directory on every read.
//
// The cache is in process only: nothing is written to disk, and a read still
// changes no file. A cached report carries the ObservedAt of the scan it came
// from, so a caller can see how old the answer is rather than assuming it is
// current. Read is read-only; the report it returns is shared with later cache
// hits and must not be mutated.
type UsageReader struct {
	// ColonyRoot is the repository whose .paseka/runs directory is read.
	ColonyRoot string
	// TTL bounds a cached report's age. Zero or negative disables caching, so
	// every read scans again; a test uses that to exercise the uncached path.
	TTL time.Duration
	// Now is the clock both the range default and cache expiry are measured
	// against. Nil uses time.Now().UTC().
	Now func() time.Time

	// mu serializes readers. A second read that lands while a scan is running
	// waits for that scan and then finds its report, rather than walking the
	// same directories twice; the alternative would hand two readers of one
	// query different ObservedAt values.
	mu    sync.Mutex
	cache map[usageCacheKey]usageCacheEntry
}

// NewUsageReader builds a reader for one colony with the default cache window.
func NewUsageReader(colonyRoot string) *UsageReader {
	return &UsageReader{ColonyRoot: colonyRoot, TTL: UsageCacheTTL}
}

// usageCacheKey is one query's identity: its range, its zone, and its filter
// set. Two requests that differ in any of those never share a report, because a
// report is only reusable for the exact view that produced it.
type usageCacheKey struct {
	since    string
	until    string
	timezone string
	adapters string
	models   string
	bees     string
	purposes string
}

type usageCacheEntry struct {
	report    UsageReport
	scannedAt time.Time
}

// Read answers one query, reusing a cached report only when the same query was
// scanned inside the cache window and refresh is false.
//
// refresh is the explicit path behind the page's refresh control: it bypasses
// the cache, scans again, and returns a report with a new ObservedAt. A query
// that does not match a cached key never reuses an entry, so a different range,
// zone, or filter set is always read from disk.
func (r *UsageReader) Read(q UsageQuery, refresh bool) (UsageReport, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := q.Now
	if now.IsZero() {
		now = r.now()
	}
	key := usageCacheKeyFor(q)

	if !refresh {
		if entry, ok := r.cache[key]; ok && r.fresh(entry, now) {
			return entry.report, nil
		}
	}

	q.Now = now
	report, err := ReadUsage(r.ColonyRoot, q)
	if err != nil {
		return UsageReport{}, err
	}
	r.store(key, report, now)
	return report, nil
}

func (r *UsageReader) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// fresh reports whether a cached scan may answer at now. A scan stamped in the
// future is not fresh: a clock that moved backwards invalidates the entry
// rather than extending its life.
func (r *UsageReader) fresh(entry usageCacheEntry, now time.Time) bool {
	if r.TTL <= 0 {
		return false
	}
	age := now.Sub(entry.scannedAt)
	return age >= 0 && age < r.TTL
}

// store keeps the scan for the next read of the same query and drops entries
// that are already stale, so browsing many ranges does not grow the map without
// bound.
func (r *UsageReader) store(key usageCacheKey, report UsageReport, now time.Time) {
	if r.TTL <= 0 {
		return
	}
	if r.cache == nil {
		r.cache = make(map[usageCacheKey]usageCacheEntry)
	}
	for existing, entry := range r.cache {
		if !r.fresh(entry, now) {
			delete(r.cache, existing)
		}
	}
	r.cache[key] = usageCacheEntry{report: report, scannedAt: report.ObservedAt}
}

// usageCacheKeyFor reduces a query to the identity a cached report is reusable
// under. Filters are normalized first, so two requests that name the same set
// in a different order are one query rather than two.
func usageCacheKeyFor(q UsageQuery) usageCacheKey {
	filters := normalizeUsageFilters(q)
	return usageCacheKey{
		since:    strings.TrimSpace(q.Since),
		until:    strings.TrimSpace(q.Until),
		timezone: strings.TrimSpace(q.Timezone),
		adapters: strings.Join(filters.Adapter, "\x00"),
		models:   strings.Join(filters.Model, "\x00"),
		bees:     strings.Join(filters.Bee, "\x00"),
		purposes: strings.Join(filters.Purpose, "\x00"),
	}
}
