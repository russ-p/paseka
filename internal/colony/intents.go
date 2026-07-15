package colony

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IntentGeneral is the conventional default intent name when present in a bee vocabulary.
const IntentGeneral = "general"

// DiscoverIntents returns the intent vocabulary and default for a bee.
// Explicit bee.Intents wins; otherwise intents are discovered from
// .paseka/prompts/_partials/<role>-intent-*.md partial files.
func DiscoverIntents(colonyRoot string, bee Bee) (intents []string, defaultIntent string, err error) {
	if len(bee.Intents) > 0 {
		intents = normalizeIntentList(bee.Intents)
	} else {
		intents, err = discoverIntentsFromPartials(colonyRoot, bee.Role)
		if err != nil {
			return nil, "", err
		}
	}
	defaultIntent = resolveDefaultIntent(bee.DefaultIntent, intents)
	return intents, defaultIntent, nil
}

func normalizeIntentList(raw []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range raw {
		name := cleanIntentName(item)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

func cleanIntentName(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func discoverIntentsFromPartials(colonyRoot, role string) ([]string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return nil, nil
	}
	prefix := role + "-intent-"
	partialsDir := filepath.Join(colonyRoot, pasekaDir, "prompts", "_partials")
	entries, err := os.ReadDir(partialsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("colony: read intent partials: %w", err)
	}
	var intents []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		stem := strings.TrimSuffix(name, ".md")
		if !strings.HasPrefix(stem, prefix) {
			continue
		}
		intent := strings.TrimPrefix(stem, prefix)
		if intent == "" {
			continue
		}
		intents = append(intents, intent)
	}
	sort.Strings(intents)
	return intents, nil
}

func resolveDefaultIntent(configured string, intents []string) string {
	configured = cleanIntentName(configured)
	if configured != "" {
		for _, intent := range intents {
			if intent == configured {
				return configured
			}
		}
	}
	for _, intent := range intents {
		if intent == IntentGeneral {
			return IntentGeneral
		}
	}
	if len(intents) > 0 {
		return intents[0]
	}
	return ""
}
