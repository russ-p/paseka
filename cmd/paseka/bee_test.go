package main

import (
	"testing"

	"github.com/russ-p/paseka/internal/protocol"
)

func TestOperatorExecutionPurpose(t *testing.T) {
	t.Run("defaults to work", func(t *testing.T) {
		purpose, analysisOf, err := operatorExecutionPurpose("", "")
		if err != nil {
			t.Fatal(err)
		}
		if purpose != protocol.PurposeWork || analysisOf != "" {
			t.Fatalf("got %q %q, want work", purpose, analysisOf)
		}
	})

	t.Run("analysis action names its milestone", func(t *testing.T) {
		purpose, analysisOf, err := operatorExecutionPurpose("analysis", " completion-r1 ")
		if err != nil {
			t.Fatal(err)
		}
		if purpose != protocol.PurposeAnalysis || analysisOf != "completion-r1" {
			t.Fatalf("got %q %q", purpose, analysisOf)
		}
	})

	t.Run("closing is runtime-owned", func(t *testing.T) {
		if _, _, err := operatorExecutionPurpose("closing", ""); err == nil {
			t.Fatal("an operator was allowed to label a run closing")
		}
	})

	t.Run("analysis without milestone is refused", func(t *testing.T) {
		if _, _, err := operatorExecutionPurpose("analysis", ""); err == nil {
			t.Fatal("analysis without analysisOf was accepted")
		}
	})

	t.Run("milestone without analysis is refused", func(t *testing.T) {
		if _, _, err := operatorExecutionPurpose("work", "completion-r1"); err == nil {
			t.Fatal("analysisOf without analysis was accepted")
		}
	})
}
