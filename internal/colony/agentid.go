package colony

import (
	"github.com/paseka/paseka/internal/ids"
)

// NewAgentID returns a lexicographically sortable agent run identifier.
// Layout: 16 lowercase hex chars (48-bit UTC ms + 16-bit random).
func NewAgentID() (string, error) {
	return ids.MiniULID()
}
