package review

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/russ-p/paseka/internal/trailconfig"
	"github.com/russ-p/paseka/internal/trailfork"
)

// IsExperimentalTrail reports whether a trail is pinned experimental work whose
// branch must never be delivered. A fork records that policy in its immutable
// configuration binding, so delivery checks do not need to re-read lineage.
func IsExperimentalTrail(colonyRoot, traceID string) (bool, error) {
	if strings.TrimSpace(colonyRoot) == "" || strings.TrimSpace(traceID) == "" {
		return false, nil
	}
	manifest, err := trailconfig.Load(colonyRoot, traceID)
	switch {
	case err == nil:
		if manifest.Policy == trailconfig.PolicyPinned {
			return true, nil
		}
	case !errors.Is(err, trailconfig.ErrNoBinding):
		return false, fmt.Errorf("review: inspect trail policy: %w", err)
	}

	// A fork's lineage is the second durable marker. Keep delivery disabled even
	// if its configuration binding is unavailable or has been replaced.
	if _, lineageErr := trailfork.LoadLineage(colonyRoot, traceID); lineageErr == nil {
		return true, nil
	} else if !errors.Is(lineageErr, os.ErrNotExist) {
		return false, fmt.Errorf("review: inspect fork lineage: %w", lineageErr)
	}
	return false, nil
}

// errExperimentalDelivery is returned by every delivery entry point that would
// otherwise merge, push, or publish an experimental branch.
func errExperimentalDelivery(traceID string) error {
	return fmt.Errorf("review: trail %s is an experimental fork; merge, branch push, and pull-request publication are disabled", traceID)
}
