package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLedgerMissingIsEmpty(t *testing.T) {
	dir := t.TempDir()
	ledger, err := LoadLedger(dir)
	if err != nil {
		t.Fatalf("a missing ledger must read as empty: %v", err)
	}
	if ledger.SchemaVersion != LedgerSchemaVersion {
		t.Fatalf("schema version = %d", ledger.SchemaVersion)
	}
	if ledger.DisabledCount() != 0 {
		t.Fatalf("disabled = %v, want empty", ledger.Disabled)
	}
}

func TestLoadLedgerReadsDisabledRecords(t *testing.T) {
	dir := t.TempDir()
	writeLedger(t, dir, `{
  "schema_version": 1,
  "disabled": {
    "paseka-release": {"at": "2026-11-06T12:00:00Z", "reason": "broken step"}
  }
}`)
	ledger, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	record, ok := ledger.Record("paseka-release")
	if !ok || record.Reason != "broken step" {
		t.Fatalf("record = %+v, ok = %v", record, ok)
	}
	if _, ok := ledger.Record("other"); ok {
		t.Fatal("an enabled skill must not have a disabled record")
	}
}

func TestLoadLedgerCorruptIsAnError(t *testing.T) {
	cases := map[string]string{
		"not json":        `{`,
		"wrong version":   `{"schema_version": 2, "disabled": {}}`,
		"no version":      `{"disabled": {}}`,
		"no at":           `{"schema_version": 1, "disabled": {"x": {"reason": "why"}}}`,
		"bad at":          `{"schema_version": 1, "disabled": {"x": {"at": "yesterday", "reason": "why"}}}`,
		"no reason":       `{"schema_version": 1, "disabled": {"x": {"at": "2026-11-06T12:00:00Z"}}}`,
		"empty skill key": `{"schema_version": 1, "disabled": {"": {"at": "2026-11-06T12:00:00Z", "reason": "why"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeLedger(t, dir, body)
			_, err := LoadLedger(dir)
			if err == nil {
				t.Fatal("expected a corrupt ledger error")
			}
			if !strings.Contains(err.Error(), "corrupt") {
				t.Fatalf("err = %v, want it to say the ledger is corrupt", err)
			}
		})
	}
}

func writeLedger(t *testing.T, colonyRoot, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(LedgerPath(colonyRoot)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LedgerPath(colonyRoot), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
