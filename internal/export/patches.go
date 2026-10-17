package export

import (
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/patches"
)

const (
	// maxInlinePatchBytes caps one patch body inlined into a report.
	//
	// Separate from the comb caps on purpose: a proposal patch is captured diff history,
	// not something a Bee handed off, and its useful size is set by what a change looked
	// like rather than by what a phone-sized preview page can hold.
	maxInlinePatchBytes = 512 * 1024
	// maxAggregateInlinePatchBytes caps the raw inlined patch-body bytes across one report.
	//
	// Escaping and report metadata still add on top of this, so the finished report can be
	// larger. It bounds body bytes, not the whole file.
	maxAggregateInlinePatchBytes = 8 << 20
)

// PatchExport is one proposal patch record for trace export.
//
// An export with omitted bodies is a history report, not a complete portable archive: a
// local ref does not make an omitted body available offline. Every entry therefore carries
// its metadata, its ref, and its size whether or not the body made it in.
type PatchExport struct {
	Ref          string    `json:"ref"`
	TraceID      string    `json:"traceId"`
	AgentID      string    `json:"agentId"`
	Bee          string    `json:"bee,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	TaskID       string    `json:"taskId,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	Workspace    string    `json:"workspace,omitempty"`
	WorktreePath string    `json:"worktreePath,omitempty"`
	BaseSha      string    `json:"baseSha,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	Sector       string    `json:"sector,omitempty"`
	Bytes        int64     `json:"bytes"`
	FileCount    *int      `json:"fileCount,omitempty"`
	CapturedAt   time.Time `json:"capturedAt"`
	Publication  string    `json:"publication"`
	PublishError string    `json:"publishError,omitempty"`
	Content      string    `json:"content,omitempty"`
	Omitted      string    `json:"omitted,omitempty"`
}

// PatchDiagnosticExport explains a record on disk that is not an ordinary listing entry.
// It rides beside the entries rather than inside them so one bad file cannot hide the
// records that are fine.
type PatchDiagnosticExport struct {
	AgentID string `json:"agentId"`
	Reason  string `json:"reason"`
}

// loadPatchExports reads one trail's proposal archive. It is filesystem-only and needs no
// bus: a purged bus removes the events, not the records.
//
// Bodies are assigned newest-first so the most recent attempts are the ones an operator is
// most likely to open, with agent id as the stable tie-breaker. Caps are applied while
// reading, so an oversized body is never loaded whole to discover its size.
func loadPatchExports(colonyRoot, traceID string) ([]PatchExport, []PatchDiagnosticExport, error) {
	store := patches.NewStore(colonyRoot, traceID)
	listed, err := store.List()
	if err != nil {
		return nil, nil, err
	}

	out := make([]PatchExport, 0, len(listed.Records))
	for _, rec := range listed.Records {
		out = append(out, PatchExport{
			Ref:          store.Ref(rec.AgentID),
			TraceID:      rec.TraceID,
			AgentID:      rec.AgentID,
			Bee:          rec.Bee,
			Mode:         rec.Mode,
			TaskID:       rec.TaskID,
			Kind:         rec.Kind,
			Workspace:    rec.Workspace,
			WorktreePath: rec.WorktreePath,
			BaseSha:      rec.BaseSha,
			Summary:      rec.Summary,
			Sector:       rec.Sector,
			Bytes:        rec.Bytes,
			FileCount:    rec.FileCount,
			CapturedAt:   rec.CapturedAt,
			Publication:  rec.Publication,
			PublishError: rec.PublishError,
		})
	}

	diagnostics := make([]PatchDiagnosticExport, 0, len(listed.Diagnostics))
	for _, d := range listed.Diagnostics {
		diagnostics = append(diagnostics, PatchDiagnosticExport{AgentID: d.AgentID, Reason: d.Reason})
	}

	assignPatchBodies(store, out)
	return out, diagnostics, nil
}

// assignPatchBodies fills bodies newest-first within the per-patch and aggregate budgets.
//
// The caps are applied from the recorded size before any read, and the read that follows is
// bounded by what is left of the aggregate, so an oversized body is never loaded whole and
// a report cannot grow without bound.
func assignPatchBodies(store patches.Store, entries []PatchExport) {
	remaining := int64(maxAggregateInlinePatchBytes)
	for i := len(entries) - 1; i >= 0; i-- {
		entry := &entries[i]
		if entry.Bytes > maxInlinePatchBytes {
			entry.Omitted = fmt.Sprintf("patch too large for inline export (%d bytes)", entry.Bytes)
			continue
		}
		if entry.Bytes > remaining {
			entry.Omitted = fmt.Sprintf("aggregate inline patch budget of %d bytes is exhausted", int64(maxAggregateInlinePatchBytes))
			continue
		}
		body, err := store.ReadContent(entry.AgentID, remaining)
		if err != nil {
			entry.Omitted = err.Error()
			continue
		}
		if body.Omitted != "" {
			entry.Omitted = body.Omitted
			continue
		}
		if body.Truncated {
			// The file grew past the recorded size between the list and the read, so this
			// is a prefix rather than the retained bytes.
			entry.Omitted = "patch changed while the report was being built"
			continue
		}
		entry.Content = string(body.Body)
		remaining -= int64(len(body.Body))
	}
}
