package gitroot

import (
	"errors"
	"fmt"
)

// ErrRefused is a policy refusal (non-fast-forward, dirty pull, nothing to push, unsafe delete).
var ErrRefused = errors.New("git: refused")

// Refused returns err wrapping ErrRefused with a Beekeeper-facing message.
func Refused(msg string) error {
	return fmt.Errorf("%w: %s", ErrRefused, msg)
}
