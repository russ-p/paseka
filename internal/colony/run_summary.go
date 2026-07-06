package colony

import (
	"fmt"
	"strings"
)

// RunSummaryPolicy controls runtime synthesis and enforcement of INSIGHT/run.summary.
type RunSummaryPolicy string

const (
	RunSummaryAuto     RunSummaryPolicy = "auto"
	RunSummaryRequired RunSummaryPolicy = "required"
	RunSummaryDisabled RunSummaryPolicy = "disabled"
)

// ResolvedRunSummaryPolicy returns the effective run-summary policy for a bee.
func (b Bee) ResolvedRunSummaryPolicy() RunSummaryPolicy {
	switch RunSummaryPolicy(strings.TrimSpace(string(b.RunSummary))) {
	case "":
		return RunSummaryAuto
	case RunSummaryAuto, RunSummaryRequired, RunSummaryDisabled:
		return RunSummaryPolicy(strings.TrimSpace(string(b.RunSummary)))
	default:
		return RunSummaryAuto
	}
}

// ValidateRunSummaryPolicy checks the configured run-summary policy at load time.
func (b Bee) ValidateRunSummaryPolicy() error {
	raw := strings.TrimSpace(string(b.RunSummary))
	if raw == "" {
		return nil
	}
	switch RunSummaryPolicy(raw) {
	case RunSummaryAuto, RunSummaryRequired, RunSummaryDisabled:
		return nil
	default:
		return fmt.Errorf("colony: bee %q: invalid run_summary %q (want auto, required, or disabled)", b.Role, raw)
	}
}
