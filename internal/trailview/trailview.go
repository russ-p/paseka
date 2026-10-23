// Package trailview is the shared trail collection read model.
//
// One walk over .paseka/runs produces every trail summary together with its
// captured configuration binding and fork lineage. Console, CLI, export,
// audit, and cleanup read this collection instead of resolving configuration,
// lineage, admission state, or pending actions independently — and none of
// them has to scan a Git checkout to answer what a trail is.
package trailview

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

// Admission state values for a runnable trail row. Pending fork requests are
// never trail rows: they are listed by Collection.PendingForkRequests instead.
const (
	// AdmissionOrdinary marks a non-experimental trail.
	AdmissionOrdinary = "ordinary"
	// AdmissionRunnable marks an admitted fork whose work may dispatch.
	AdmissionRunnable = "runnable"
	// AdmissionPinned marks a pinned trail whose lineage record is missing.
	AdmissionPinned = "pinned"
)

// Trail is one runnable trail with the evidence every surface needs.
type Trail struct {
	TraceID        string    `json:"traceId"`
	Title          string    `json:"title,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	LastActivityAt time.Time `json:"lastActivityAt"`
	RunCount       int       `json:"runCount"`
	TaskCount      int       `json:"taskCount"`
	EventCount     int       `json:"eventCount"`
	Bees           []string  `json:"bees,omitempty"`
	HasFailures    bool      `json:"hasFailures"`
	HasActive      bool      `json:"hasActive"`

	// Configuration binding. Config is the complete manifest; the flattened
	// fields keep list rows cheap to consume without dropping the history.
	ConfigBound        bool                  `json:"configBound"`
	ConfigRef          string                `json:"configRef,omitempty"`
	InitialConfigRef   string                `json:"initialConfigRef,omitempty"`
	ActiveConfigRef    string                `json:"activeConfigRef,omitempty"`
	ConfigRevision     int                   `json:"configRevision,omitempty"`
	CandidateConfigRef string                `json:"candidateConfigRef,omitempty"`
	Policy             string                `json:"policy,omitempty"`
	WorkspaceRoot      string                `json:"workspaceRoot,omitempty"`
	Config             *trailconfig.Manifest `json:"config,omitempty"`
	PendingConfig      *trailconfig.Change   `json:"pendingConfig,omitempty"`

	// Fork lineage, when this trail is an experiment.
	Lineage              *trailfork.Lineage `json:"lineage,omitempty"`
	ForkID               string             `json:"forkId,omitempty"`
	ForkOf               string             `json:"forkOf,omitempty"`
	SourceTask           string             `json:"sourceTask,omitempty"`
	SourceAttempt        string             `json:"sourceAttempt,omitempty"`
	SourceConfigRef      string             `json:"sourceConfigRef,omitempty"`
	SourceConfigRevision int                `json:"sourceConfigRevision,omitempty"`
	SourceBaseSHA        string             `json:"sourceBaseSha,omitempty"`
	BaseSHA              string             `json:"baseSha,omitempty"`
	BaseOverridden       bool               `json:"baseOverridden,omitempty"`
	Delta                *trailfork.Delta   `json:"delta,omitempty"`
	Budget               int                `json:"budget,omitempty"`
	Branch               string             `json:"branch,omitempty"`

	// AdmissionState is ordinary, runnable, or pinned. Pending fork requests
	// are counted separately and never become runnable trails.
	AdmissionState      string `json:"admissionState"`
	PendingForkRequests int    `json:"pendingForkRequests,omitempty"`

	// Evidence lists missing or inconsistent evidence honestly. An empty list
	// means the read model found no gap; it does not prove an external effect
	// happened.
	Evidence []string `json:"evidence,omitempty"`
}

// Collection is the whole trail read model: runnable trails plus the pending
// fork requests that are deliberately not trails yet.
type Collection struct {
	Trails              []Trail             `json:"trails"`
	PendingForkRequests []trailfork.Request `json:"pendingForkRequests"`
}

// Load scans the colony's run records once and returns the shared collection.
func Load(ctx colony.Context) (Collection, error) {
	if strings.TrimSpace(ctx.ColonyRoot) == "" {
		return Collection{}, fmt.Errorf("trailview: colony root is required")
	}
	summaries, err := runs.ScanAllTraces(ctx.ColonyRoot)
	if err != nil {
		return Collection{}, err
	}
	trails, pending, err := LoadSummaries(ctx, summaries)
	if err != nil {
		return Collection{}, err
	}
	return Collection{Trails: trails, PendingForkRequests: pending}, nil
}

// LoadSummaries enriches an already-scanned set of run summaries. Paged
// readers use it to avoid rescanning the whole colony while still sharing the
// same configuration, lineage, and pending-request rules as Load.
func LoadSummaries(ctx colony.Context, summaries []runs.TraceSummary) ([]Trail, []trailfork.Request, error) {
	if strings.TrimSpace(ctx.ColonyRoot) == "" {
		return nil, nil, fmt.Errorf("trailview: colony root is required")
	}
	pending, err := trailfork.ListPending(ctx.ColonyRoot)
	if err != nil {
		return nil, nil, err
	}
	bySource := pendingBySource(pending)
	trails := make([]Trail, 0, len(summaries))
	for _, summary := range summaries {
		trail := TrailFromSummary(ctx, summary, bySource[summary.TraceID])
		trails = append(trails, trail)
	}
	if pending == nil {
		pending = []trailfork.Request{}
	}
	return trails, pending, nil
}

// LoadTrail reads one trail's summary and evidence without returning the rest
// of the collection.
func LoadTrail(ctx colony.Context, traceID string) (Trail, bool, error) {
	if strings.TrimSpace(ctx.ColonyRoot) == "" {
		return Trail{}, false, fmt.Errorf("trailview: colony root is required")
	}
	summary, err := runs.LoadTraceSummary(ctx.ColonyRoot, traceID)
	if err != nil {
		return Trail{}, false, err
	}
	if summary.RunCount == 0 && summary.TaskCount == 0 && summary.EventCount == 0 {
		return Trail{}, false, nil
	}
	pending, err := trailfork.ListPending(ctx.ColonyRoot)
	if err != nil {
		return Trail{}, false, err
	}
	return TrailFromSummary(ctx, summary, pendingBySource(pending)[traceID]), true, nil
}

// TrailFromSummary enriches one raw run summary with configuration and fork
// evidence. Pending fork requests are passed only for the matching source.
func TrailFromSummary(ctx colony.Context, summary runs.TraceSummary, pending []trailfork.Request) Trail {
	trail := Trail{
		TraceID:             summary.TraceID,
		LastActivityAt:      summary.LastActivityAt,
		RunCount:            summary.RunCount,
		TaskCount:           summary.TaskCount,
		EventCount:          summary.EventCount,
		Bees:                append([]string(nil), summary.Bees...),
		HasFailures:         summary.HasFailures,
		HasActive:           summary.HasActive,
		AdmissionState:      AdmissionOrdinary,
		PendingForkRequests: len(pending),
	}
	if title, err := runs.ResolveTraceTitle(ctx.ColonyRoot, summary.TraceID); err == nil {
		trail.Title = title
	}
	if text, err := runs.ResolveTraceSummary(ctx.ColonyRoot, summary.TraceID); err == nil {
		trail.Summary = text
	}

	manifest, bound, err := trailconfig.LoadManifestIfExists(ctx.ColonyRoot, summary.TraceID)
	switch {
	case err != nil:
		trail.Evidence = append(trail.Evidence, fmt.Sprintf("configuration manifest is unreadable: %v", err))
	case bound:
		trail.applyConfig(manifest)
		trail.verifyConfig(ctx.ColonyRoot)
	default:
		trail.Evidence = append(trail.Evidence, "no configuration binding was captured (legacy trail); configuration evidence is unavailable")
	}

	lineage, err := trailfork.LoadLineage(ctx.ColonyRoot, summary.TraceID)
	switch {
	case err == nil:
		trail.applyLineage(lineage)
	case errors.Is(err, os.ErrNotExist):
		if bound && manifest.Policy == trailconfig.PolicyPinned {
			trail.AdmissionState = AdmissionPinned
			trail.Evidence = append(trail.Evidence, "pinned trail has no fork lineage record; source comparison is unavailable")
		}
	default:
		trail.Evidence = append(trail.Evidence, fmt.Sprintf("fork lineage record is unreadable: %v", err))
	}
	return trail
}

func (t *Trail) applyConfig(manifest trailconfig.Manifest) {
	t.ConfigBound = true
	t.Config = &manifest
	t.ConfigRef = manifest.ActiveConfigRef
	t.ActiveConfigRef = manifest.ActiveConfigRef
	t.InitialConfigRef = manifest.InitialConfigRef
	t.ConfigRevision = manifest.ConfigRevision
	t.Policy = manifest.Policy
	t.WorkspaceRoot = manifest.WorkspaceRoot
	t.PendingConfig = manifest.Pending
}

func (t *Trail) verifyConfig(colonyRoot string) {
	manifest := t.Config
	if manifest == nil {
		return
	}
	if len(manifest.Snapshots) == 0 {
		t.Evidence = append(t.Evidence, "configuration manifest retains no snapshots")
	}
	for _, snapshot := range manifest.Snapshots {
		if err := trailconfig.VerifySnapshot(colonyRoot, t.TraceID, snapshot.ConfigRef); err != nil {
			t.Evidence = append(t.Evidence,
				fmt.Sprintf("configuration %s is unavailable: %v", snapshot.ConfigRef, err))
		}
	}
	if !retainsRef(manifest, manifest.ActiveConfigRef) {
		t.Evidence = append(t.Evidence,
			fmt.Sprintf("active configuration %s is not in the retained snapshot inventory", manifest.ActiveConfigRef))
	}
}

func (t *Trail) applyLineage(lineage trailfork.Lineage) {
	t.Lineage = &lineage
	t.ForkID = lineage.ForkID
	t.ForkOf = lineage.SourceTrace
	t.SourceTask = lineage.SourceTask
	t.SourceAttempt = lineage.SourceAttempt
	t.SourceConfigRef = lineage.SourceConfigRef
	t.SourceConfigRevision = lineage.SourceConfigRevision
	t.SourceBaseSHA = lineage.SourceBaseSHA
	t.BaseSHA = lineage.BaseSHA
	t.BaseOverridden = lineage.BaseOverridden
	t.Delta = &lineage.Delta
	t.Budget = lineage.Budget
	t.Branch = lineage.Branch
	t.CandidateConfigRef = lineage.ConfigRef
	if t.Policy == "" {
		t.Policy = lineage.Policy
	}
	if strings.TrimSpace(lineage.State) != "" {
		t.AdmissionState = lineage.State
	} else {
		t.AdmissionState = AdmissionRunnable
	}
	if t.ConfigBound && lineage.ConfigRef != "" && lineage.ConfigRef != t.ActiveConfigRef {
		t.Evidence = append(t.Evidence,
			fmt.Sprintf("fork lineage candidate %s does not match active configuration %s", lineage.ConfigRef, t.ActiveConfigRef))
	}
	if !t.ConfigBound {
		t.Evidence = append(t.Evidence, "fork lineage exists but the fork has no configuration binding")
	}
}

// Forks returns runnable experimental trails, newest activity first.
func (c Collection) Forks() []Trail {
	out := make([]Trail, 0)
	for _, trail := range c.Trails {
		if trail.IsFork() {
			out = append(out, trail)
		}
	}
	return out
}

// ForksOf returns the runnable forks whose source is sourceTrace. Passing an
// empty source returns every fork.
func (c Collection) ForksOf(sourceTrace string) []Trail {
	sourceTrace = strings.TrimSpace(sourceTrace)
	out := make([]Trail, 0)
	for _, trail := range c.Forks() {
		if sourceTrace == "" || trail.ForkOf == sourceTrace {
			out = append(out, trail)
		}
	}
	return out
}

// Find returns one trail row by identity.
func (c Collection) Find(traceID string) (Trail, bool) {
	for _, trail := range c.Trails {
		if trail.TraceID == traceID {
			return trail, true
		}
	}
	return Trail{}, false
}

// PendingForks returns the pending requests that are deliberately not trails.
func (c Collection) PendingForks() []trailfork.Request {
	return append([]trailfork.Request(nil), c.PendingForkRequests...)
}

func (t Trail) IsFork() bool { return t.Lineage != nil || t.ForkOf != "" }

func pendingBySource(pending []trailfork.Request) map[string][]trailfork.Request {
	out := make(map[string][]trailfork.Request)
	for _, req := range pending {
		source := strings.TrimSpace(req.Evidence.SourceTrace)
		if source == "" {
			continue
		}
		out[source] = append(out[source], req)
	}
	return out
}

func retainsRef(manifest *trailconfig.Manifest, ref string) bool {
	for _, snapshot := range manifest.Snapshots {
		if snapshot.ConfigRef == ref {
			return true
		}
	}
	return false
}

// SortTrails orders rows by last activity, newest first, with the trace id as a
// total tie-break. Callers that need a different order can sort the copy.
func SortTrails(trails []Trail) {
	sort.SliceStable(trails, func(i, j int) bool {
		if !trails[i].LastActivityAt.Equal(trails[j].LastActivityAt) {
			return trails[i].LastActivityAt.After(trails[j].LastActivityAt)
		}
		return trails[i].TraceID < trails[j].TraceID
	})
}
