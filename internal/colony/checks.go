package colony

import (
	"fmt"
	"strings"
	"time"
)

const (
	// DefaultCheckTimeout bounds a runtime check gate that declares no timeout.
	DefaultCheckTimeout = 10 * time.Minute
	// DefaultCheckRejectionCap is the consecutive gate rejection count after
	// which a task attempt stops producing rework dispatches.
	DefaultCheckRejectionCap = 2
)

// Checks is the optional runtime-owned mechanical gate for a bee's AFK runs.
//
// The block is colony configuration, not prompt instruction: the runtime reads
// it from the resolved configuration snapshot and runs the command in the
// dispatch workspace after the adapter exits. Nothing about a check is inferred
// from the workspace under review.
type Checks struct {
	Command Command `yaml:"command,omitempty"`
	// Timeout is a duration string. Absent means DefaultCheckTimeout; an
	// explicit zero or negative value is a load error, because there is no
	// unbounded gate.
	Timeout *string `yaml:"timeout,omitempty"`
	// RejectionCap is the number of consecutive gate rejections a task attempt
	// tolerates before the task is failed instead of redispatched. Absent means
	// DefaultCheckRejectionCap; an explicit zero means no cap.
	RejectionCap *int `yaml:"rejection_cap,omitempty"`
}

// IsSet reports whether a gate command was declared.
func (c *Checks) IsSet() bool {
	return c != nil && c.Command.IsSet()
}

// TimeoutDuration resolves the gate timeout, applying the default when absent.
func (c *Checks) TimeoutDuration() (time.Duration, error) {
	if c == nil || c.Timeout == nil {
		return DefaultCheckTimeout, nil
	}
	s := strings.TrimSpace(*c.Timeout)
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("checks.timeout must be a duration string (e.g. 90s, 10m)")
	}
	if d <= 0 {
		return 0, fmt.Errorf("checks.timeout must be a positive duration (got %q)", *c.Timeout)
	}
	return d, nil
}

// ResolvedRejectionCap returns the consecutive rejection cap. Zero means no cap.
func (c *Checks) ResolvedRejectionCap() int {
	if c == nil || c.RejectionCap == nil {
		return DefaultCheckRejectionCap
	}
	return *c.RejectionCap
}

// CheckGate returns the bee's gate when it declares a command, nil otherwise.
// A missing block is not a gate; a block without a command is a load error and
// must never silently mean "no gate".
func (b Bee) CheckGate() *Checks {
	if !b.Checks.IsSet() {
		return nil
	}
	return b.Checks
}

// ValidateChecks rejects an incomplete or impossible gate at load time.
func (b Bee) ValidateChecks() error {
	if b.Checks == nil {
		return nil
	}
	if !b.Checks.Command.IsSet() {
		return fmt.Errorf("colony: bee %q: checks.command is required", b.Role)
	}
	if _, err := b.Checks.TimeoutDuration(); err != nil {
		return fmt.Errorf("colony: bee %q: %w", b.Role, err)
	}
	if b.Checks.RejectionCap != nil && *b.Checks.RejectionCap < 0 {
		return fmt.Errorf("colony: bee %q: checks.rejection_cap must not be negative (got %d)", b.Role, *b.Checks.RejectionCap)
	}
	return nil
}
