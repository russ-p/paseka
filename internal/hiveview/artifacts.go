package hiveview

import (
	"github.com/paseka/paseka/internal/artifacts"
	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runs"
)

// ArtifactView is one trail comb file for Console or export.
type ArtifactView struct {
	Ref          string `json:"ref"`
	ArtifactKind string `json:"artifactKind"`
	Title        string `json:"title,omitempty"`
	Updated      int64  `json:"updated,omitempty"`
	Producer     string `json:"producer,omitempty"`
	Announced    bool   `json:"announced"`
	Staged       bool   `json:"staged"`
}

// ArtifactContentView is file body for preview.
type ArtifactContentView struct {
	Ref         string `json:"ref"`
	Content     string `json:"content,omitempty"`
	ContentHTML string `json:"contentHtml,omitempty"`
	Omitted     string `json:"omitted,omitempty"`
}

// ListTraceArtifacts returns comb files with announced/staged labels.
func ListTraceArtifacts(ctx colony.Context, traceID string) ([]ArtifactView, error) {
	items, err := artifacts.ListItems(ctx.ColonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	events, err := runs.ReadTraceEvents(ctx.ColonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	merged := artifacts.MergeAnnounced(items, events)
	out := make([]ArtifactView, 0, len(merged))
	for _, item := range merged {
		out = append(out, ArtifactView{
			Ref:          item.Ref,
			ArtifactKind: item.ArtifactKind,
			Title:        item.Title,
			Updated:      item.Updated,
			Producer:     item.Producer,
			Announced:    item.Announced,
			Staged:       !item.Announced,
		})
	}
	return out, nil
}

// GetTraceArtifactContent returns raw text and optional HTML for a comb ref.
func GetTraceArtifactContent(ctx colony.Context, traceID, ref string) (ArtifactContentView, error) {
	data, err := artifacts.ReadContent(ctx.ColonyRoot, traceID, ref)
	if err != nil {
		return ArtifactContentView{}, err
	}
	canonical, err := artifacts.CanonicalRef(ctx.ColonyRoot, traceID, ref)
	if err != nil {
		return ArtifactContentView{}, err
	}
	view := ArtifactContentView{Ref: canonical}
	if !artifacts.IsTextContent(data) {
		view.Omitted = "binary or invalid UTF-8"
		return view, nil
	}
	if len(data) > artifacts.MaxInlineExportBytes {
		view.Omitted = "file too large for inline preview"
		return view, nil
	}
	view.Content = string(data)
	return view, nil
}
