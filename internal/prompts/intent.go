package prompts

import (
	"strings"

	"github.com/paseka/paseka/internal/colony"
)

// IntentGeneral is the conventional default intent name when present in a bee vocabulary.
const IntentGeneral = colony.IntentGeneral

// DiscoverIntents returns the intent vocabulary and default for a bee.
func DiscoverIntents(colonyRoot string, bee colony.Bee) (intents []string, defaultIntent string, err error) {
	return colony.DiscoverIntents(colonyRoot, bee)
}

// NormalizeIntent maps a caller intent to a stable partial name for the bee vocabulary.
// Empty values become defaultIntent. Unknown values also become defaultIntent;
// the raw requested value remains in Context.IntentRaw.
func NormalizeIntent(raw string, known []string, defaultIntent string) string {
	raw = cleanIntentName(raw)
	if raw == "" {
		return defaultIntent
	}
	for _, intent := range known {
		if raw == intent {
			return raw
		}
	}
	return defaultIntent
}

func cleanIntentName(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}
