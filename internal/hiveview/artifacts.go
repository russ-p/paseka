package hiveview

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/artifacts"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
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
	// Bytes is the file's size, so a file the preview will refuse is at least legible
	// before anybody opens it. For a folded proposal bundle it is the bundle's total.
	Bytes int64 `json:"bytes,omitempty"`
	// Skill and Files describe a folded skill-proposal bundle: one row for the
	// directory the Archivist proposed, not one row for each file in it. Skill is
	// empty for an ordinary comb file.
	Skill string `json:"skill,omitempty"`
	Files int    `json:"files,omitempty"`
}

// ArtifactContentView is file body for preview.
type ArtifactContentView struct {
	Ref         string `json:"ref"`
	Content     string `json:"content,omitempty"`
	ContentHTML string `json:"contentHtml,omitempty"`
	Omitted     string `json:"omitted,omitempty"`
	// Bytes rides with the body so an omitted body can say how big it was, rather than
	// leaving "too large" as the whole of what the operator knows.
	Bytes int64 `json:"bytes,omitempty"`
}

// ProposalBundleKind is the artifact kind of a collapsed skill-proposal row. A
// proposal is one thing a Beekeeper decides about, and a bundle is a directory,
// so the comb listing names it once instead of once per file.
const ProposalBundleKind = "skill-proposal"

// ListTraceArtifacts returns comb files with announced/staged labels.
//
// A skill proposal's bundle is folded into one row: its `INSIGHT/skill.proposed`
// discovery is what announces it, so a listing that showed its files separately
// would present one decision as N handoff files, none of which carries the
// approval. A bundle no discovery names — the leftover of a run that failed its
// completion contract — keeps its files as ordinary rows, because that is the
// truth about it.
func ListTraceArtifacts(ctx colony.Context, traceID string) ([]ArtifactView, error) {
	items, err := artifacts.ListItems(ctx.ColonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	events, err := runs.ReadTraceEvents(ctx.ColonyRoot, traceID)
	if err != nil {
		return nil, err
	}
	bundles := proposalBundles(events)

	merged := artifacts.MergeAnnounced(items, events)
	out := make([]ArtifactView, 0, len(merged))
	folded := make(map[string]*ArtifactView, len(bundles))
	var foldedOrder []string
	for _, item := range merged {
		combRel, inComb := combRelativeRef(traceID, item.Ref)
		bundle, isBundle := proposalBundle{}, false
		if inComb {
			bundle, isBundle = bundleFor(combRel, bundles)
		}
		if !isBundle {
			out = append(out, artifactView(item))
			continue
		}
		row, seen := folded[bundle.dir]
		if !seen {
			row = &ArtifactView{
				Ref:          combDirRef(traceID, bundle.dir),
				ArtifactKind: ProposalBundleKind,
				Title:        bundle.skill,
				Producer:     bundle.producer,
				Skill:        bundle.skill,
				// The discovery is the announcement. The runtime also flushes the
				// bundle's bytes through `artifact.written`, but the fact an operator
				// acts on is that a proposal was made.
				Announced: true,
			}
			folded[bundle.dir] = row
			foldedOrder = append(foldedOrder, bundle.dir)
		}
		row.Files++
		row.Bytes += item.Bytes
		if item.Updated > row.Updated {
			row.Updated = item.Updated
		}
	}
	for _, dir := range foldedOrder {
		out = append(out, *folded[dir])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

func artifactView(item artifacts.Item) ArtifactView {
	return ArtifactView{
		Ref:          item.Ref,
		ArtifactKind: item.ArtifactKind,
		Title:        item.Title,
		Updated:      item.Updated,
		Producer:     item.Producer,
		Announced:    item.Announced,
		Staged:       !item.Announced,
		Bytes:        item.Bytes,
	}
}

// proposalBundle is one bundle a `skill.proposed` discovery names.
type proposalBundle struct {
	skill    string
	dir      string // comb-relative, slash-separated
	producer string // the run that proposed it
}

// proposalBundles reads the trail's proposal discoveries. The bundle reference
// is normalized by the same rule the payload validator applies, so a record the
// listing cannot trust is simply not a bundle it folds.
func proposalBundles(events []protocol.Event) []proposalBundle {
	var out []proposalBundle
	for _, event := range events {
		if event.Type != protocol.EventInsight || protocol.PayloadKind(event.Payload) != string(protocol.InsightSkillProposed) {
			continue
		}
		var payload protocol.SkillProposedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			continue
		}
		dir, ok := protocol.CombRelativeBundle(payload.Bundle)
		if !ok {
			continue
		}
		out = append(out, proposalBundle{
			skill:    strings.TrimSpace(payload.Name),
			dir:      dir,
			producer: event.AgentID,
		})
	}
	return out
}

// bundleFor names the bundle a comb-relative path belongs to. The longest
// matching directory wins, so one bundle nested inside another cannot claim the
// other's files.
func bundleFor(combRel string, bundles []proposalBundle) (proposalBundle, bool) {
	best, found := proposalBundle{}, false
	for _, bundle := range bundles {
		if combRel != bundle.dir && !strings.HasPrefix(combRel, bundle.dir+"/") {
			continue
		}
		if !found || len(bundle.dir) > len(best.dir) {
			best, found = bundle, true
		}
	}
	return best, found
}

// combRelativeRef strips the trail comb prefix from a canonical item ref.
func combRelativeRef(traceID, ref string) (string, bool) {
	prefix := combDirPrefix(traceID)
	norm := filepath.ToSlash(strings.TrimSpace(ref))
	if !strings.HasPrefix(norm, prefix) {
		return "", false
	}
	rel := strings.TrimPrefix(norm, prefix)
	return rel, rel != ""
}

func combDirPrefix(traceID string) string {
	return ".paseka/runs/" + traceID + "/artifacts/"
}

func combDirRef(traceID, combRel string) string {
	return combDirPrefix(traceID) + strings.TrimSuffix(combRel, "/")
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
	view := ArtifactContentView{Ref: canonical, Bytes: int64(len(data))}
	if !artifacts.IsTextContent(data) {
		view.Omitted = "binary or invalid UTF-8"
		return view, nil
	}
	if len(data) > artifacts.MaxInlinePreviewBytes {
		view.Omitted = "file too large for inline preview"
		return view, nil
	}
	view.Content = string(data)
	return view, nil
}
