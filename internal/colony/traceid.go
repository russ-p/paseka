package colony

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const traceIDPrefix = "trace-"

// NewTraceID returns a lexicographically sortable trace identifier.
// Layout: 48-bit Unix ms (big-endian) + 16-bit random, encoded as 16 lowercase hex chars.
func NewTraceID() (string, error) {
	ms := uint64(time.Now().UTC().UnixMilli()) & 0xFFFFFFFFFFFF

	var rnd [2]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", fmt.Errorf("colony: trace id: %w", err)
	}

	var buf [8]byte
	buf[0] = byte(ms >> 40)
	buf[1] = byte(ms >> 32)
	buf[2] = byte(ms >> 24)
	buf[3] = byte(ms >> 16)
	buf[4] = byte(ms >> 8)
	buf[5] = byte(ms)
	buf[6] = rnd[0]
	buf[7] = rnd[1]

	return traceIDPrefix + hex.EncodeToString(buf[:]), nil
}

// ParseTraceIDTime extracts the UTC timestamp from an auto-generated mini-ULID trace id.
// Returns false for ids that do not match the expected format.
func ParseTraceIDTime(id string) (time.Time, bool) {
	body, ok := strings.CutPrefix(id, traceIDPrefix)
	if !ok || len(body) != 16 {
		return time.Time{}, false
	}
	raw, err := hex.DecodeString(body)
	if err != nil || len(raw) != 8 {
		return time.Time{}, false
	}
	ms := (uint64(raw[0]) << 40) |
		(uint64(raw[1]) << 32) |
		(uint64(raw[2]) << 24) |
		(uint64(raw[3]) << 16) |
		(uint64(raw[4]) << 8) |
		uint64(raw[5])
	return time.UnixMilli(int64(ms)).UTC(), true
}
