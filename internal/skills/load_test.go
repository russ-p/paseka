package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadResolution(t *testing.T, root, role string) RunResolution {
	t.Helper()
	resolved, err := ResolveRun(RunInput{ConfigRoot: root, Role: role})
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func TestOfferedServesOnlyTheRunsSet(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release")
	writeRunSkill(t, root, "paseka-ship", "Ship procedure.", "# Ship")
	resolved := loadResolution(t, root, "builder")

	skill, err := Offered(resolved, "paseka-release")
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "paseka-release" {
		t.Fatalf("served %q", skill.Name)
	}

	for _, raw := range []string{"paseka-ship", "paseka-*", "*"} {
		_, err := Offered(resolved, raw)
		if err == nil {
			t.Fatalf("%q must be refused", raw)
		}
		var refused *NotOfferedError
		if !errors.As(err, &refused) {
			t.Fatalf("%q error is not a refusal: %v", raw, err)
		}
		if !strings.Contains(err.Error(), "skill_not_offered") {
			t.Fatalf("%q refusal does not name its kind: %v", raw, err)
		}
	}

	// An invalid reference is not a procedure this run carries either.
	_, err = Offered(resolved, "shared:paseka-release")
	if err == nil || !strings.Contains(err.Error(), "skill_not_offered") {
		t.Fatalf("an invalid reference must be refused as not offered: %v", err)
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestServedHashesTheServedBytesAndStripsFrontmatter(t *testing.T) {
	root := runFixture(t, "builder", "role: builder\nadapter: script\ncommand: [echo]\nskills:\n  - paseka-release\n")
	writeRunSkill(t, root, "paseka-release", "Release procedure.", "# Release\n\nStep one.")
	resolved := loadResolution(t, root, "builder")

	body, hash, err := Served(resolved.Skills[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "description:") || strings.Contains(body, "---") {
		t.Fatalf("served body still carries frontmatter: %q", body)
	}
	if !strings.Contains(body, "Step one.") {
		t.Fatalf("served body = %q", body)
	}
	data, err := os.ReadFile(resolved.Skills[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256Hex(data)
	if hash != want {
		t.Fatalf("contentHash = %q, want %q", hash, want)
	}
}

func TestUsageRecordsOneLinePerLoadAndDedupeFirstWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, UsedFileName)
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC).Format(time.RFC3339)

	records := []UsageRecord{
		{Name: "b", ContentHash: "hash-b1", At: at},
		{Name: "a", ContentHash: "hash-a", At: at},
		{Name: "b", ContentHash: "hash-b2", At: at},
	}
	for _, record := range records {
		if err := AppendUsage(path, record); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ReadUsage(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d records, want one per load: %+v", len(got), got)
	}
	if got[0].ContentHash != "hash-b1" || got[2].ContentHash != "hash-b2" {
		t.Fatalf("records were not written in load order: %+v", got)
	}
	if names := UsedNames(got); !equalStrings(names, []string{"b", "a"}) {
		t.Fatalf("used names = %v, want the deduplicated first-load order", names)
	}
}

func TestReadUsageMissingFileIsNoLoads(t *testing.T) {
	records, err := ReadUsage(filepath.Join(t.TempDir(), UsedFileName))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v", records)
	}
}

func TestReadUsageCorruptLineNamesTheLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), UsedFileName)
	if err := os.WriteFile(path, []byte("{\"name\":\"a\"}\nnot-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadUsage(path)
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v, want the corrupt line named", err)
	}
}
