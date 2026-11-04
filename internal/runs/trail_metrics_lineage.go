package runs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Spec 042 owns the fork lineage and configuration binding records. The metrics
// reader depends only on their retained JSON shape: importing the owning
// packages would invert the dependency (they read run records from here) and a
// metrics read must work from disk alone.
const (
	metricsForkLineageFileName = "fork.json"
	metricsConfigDirName       = "config"
	metricsConfigManifestName  = "manifest.json"
	metricsConfigPolicyPinned  = "pinned"
)

// metricsConfigManifest is the subset of trailconfig's manifest.json the
// projection consumes.
type metricsConfigManifest struct {
	Version          int    `json:"version"`
	InitialConfigRef string `json:"initialConfigRef"`
	ActiveConfigRef  string `json:"activeConfigRef"`
	ConfigRevision   int    `json:"configRevision"`
	Policy           string `json:"policy"`
}

// metricsForkRecord is the subset of trailfork's fork.json the projection
// consumes. SourceConfigRevision is read from the record's `sourceRevision`:
// it is the selected source attempt's recorded configuration revision, not the
// source trail's current one.
type metricsForkRecord struct {
	ForkID               string `json:"forkId"`
	TraceID              string `json:"traceId"`
	SourceTrace          string `json:"sourceTrace"`
	SourceTask           string `json:"sourceTask"`
	SourceAttempt        string `json:"sourceAttempt"`
	SourceRun            string `json:"sourceRun"`
	SourceConfigRef      string `json:"sourceConfigRef"`
	SourceConfigRevision int    `json:"sourceRevision"`
	State                string `json:"state"`
	Delta                struct {
		Form string `json:"form"`
	} `json:"delta"`
}

func metricsForkLineagePath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, metricsForkLineageFileName)
}

func metricsConfigManifestPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, metricsConfigDirName, metricsConfigManifestName)
}

// readMetricsConfigManifest loads the retained binding. A missing record is not
// an error: it is a legacy or unbound trail, and the caller reports that rather
// than substituting today's colony configuration.
func readMetricsConfigManifest(colonyRoot, traceID string) (metricsConfigManifest, bool, error) {
	data, err := os.ReadFile(metricsConfigManifestPath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return metricsConfigManifest{}, false, nil
		}
		return metricsConfigManifest{}, false, fmt.Errorf("runs: read configuration manifest: %w", err)
	}
	var manifest metricsConfigManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return metricsConfigManifest{}, false, fmt.Errorf("runs: parse configuration manifest: %w", err)
	}
	return manifest, true, nil
}

// readMetricsForkRecord loads the retained fork lineage. A missing record is
// not an error; the caller decides whether that is a known ordinary trail or
// unavailable lineage.
func readMetricsForkRecord(colonyRoot, traceID string) (metricsForkRecord, bool, error) {
	data, err := os.ReadFile(metricsForkLineagePath(colonyRoot, traceID))
	if err != nil {
		if os.IsNotExist(err) {
			return metricsForkRecord{}, false, nil
		}
		return metricsForkRecord{}, false, fmt.Errorf("runs: read fork lineage: %w", err)
	}
	var record metricsForkRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return metricsForkRecord{}, false, fmt.Errorf("runs: parse fork lineage: %w", err)
	}
	return record, true, nil
}

