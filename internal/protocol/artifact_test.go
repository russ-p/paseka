package protocol

import (
	"encoding/json"
	"testing"
)

func TestValidateArtifactWritten(t *testing.T) {
	raw := []byte(`{"kind":"artifact.written","artifacts":[{"ref":".paseka/runs/t/artifacts/a.md","artifactKind":"a"}]}`)
	in := EventInput{TraceID: "t", AgentID: "agent-1", Type: EventSignal, Payload: raw}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("validate: %+v", details)
	}
}

func TestValidateArtifactWrittenSugar(t *testing.T) {
	raw := []byte(`{"kind":"artifact.written","ref":"notes.md","artifactKind":"notes","title":"Notes"}`)
	in := EventInput{TraceID: "t", AgentID: "console", Type: EventSignal, Payload: raw}
	if details := in.Validate(); len(details) != 0 {
		t.Fatalf("validate sugar: %+v", details)
	}
	var p ArtifactWrittenPayload
	_ = json.Unmarshal(raw, &p)
	NormalizeArtifactWrittenPayload(&p)
	if len(p.Artifacts) != 1 || p.Artifacts[0].Ref != "notes.md" {
		t.Fatalf("normalize: %+v", p.Artifacts)
	}
}

func TestArtifactWrittenRefs(t *testing.T) {
	raw := json.RawMessage(`{"kind":"artifact.written","ref":"a.md","artifactKind":"a"}`)
	refs, err := ArtifactWrittenRefs(raw)
	if err != nil || len(refs) != 1 || refs[0] != "a.md" {
		t.Fatalf("refs = %v err = %v", refs, err)
	}
}

func TestValidateArtifactWrittenRejectsEmpty(t *testing.T) {
	raw := []byte(`{"kind":"artifact.written","artifacts":[]}`)
	in := EventInput{TraceID: "t", AgentID: "a", Type: EventSignal, Payload: raw}
	if details := in.Validate(); len(details) == 0 {
		t.Fatal("expected validation error")
	}
}
