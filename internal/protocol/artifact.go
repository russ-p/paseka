package protocol

import (
	"encoding/json"
	"strings"
)

// ArtifactEventKind identifies SIGNAL artifact payload variants.
type ArtifactEventKind string

const (
	SignalArtifactWritten ArtifactEventKind = "artifact.written"
)

// ArtifactWrittenItem is one handoff file in artifact.written.
type ArtifactWrittenItem struct {
	Ref          string `json:"ref"`
	ArtifactKind string `json:"artifactKind"`
	Title        string `json:"title,omitempty"`
}

// ArtifactWrittenPayload is emitted as SIGNAL with payload.kind=artifact.written.
type ArtifactWrittenPayload struct {
	Kind         ArtifactEventKind     `json:"kind"`
	Ref          string                `json:"ref,omitempty"`
	ArtifactKind string                `json:"artifactKind,omitempty"`
	Title        string                `json:"title,omitempty"`
	Artifacts    []ArtifactWrittenItem `json:"artifacts,omitempty"`
}

// NormalizeArtifactWrittenPayload expands sugar fields into artifacts[].
func NormalizeArtifactWrittenPayload(p *ArtifactWrittenPayload) {
	if p == nil {
		return
	}
	p.Kind = SignalArtifactWritten
	if len(p.Artifacts) > 0 {
		for i := range p.Artifacts {
			if strings.TrimSpace(p.Artifacts[i].ArtifactKind) == "" {
				p.Artifacts[i].ArtifactKind = artifactKindFromRef(p.Artifacts[i].Ref)
			}
		}
		return
	}
	if ref := strings.TrimSpace(p.Ref); ref != "" {
		kind := strings.TrimSpace(p.ArtifactKind)
		if kind == "" {
			kind = artifactKindFromRef(ref)
		}
		item := ArtifactWrittenItem{
			Ref:          ref,
			ArtifactKind: kind,
		}
		if title := strings.TrimSpace(p.Title); title != "" {
			item.Title = title
		}
		p.Artifacts = []ArtifactWrittenItem{item}
	}
	p.Ref = ""
	p.ArtifactKind = ""
	p.Title = ""
}

func artifactKindFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = strings.ReplaceAll(ref, "\\", "/")
	base := ref
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if i := strings.LastIndex(base, "."); i > 0 {
		base = base[:i]
	}
	return base
}

// ArtifactWrittenRefs extracts refs from a normalized or sugar payload JSON.
func ArtifactWrittenRefs(payload json.RawMessage) ([]string, error) {
	var p ArtifactWrittenPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	NormalizeArtifactWrittenPayload(&p)
	var refs []string
	for _, item := range p.Artifacts {
		if ref := strings.TrimSpace(item.Ref); ref != "" {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