// applyLineageEvidence fills forkOf from the fork's own retained record.
//
// The source trail is never consulted: a fork names the source trace, task,
// attempt/run, and configuration it inherited at admission. A later change to
// the source's active configuration or task state must not rewrite that
// evidence.
func applyLineageEvidence(m *TrailMetrics, colonyRoot, traceID string, origin TrailOrigin, originOK bool, originErr error, manifest metricsConfigManifest, manifestOK bool, manifestErr error) {
	record, recordOK, recordErr := readMetricsForkRecord(colonyRoot, traceID)
	switch {
	case recordErr != nil:
		m.warn("lineage", WarningUnreadable, fmt.Sprintf("fork lineage record: %v", recordErr))
		m.Coverage.Lineage = CoverageUnavailable
		return
	case recordOK:
		forkOf := &MetricsForkOf{
			SourceTrace:          record.SourceTrace,
			SourceTask:           record.SourceTask,
			SourceAttempt:        record.SourceAttempt,
			SourceRun:            record.SourceRun,
			SourceConfigRef:      record.SourceConfigRef,
			SourceConfigRevision: record.SourceConfigRevision,
			ForkID:               record.ForkID,
			Delta:                record.Delta.Form,
			State:                record.State,
		}
		m.ForkOf = forkOf
		m.Coverage.Lineage = CoverageComplete
		if missing := missingForkLineageFields(record); len(missing) > 0 {
			m.warn("lineage", WarningMissing,
				fmt.Sprintf("fork lineage record is missing %s", strings.Join(missing, ", ")))
			m.Coverage.Lineage = CoveragePartial
		}
		if recorded := strings.TrimSpace(record.TraceID); recorded != "" && recorded != traceID {
			m.warn("lineage", WarningConflicting,
				fmt.Sprintf("fork lineage record belongs to trace %s, not %s", recorded, traceID))
			m.Coverage.Lineage = CoveragePartial
		}
		if originOK && origin.Kind != OriginKindFork {
			m.warn("lineage", WarningConflicting,
				fmt.Sprintf("fork lineage record exists but the trail origin is %q", origin.Kind))
			m.Coverage.Lineage = CoveragePartial
		}
		return
	}

	// No lineage record. Distinguish a trail that is known ordinary from one
	// whose lineage provenance is unavailable.
	switch {
	case originErr != nil:
		m.warn("lineage", WarningUnreadable, fmt.Sprintf("trail origin record: %v", originErr))
		m.Coverage.Lineage = CoverageUnavailable
	case originOK && origin.Kind == OriginKindFork:
		m.warn("lineage", WarningMissing,
			"trail origin is a fork but the retained fork lineage record is absent")
		m.Coverage.Lineage = CoverageUnavailable
	case originOK:
		m.Coverage.Lineage = CoverageAbsent
	case manifestErr != nil:
		m.warn("lineage", WarningUnreadable, fmt.Sprintf("configuration manifest: %v", manifestErr))
		m.Coverage.Lineage = CoverageUnavailable
	case manifestOK && strings.TrimSpace(manifest.Policy) == metricsConfigPolicyPinned:
		m.warn("lineage", WarningMissing,
			"pinned trail has no fork lineage record; source lineage is unavailable")
		m.Coverage.Lineage = CoverageUnavailable
	case manifestOK:
		// A readable non-pinned binding proves the trail was created ordinary.
		m.Coverage.Lineage = CoverageAbsent
	default:
		m.warn("lineage", WarningLegacy,
			"no trail origin or fork lineage record; fork lineage is unknown")
		m.Coverage.Lineage = CoverageUnavailable
	}
}

func missingForkLineageFields(record metricsForkRecord) []string {
	var missing []string
	if strings.TrimSpace(record.SourceTrace) == "" {
		missing = append(missing, "sourceTrace")
	}
	if strings.TrimSpace(record.SourceTask) == "" {
		missing = append(missing, "sourceTask")
	}
	if strings.TrimSpace(record.SourceAttempt) == "" {
		missing = append(missing, "sourceAttempt")
	}
	if strings.TrimSpace(record.SourceConfigRef) == "" {
		missing = append(missing, "sourceConfigRef")
	}
	return missing
}

