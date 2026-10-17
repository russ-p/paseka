package runs

import (
	"bufio"
	"os"
)

// maxLineBytes bounds one NDJSON line. bufio.Scanner defaults to 64 KiB, which
// a payload reaches for reasons this package allows: a hand-authored MUTATION
// may inline a diff of any size (protocol validation caps none) and still lands
// under JetStream's default 1 MiB payload, and a transcript entry carries
// free-form agent output. The default limit turns one long line into a read
// error for the whole file, which is how a single large proposal used to fail
// every trail surface that merges run logs.
const maxLineBytes = 8 << 20 // 8 MiB

// newLineScanner returns a scanner over an NDJSON file sized for payloads
// rather than identifiers. Every reader of a .ndjson file in this package goes
// through it so the limit is stated once.
func newLineScanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	return sc
}
