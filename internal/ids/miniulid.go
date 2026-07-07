package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const miniULIDHexLen = 16

// MiniULID returns a lexicographically sortable identifier body.
// Layout: 48-bit Unix ms (big-endian) + 16-bit random, encoded as 16 lowercase hex chars.
func MiniULID() (string, error) {
	ms := uint64(time.Now().UTC().UnixMilli()) & 0xFFFFFFFFFFFF

	var rnd [2]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return "", fmt.Errorf("ids: mini ulid: %w", err)
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

	return hex.EncodeToString(buf[:]), nil
}

// Prefixed returns prefix + MiniULID().
func Prefixed(prefix string) (string, error) {
	body, err := MiniULID()
	if err != nil {
		return "", err
	}
	return prefix + body, nil
}

// ParseMiniULIDTime extracts the UTC timestamp from a MiniULID body.
// Returns false when the body is not exactly 16 lowercase hex chars.
func ParseMiniULIDTime(body string) (time.Time, bool) {
	if len(body) != miniULIDHexLen {
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

// ParsePrefixedTime extracts the UTC timestamp from prefix + MiniULID.
func ParsePrefixedTime(prefix, id string) (time.Time, bool) {
	body, ok := strings.CutPrefix(id, prefix)
	if !ok {
		return time.Time{}, false
	}
	return ParseMiniULIDTime(body)
}