// applyConfigurationEvidence fills the configuration block from the retained
// binding and the references recorded by retained executions.
//
// Initial and active references come from the binding record. Used references
// come only from execution records: a manifest's active reference is what
// future work would run, not proof that any execution used it. A legacy run
// without a recorded reference stays unknown and is never backfilled from the
// manifest or from today's colony defaults.
func applyConfigurationEvidence(m *TrailMetrics, manifest metricsConfigManifest, manifestOK bool, manifestErr error, metas []RunMeta, runUnreadable bool) {
	type useAggregate struct {
		runs      int
		revisions map[int]struct{}
	}
	used := map[string]*useAggregate{}
	unknownRuns := 0
	for _, meta := range metas {
		ref := strings.TrimSpace(meta.ConfigRef)
		if ref == "" {
			unknownRuns++
			continue
		}
		agg := used[ref]
		if agg == nil {
			agg = &useAggregate{revisions: map[int]struct{}{}}
			used[ref] = agg
		}
		agg.runs++
		if meta.ConfigRevision > 0 {
			agg.revisions[meta.ConfigRevision] = struct{}{}
		}
	}

	usedEntries := make([]MetricsConfigUse, 0, len(used))
	versions := map[string]struct{}{}
	for ref, agg := range used {
		usedEntries = append(usedEntries, MetricsConfigUse{
			ConfigRef:      ref,
			ConfigRevision: singleRevision(agg.revisions),
			Runs:           agg.runs,
		})
		versions[ref] = struct{}{}
	}
	sort.SliceStable(usedEntries, func(i, j int) bool {
		return usedEntries[i].ConfigRef < usedEntries[j].ConfigRef
	})

	if manifestOK {
		if ref := strings.TrimSpace(manifest.InitialConfigRef); ref != "" {
			versions[ref] = struct{}{}
		}
		if ref := strings.TrimSpace(manifest.ActiveConfigRef); ref != "" {
			versions[ref] = struct{}{}
		}
	}
	// When the binding cannot be read, recorded execution references are still
	// real evidence, so they are reported without inventing initial/active ones.

	multipleVersions := (manifestOK && manifest.ConfigRevision > 1) || len(versions) > 1

	if !manifestOK && manifestErr == nil && len(usedEntries) == 0 {
		// No binding and no execution reference: the trail retains no
		// configuration provenance at all, so the block stays null.
		m.Coverage.Configuration = CoverageUnavailable
		m.warn("configuration", WarningMissing,
			"no configuration binding record; configuration provenance is unavailable")
		return
	}

	block := &MetricsConfiguration{
		Used:             usedEntries,
		UnknownRuns:      unknownRuns,
		MultipleVersions: multipleVersions,
	}
	if manifestOK {
		block.InitialConfigRef = strings.TrimSpace(manifest.InitialConfigRef)
		block.ActiveConfigRef = strings.TrimSpace(manifest.ActiveConfigRef)
		block.ConfigRevision = manifest.ConfigRevision
		block.Policy = strings.TrimSpace(manifest.Policy)
	}
	m.Configuration = block

	if manifestErr != nil {
		m.Coverage.Configuration = CoverageUnavailable
		m.warn("configuration", WarningUnreadable,
			fmt.Sprintf("configuration manifest: %v", manifestErr))
	} else if !manifestOK {
		m.Coverage.Configuration = CoveragePartial
		m.warn("configuration", WarningMissing,
			"no configuration binding record; initial and active references are unknown")
	} else if block.InitialConfigRef == "" && block.ActiveConfigRef == "" {
		m.Coverage.Configuration = CoveragePartial
		m.warn("configuration", WarningMissing,
			"configuration manifest records no initial or active reference")
	} else if unknownRuns > 0 || runUnreadable {
		m.Coverage.Configuration = CoveragePartial
		if unknownRuns > 0 {
			m.warn("configuration", WarningLegacy,
				fmt.Sprintf("%d retained execution(s) recorded no configuration reference; their configuration version is unknown", unknownRuns))
		}
		if runUnreadable {
			m.warn("configuration", WarningUnreadable,
				"some run records are unreadable; execution configuration usage may be incomplete")
		}
	} else {
		m.Coverage.Configuration = CoverageComplete
	}
}

// singleRevision returns the revision shared by every recorded execution of one
// reference, or zero when the records disagree or never recorded one. A guessed
// revision would claim provenance the record does not support.
func singleRevision(revisions map[int]struct{}) int {
	if len(revisions) != 1 {
		return 0
	}
	for revision := range revisions {
		return revision
	}
	return 0
}
