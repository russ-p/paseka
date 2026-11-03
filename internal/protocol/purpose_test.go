package protocol_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestPurposeForTaskPhase(t *testing.T) {
	cases := []struct {
		name  string
		phase protocol.TaskPhase
		want  protocol.ExecutionPurpose
	}{
		{name: "empty defaults to work", phase: "", want: protocol.PurposeWork},
		{name: "work", phase: protocol.TaskPhaseWork, want: protocol.PurposeWork},
		{name: "closing", phase: protocol.TaskPhaseClosing, want: protocol.PurposeClosing},
		{name: "merge is not goal work", phase: protocol.TaskPhaseMerge, want: protocol.PurposeWork},
		{name: "unknown phase is not closing", phase: protocol.TaskPhase("deploy"), want: protocol.PurposeWork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := protocol.PurposeForTaskPhase(tc.phase); got != tc.want {
				t.Fatalf("PurposeForTaskPhase(%q) = %q, want %q", tc.phase, got, tc.want)
			}
		})
	}
}

func TestResolveAdmissionAttribution(t *testing.T) {
	t.Run("empty purpose is ordinary work", func(t *testing.T) {
		purpose, analysisOf, err := protocol.ResolveAdmissionAttribution("", "")
		if err != nil {
			t.Fatal(err)
		}
		if purpose != protocol.PurposeWork || analysisOf != "" {
			t.Fatalf("got %q %q, want work and empty milestone", purpose, analysisOf)
		}
	})

	t.Run("analysis keeps its studied milestone", func(t *testing.T) {
		purpose, analysisOf, err := protocol.ResolveAdmissionAttribution(protocol.PurposeAnalysis, " completion-r1 ")
		if err != nil {
			t.Fatal(err)
		}
		if purpose != protocol.PurposeAnalysis || analysisOf != "completion-r1" {
			t.Fatalf("got %q %q", purpose, analysisOf)
		}
	})

	t.Run("analysis without a milestone is refused", func(t *testing.T) {
		if _, _, err := protocol.ResolveAdmissionAttribution(protocol.PurposeAnalysis, ""); err == nil {
			t.Fatal("err = nil, want an analysisOf requirement")
		}
	})

	t.Run("analysisOf without analysis is refused", func(t *testing.T) {
		if _, _, err := protocol.ResolveAdmissionAttribution(protocol.PurposeWork, "completion-r1"); err == nil {
			t.Fatal("err = nil, want analysisOf to require analysis purpose")
		}
	})

	t.Run("unsupported purpose is refused", func(t *testing.T) {
		if _, _, err := protocol.ResolveAdmissionAttribution(protocol.ExecutionPurpose("deploy"), ""); err == nil {
			t.Fatal("err = nil, want an unsupported-purpose error")
		}
	})
}
