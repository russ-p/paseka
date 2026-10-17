package hiveview

import (
	"fmt"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/patches"
)

// ProposalPatchView is one archived proposal record.
//
// The archive is what the diff collector returned when a run finished. It is not a strict
// per-run delta, not a workspace backup, and not proof of the code a Guard approved — the
// list says Publication, not Approved, for exactly that reason.
type ProposalPatchView struct {
	Ref          string `json:"ref"`
	TraceID      string `json:"traceId"`
	AgentID      string `json:"agentId"`
	Bee          string `json:"bee,omitempty"`
	Mode         string `json:"mode,omitempty"`
	TaskID       string `json:"taskId,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Workspace    string `json:"workspace,omitempty"`
	WorktreePath string `json:"worktreePath,omitempty"`
	BaseSha      string `json:"baseSha,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Sector       string `json:"sector,omitempty"`
	CapturedAt   string `json:"capturedAt"`
	// Bytes is the recorded body size, so a patch the preview will refuse is at least
	// legible before anybody opens it.
	Bytes int64 `json:"bytes"`
	// FileCount is best effort: a diff the collector reformatted may not carry every file
	// header, in which case it is nil rather than zero.
	FileCount    *int   `json:"fileCount,omitempty"`
	Publication  string `json:"publication"`
	PublishError string `json:"publishError,omitempty"`
	HasBody      bool   `json:"hasBody"`
}

// ProposalPatchDiagnostic explains a record on disk that is not an ordinary listing entry.
type ProposalPatchDiagnostic struct {
	AgentID string `json:"agentId"`
	Reason  string `json:"reason"`
}

// ProposalPatchContentView is a bounded read of a patch body plus why bytes were withheld.
type ProposalPatchContentView struct {
	Ref     string `json:"ref"`
	AgentID string `json:"agentId"`
	Content string `json:"content,omitempty"`
	// Truncated is set when Content is a strict prefix of the retained body. A truncated
	// preview is never presented as the original patch.
	Truncated   bool   `json:"truncated,omitempty"`
	Omitted     string `json:"omitted,omitempty"`
	Bytes       int64  `json:"bytes"`
	Publication string `json:"publication,omitempty"`
}

// ListTraceProposalPatches returns every complete proposal record for a trail.
//
// A trail with no patch directory — one that predates the archive, or one where nothing was
// proposed — reads as an empty list, not an error. Diagnostics ride beside the list so an
// incomplete record never hides the complete ones.
func ListTraceProposalPatches(ctx colony.Context, traceID string) ([]ProposalPatchView, []ProposalPatchDiagnostic, error) {
	store := patches.NewStore(ctx.ColonyRoot, traceID)
	listed, err := store.List()
	if err != nil {
		return nil, nil, err
	}
	out := make([]ProposalPatchView, 0, len(listed.Records))
	for _, rec := range listed.Records {
		out = append(out, ProposalPatchView{
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
			CapturedAt:   rec.CapturedAt.UTC().Format(time.RFC3339),
			Bytes:        rec.Bytes,
			FileCount:    rec.FileCount,
			Publication:  rec.Publication,
			PublishError: rec.PublishError,
			HasBody:      true,
		})
	}
	diagnostics := make([]ProposalPatchDiagnostic, 0, len(listed.Diagnostics))
	for _, d := range listed.Diagnostics {
		diagnostics = append(diagnostics, ProposalPatchDiagnostic{AgentID: d.AgentID, Reason: d.Reason})
	}
	return out, diagnostics, nil
}

// GetTraceProposalPatchContent returns a bounded preview of one archived patch body.
func GetTraceProposalPatchContent(ctx colony.Context, traceID, agentID string) (ProposalPatchContentView, error) {
	store := patches.NewStore(ctx.ColonyRoot, traceID)
	rec, ok, err := store.Read(agentID)
	if err != nil {
		return ProposalPatchContentView{}, err
	}
	if !ok {
		return ProposalPatchContentView{}, fmt.Errorf("proposal patch %q not found for trace %q", agentID, traceID)
	}
	view := ProposalPatchContentView{
		Ref:         store.Ref(rec.AgentID),
		AgentID:     rec.AgentID,
		Publication: rec.Publication,
	}
	body, err := store.ReadContent(rec.AgentID, patches.MaxPreviewBytes)
	if err != nil {
		return view, err
	}
	view.Bytes = body.Bytes
	view.Truncated = body.Truncated
	view.Omitted = body.Omitted
	view.Content = string(body.Body)
	return view, nil
}
