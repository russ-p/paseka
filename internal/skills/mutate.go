package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Disable records that one skill must not be offered. The record carries when
// and why because those are the two facts a skill file cannot hold, and the
// caller owns the decision. A disable is neutralisation, not deletion: the
// directory stays in the store and its history stays in git.
func (l *Ledger) Disable(name, reason string, at time.Time) error {
	ref, err := ParseReference(name)
	if err != nil {
		return err
	}
	if ref.IsPattern() {
		return fmt.Errorf("skills: disable acts on one skill; %q is a pattern", ref)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return fmt.Errorf("skills: disable %q needs a reason; pass --reason with why it must not be offered", name)
	}
	record := DisabledRecord{At: at.UTC().Format(time.RFC3339), Reason: reason}
	if problem := validateDisabledRecord(name, record); problem != "" {
		return fmt.Errorf("skills: %s", problem)
	}
	if l.Disabled == nil {
		l.Disabled = map[string]DisabledRecord{}
	}
	l.Disabled[name] = record
	return nil
}

// Enable clears one skill's disabled record. It reports whether there was a
// record to clear, so a caller can say plainly that a flip changed nothing
// rather than claiming a write it did not make.
func (l *Ledger) Enable(name string) (bool, error) {
	ref, err := ParseReference(name)
	if err != nil {
		return false, err
	}
	if ref.IsPattern() {
		return false, fmt.Errorf("skills: enable acts on one skill; %q is a pattern", ref)
	}
	if _, ok := l.Disabled[name]; !ok {
		return false, nil
	}
	delete(l.Disabled, name)
	return true, nil
}

// SaveLedger writes the ledger beside the store atomically, so an interrupted
// write leaves the previous ledger readable rather than a truncated one. It
// validates before writing: a caller can never turn an in-memory mistake into
// the committed state, and an existing corrupt ledger is never silently
// replaced because LoadLedger already refused it.
func SaveLedger(colonyRoot string, ledger Ledger) error {
	return SaveLedgerFile(LedgerPath(colonyRoot), ledger)
}

// SaveLedgerFile writes one ledger file. It exists for the same reason
// LoadLedgerFile does: a trail's frozen copy and the repository store differ
// only in the path they read and write.
func SaveLedgerFile(path string, ledger Ledger) error {
	if ledger.SchemaVersion == 0 {
		ledger.SchemaVersion = LedgerSchemaVersion
	}
	if ledger.SchemaVersion != LedgerSchemaVersion {
		return fmt.Errorf(
			"skills: cannot write ledger %s: schema_version is %d; this runtime writes %d",
			path, ledger.SchemaVersion, LedgerSchemaVersion,
		)
	}
	if ledger.Disabled == nil {
		ledger.Disabled = map[string]DisabledRecord{}
	}
	for name, record := range ledger.Disabled {
		if reason := validateDisabledRecord(name, record); reason != "" {
			return fmt.Errorf("skills: cannot write ledger %s: %s", path, reason)
		}
	}

	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: encode ledger %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("skills: write ledger %s: %w", path, err)
	}
	return nil
}

// writeFileAtomic writes data to a temporary file in the destination directory
// and renames it into place. A rename on the same filesystem is atomic, so a
// reader sees either the old ledger or the new one.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// RemoveSkill deletes one store entry and its ledger record. The ledger record
// goes first, so a failure midway leaves an enabled skill rather than a record
// pointing at a directory that no longer exists. The caller is responsible for
// refusing while a role still references the skill.
//
// It reloads the ledger instead of trusting a caller's copy: a corrupt ledger
// must fail the removal before a single directory is touched, and this function
// is also the one the tests drive directly.
func RemoveSkill(colonyRoot, name string) (Ledger, error) {
	ref, err := ParseReference(name)
	if err != nil {
		return Ledger{}, err
	}
	if ref.IsPattern() {
		return Ledger{}, fmt.Errorf("skills: remove acts on one skill; %q is a pattern", ref)
	}
	ledger, err := LoadLedger(colonyRoot)
	if err != nil {
		return Ledger{}, err
	}
	dir := filepath.Join(Dir(colonyRoot), name)
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ledger, fmt.Errorf("skills: %q is not in the store at %s", name, Dir(colonyRoot))
		}
		return ledger, fmt.Errorf("skills: inspect store entry %s: %w", dir, err)
	}
	if !info.IsDir() {
		return ledger, fmt.Errorf("skills: store entry %s is not a directory", dir)
	}

	if _, disabled := ledger.Disabled[name]; disabled {
		delete(ledger.Disabled, name)
		if err := SaveLedger(colonyRoot, ledger); err != nil {
			return ledger, err
		}
	}
	if err := os.RemoveAll(dir); err != nil {
		return ledger, fmt.Errorf("skills: remove store entry %s: %w", dir, err)
	}
	return ledger, nil
}
