package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLedgerDisableAndEnableUndoEachOther(t *testing.T) {
	dir := t.TempDir()
	ledger, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	if err := ledger.Disable("paseka-release", "  step 2 is wrong  ", at); err != nil {
		t.Fatal(err)
	}
	record, ok := ledger.Record("paseka-release")
	if !ok {
		t.Fatal("disable did not record the skill")
	}
	if record.At != "2026-11-06T12:00:00Z" || record.Reason != "step 2 is wrong" {
		t.Fatalf("record = %+v", record)
	}
	if err := SaveLedger(dir, ledger); err != nil {
		t.Fatal(err)
	}
	reloaded, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Record("paseka-release"); !ok {
		t.Fatal("the record did not survive a save and load")
	}

	changed, err := reloaded.Enable("paseka-release")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("enable reported no change for a disabled skill")
	}
	if _, ok := reloaded.Record("paseka-release"); ok {
		t.Fatal("enable did not clear the record")
	}
	if err := SaveLedger(dir, reloaded); err != nil {
		t.Fatal(err)
	}
	after, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if after.DisabledCount() != 0 {
		t.Fatalf("disabled = %v, want empty", after.Disabled)
	}
}

func TestLedgerEnableUnchanged(t *testing.T) {
	ledger, err := LoadLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	changed, err := ledger.Enable("other")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("enable reported a change when there was no record")
	}
}

func TestLedgerMutationsRefusePatterns(t *testing.T) {
	ledger, err := LoadLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"paseka-*", "*"} {
		if err := ledger.Disable(name, "why", time.Now()); err == nil {
			t.Fatalf("disable %q was accepted", name)
		}
		if _, err := ledger.Enable(name); err == nil {
			t.Fatalf("enable %q was accepted", name)
		}
	}
}

func TestLedgerDisableNeedsAReason(t *testing.T) {
	ledger, err := LoadLedger(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = ledger.Disable("paseka-release", "   ", time.Now())
	if err == nil {
		t.Fatal("a disable without a reason was accepted")
	}
	if !strings.Contains(err.Error(), "needs a reason") {
		t.Fatalf("err = %v", err)
	}
}

func TestSaveLedgerRefusesToWriteAnInvalidLedger(t *testing.T) {
	dir := t.TempDir()
	ledger := Ledger{
		SchemaVersion: LedgerSchemaVersion,
		Disabled: map[string]DisabledRecord{
			"paseka-release": {At: "yesterday", Reason: "why"},
		},
	}
	if err := SaveLedger(dir, ledger); err == nil {
		t.Fatal("an invalid ledger was written")
	}
	if _, err := os.Stat(LedgerPath(dir)); !os.IsNotExist(err) {
		t.Fatalf("a refused write left a ledger behind: %v", err)
	}
}

func TestSaveLedgerWritesAtomically(t *testing.T) {
	dir := t.TempDir()
	ledger, err := LoadLedger(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Disable("paseka-release", "why", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := SaveLedger(dir, ledger); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("a temporary ledger file was left behind: %s", entry.Name())
		}
	}
	data, err := os.ReadFile(LedgerPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatalf("ledger does not end with a newline: %q", data)
	}
}

func TestRemoveSkillDeletesEntryAndRecord(t *testing.T) {
	colonyRoot := t.TempDir()
	dir := filepath.Join(Dir(colonyRoot), "paseka-release")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestName), []byte("---\nname: paseka-release\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := LoadLedger(colonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Disable("paseka-release", "why", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := SaveLedger(colonyRoot, ledger); err != nil {
		t.Fatal(err)
	}

	after, err := RemoveSkill(colonyRoot, "paseka-release")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("store entry still exists: %v", err)
	}
	if _, ok := after.Record("paseka-release"); ok {
		t.Fatal("the ledger record was left behind")
	}
	reloaded, err := LoadLedger(colonyRoot)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.DisabledCount() != 0 {
		t.Fatalf("disabled = %v, want empty", reloaded.Disabled)
	}
}

func TestRemoveSkillRefusesAPattern(t *testing.T) {
	colonyRoot := t.TempDir()
	if _, err := RemoveSkill(colonyRoot, "paseka-*"); err == nil {
		t.Fatal("remove accepted a pattern")
	}
}

func TestRemoveSkillCorruptLedgerTouchesNothing(t *testing.T) {
	colonyRoot := t.TempDir()
	dir := filepath.Join(Dir(colonyRoot), "paseka-release")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeLedger(t, colonyRoot, "{")
	if _, err := RemoveSkill(colonyRoot, "paseka-release"); err == nil {
		t.Fatal("remove accepted a corrupt ledger")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("a refused removal deleted the store entry: %v", err)
	}
}
