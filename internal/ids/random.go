package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// RandomHexLen is the number of hex characters in a Random body: 128 bits.
const RandomHexLen = 32

// Random returns an opaque identifier body: 128 bits of crypto randomness as
// lowercase hex.
//
// It exists because MiniULID is the wrong shape for an identity that must never
// repeat. MiniULID spends 48 of its 64 bits on the millisecond and 16 on
// randomness, so two ids minted in the same millisecond collide with birthday
// odds that are high enough to matter at event rates — a few hundred events
// inside one millisecond is a coin flip, and a collision is silent because
// nothing can tell two equal ids from one event. 128 random bits put that
// probability far below anything a colony can reach.
//
// It also carries no timestamp, so an id says nothing about when it was minted
// or how many were minted around it.
func Random() (string, error) {
	var buf [RandomHexLen / 2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("ids: random: %w", err)
	}
	return hex.EncodeToString(buf[:]), nil
}

// PrefixedRandom returns prefix + Random().
func PrefixedRandom(prefix string) (string, error) {
	body, err := Random()
	if err != nil {
		return "", err
	}
	return prefix + body, nil
}
