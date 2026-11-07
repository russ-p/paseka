package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// LedgerSchemaVersion is the only schema version this binary understands. A
// ledger written by a newer runtime fails the load rather than being read
// partially, because a half-understood disable is a gate that opens silently.
const LedgerSchemaVersion = 1

// DisabledRecord is why one skill is not offered. The ledger holds only the
// state a skill file cannot: when it was disabled and why.
type DisabledRecord struct {
	At     string `json:"at"`
	Reason string `json:"reason"`
}

// Ledger is the committed enabled/disabled state beside the store. The
// inventory is the directory listing and provenance lives in the skill's own
// metadata, so there is no mirror here to reconcile.
type Ledger struct {
	SchemaVersion int                       `json:"schema_version"`
	Disabled      map[string]DisabledRecord `json:"disabled,omitempty"`
}

// LoadLedger reads the ledger beside the repository store. A missing ledger is
// an empty ledger; a corrupt one is an error.
func LoadLedger(colonyRoot string) (Ledger, error) {
	return LoadLedgerFile(LedgerPath(colonyRoot))
}

// LoadLedgerFile reads one ledger file. It is used for the repository store and
// for a trail's frozen copy, which differ only in the bytes they read.
func LoadLedgerFile(path string) (Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return emptyLedger(), nil
		}
		return Ledger{}, fmt.Errorf("skills: read ledger %s: %w", path, err)
	}
	var ledger Ledger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return Ledger{}, fmt.Errorf("skills: ledger %s is corrupt: %w", path, err)
	}
	if ledger.SchemaVersion != LedgerSchemaVersion {
		return Ledger{}, fmt.Errorf(
			"skills: ledger %s is corrupt: schema_version is %d; this runtime understands %d",
			path, ledger.SchemaVersion, LedgerSchemaVersion,
		)
	}
	if ledger.Disabled == nil {
		ledger.Disabled = map[string]DisabledRecord{}
	}
	for name, record := range ledger.Disabled {
		if reason := validateDisabledRecord(name, record); reason != "" {
			return Ledger{}, fmt.Errorf("skills: ledger %s is corrupt: %s", path, reason)
		}
	}
	return ledger, nil
}

// Record returns the disabled record for a skill, when it has one.
func (l Ledger) Record(name string) (DisabledRecord, bool) {
	record, ok := l.Disabled[name]
	return record, ok
}

// DisabledCount is the number of skills the ledger has neutralised.
func (l Ledger) DisabledCount() int {
	return len(l.Disabled)
}

func emptyLedger() Ledger {
	return Ledger{SchemaVersion: LedgerSchemaVersion, Disabled: map[string]DisabledRecord{}}
}

func validateDisabledRecord(name string, record DisabledRecord) string {
	if strings.TrimSpace(name) == "" {
		return "a disabled record has an empty skill name"
	}
	if strings.TrimSpace(record.At) == "" {
		return fmt.Sprintf("disabled skill %q has no at", name)
	}
	if _, err := time.Parse(time.RFC3339, record.At); err != nil {
		return fmt.Sprintf("disabled skill %q has an invalid at: %v", name, err)
	}
	if strings.TrimSpace(record.Reason) == "" {
		return fmt.Sprintf("disabled skill %q has no reason", name)
	}
	return ""
}
