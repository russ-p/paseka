package colony_test

import (
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/protocol"
)

func TestResolvedDefaultBee(t *testing.T) {
	t.Run("platform fallback", func(t *testing.T) {
		got := (colony.Colony{}).ResolvedDefaultBee()
		if got != protocol.DefaultBee {
			t.Fatalf("ResolvedDefaultBee() = %q, want %q", got, protocol.DefaultBee)
		}
	})

	t.Run("colony override", func(t *testing.T) {
		got := (colony.Colony{Defaults: colony.Defaults{DefaultBee: "scribe"}}).ResolvedDefaultBee()
		if got != "scribe" {
			t.Fatalf("ResolvedDefaultBee() = %q, want scribe", got)
		}
	})

	t.Run("whitespace ignored", func(t *testing.T) {
		got := colony.EffectiveTaskBee("  ", colony.Defaults{DefaultBee: "  scribe  "})
		if got != "scribe" {
			t.Fatalf("EffectiveTaskBee() = %q, want scribe", got)
		}
	})

	t.Run("task bee wins", func(t *testing.T) {
		got := colony.EffectiveTaskBee("guard", colony.Defaults{DefaultBee: "scribe"})
		if got != "guard" {
			t.Fatalf("EffectiveTaskBee() = %q, want guard", got)
		}
	})
}
