package patches_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/patches"
)

func store(t *testing.T) (string, patches.Store) {
	t.Helper()
	root := t.TempDir()
	return root, patches.NewStore(root, "trace-1")
}

func record(agentID string) patches.Record {
	return patches.Record{
		AgentID:     agentID,
		Bee:         "builder",
		Mode:        patches.ModeAFK,
		Kind:        "code.proposal.isolated",
		Workspace:   "isolated",
		Summary:     "add the export flag",
		FileCount:   patches.FileCountFromDiff("diff --git a/x b/x\n"),
		CapturedAt:  time.Date(2026, 1, 9, 10, 15, 0, 0, time.UTC),
		Publication: patches.PublicationUnpublished,
	}
}

func TestWriteRoundTripsBodyAndMetadata(t *testing.T) {
	_, s := store(t)
	body := []byte("diff --git a/x b/x\n+added\n")

	installed, err := s.Write(record("agent-1"), body)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Bytes != int64(len(body)) {
		t.Fatalf("bytes = %d, want %d", installed.Bytes, len(body))
	}
	if installed.SchemaVersion != patches.SchemaVersion {
		t.Fatalf("schema = %d", installed.SchemaVersion)
	}
	if !installed.CapturedAt.Equal(record("agent-1").CapturedAt) {
		t.Fatalf("capturedAt = %v", installed.CapturedAt)
	}

	got, ok, err := s.Read("agent-1")
	if err != nil || !ok {
		t.Fatalf("read: ok=%v err=%v", ok, err)
	}
	if got.Summary != "add the export flag" || got.Bee != "builder" {
		t.Fatalf("metadata lost: %+v", got)
	}
	if got.FileCount == nil || *got.FileCount != 1 {
		t.Fatalf("fileCount = %v", got.FileCount)
	}

	content, err := s.ReadContent("agent-1", patches.MaxPreviewBytes)
	if err != nil {
		t.Fatal(err)
	}
	if string(content.Body) != string(body) {
		t.Fatalf("body = %q", content.Body)
	}
	if content.Omitted != "" || content.Truncated {
		t.Fatalf("unexpected omission: %+v", content)
	}
}

// Bytes are stored exactly as the collector produced them. A diff is not guaranteed to be
// valid UTF-8, and rewriting a history record to fit a renderer would be silent data loss.
func TestNonUTF8BytesRoundTripWithoutMutation(t *testing.T) {
	_, s := store(t)
	body := []byte{'d', 'i', 'f', 'f', 0xff, 0xfe, 0x00, 0x80}

	if _, err := s.Write(record("agent-1"), body); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.PatchPath("agent-1"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(body) {
		t.Fatalf("stored bytes were rewritten: %q", raw)
	}

	// A text surface omits it with a reason rather than showing half a rune.
	content, err := s.ReadContent("agent-1", patches.MaxPreviewBytes)
	if err != nil {
		t.Fatal(err)
	}
	if content.Omitted == "" {
		t.Fatal("unsuitable content must be omitted with a reason")
	}
	if content.Bytes != int64(len(body)) {
		t.Fatalf("omitted content lost its size: %d", content.Bytes)
	}
}

func TestListIsOrderedByCaptureTimeWithAgentTieBreak(t *testing.T) {
	_, s := store(t)
	for _, rec := range []patches.Record{
		{AgentID: "c", CapturedAt: time.Date(2026, 1, 9, 12, 0, 0, 0, time.UTC)},
		{AgentID: "b", CapturedAt: time.Date(2026, 1, 9, 10, 0, 0, 0, time.UTC)},
		{AgentID: "a", CapturedAt: time.Date(2026, 1, 9, 10, 0, 0, 0, time.UTC)},
	} {
		if _, err := s.Write(rec, []byte(rec.AgentID)); err != nil {
			t.Fatal(err)
		}
	}

	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, rec := range listed.Records {
		got = append(got, rec.AgentID)
	}
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

// A trail that predates the archive has no proposal directory, and that is absence rather
// than failure.
func TestListOnMissingStoreIsEmptyNotAnError(t *testing.T) {
	_, s := store(t)
	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 || len(listed.Diagnostics) != 0 {
		t.Fatalf("expected empty, got %+v", listed)
	}
}

// Retrying a finalization must reuse the record: recomputing bytes or a timestamp would
// replace the history of a run with a second version of itself.
func TestIdenticalRetryReusesTheRecord(t *testing.T) {
	_, s := store(t)
	first, err := s.Write(record("agent-1"), []byte("original"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Write(record("agent-1"), []byte("different"))
	if err != nil {
		t.Fatalf("retry must not fail: %v", err)
	}
	if second.Bytes != first.Bytes || !second.CapturedAt.Equal(first.CapturedAt) {
		t.Fatalf("retry replaced the record: %+v vs %+v", first, second)
	}
	body, _ := os.ReadFile(s.PatchPath("agent-1"))
	if string(body) != "original" {
		t.Fatalf("bytes replaced: %q", body)
	}
}

func TestConflictingCreateIsAnError(t *testing.T) {
	_, s := store(t)
	// A patch with no sidecar: the state an interrupted write leaves behind.
	if err := os.MkdirAll(s.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.PatchPath("agent-1"), []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write(record("agent-1"), []byte("different length")); err == nil {
		t.Fatal("a conflicting orphan must be refused, not overwritten")
	}
	body, _ := os.ReadFile(s.PatchPath("agent-1"))
	if string(body) != "orphan" {
		t.Fatalf("orphan bytes changed: %q", body)
	}
}

// An interrupted write can leave a complete patch body without its sidecar. Finishing it
// is the one repair that does not rewrite bytes.
func TestOrphanPatchIsFinishedByInstallingItsSidecar(t *testing.T) {
	_, s := store(t)
	if err := os.MkdirAll(s.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.PatchPath("agent-1"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write(record("agent-1"), []byte("body")); err != nil {
		t.Fatalf("orphan repair: %v", err)
	}
	if _, ok, err := s.Read("agent-1"); err != nil || !ok {
		t.Fatalf("record still incomplete: ok=%v err=%v", ok, err)
	}

}

func TestConcurrentCreatesCannotReplaceEachOther(t *testing.T) {
	_, s := store(t)
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Write(record("agent-1"), []byte(strings.Repeat("x", i+1)))
		}(i)
	}
	wg.Wait()

	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 {
		t.Fatalf("expected exactly one record, got %d", len(listed.Records))
	}
	// Whoever won, the stored body matches the recorded size: no interleaved write left
	// a record that lies about its own bytes.
	body, err := os.ReadFile(s.PatchPath("agent-1"))
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(body)) != listed.Records[0].Bytes {
		t.Fatalf("size %d does not match body %d", listed.Records[0].Bytes, len(body))
	}
}

func TestIncompleteAndMalformedRecordsAreDiagnosedNotHidden(t *testing.T) {
	_, s := store(t)
	if _, err := s.Write(record("good"), []byte("good body")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root(), "orphan.json"), []byte(`{"agentId":"orphan","publication":"unpublished","bytes":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root(), "broken.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 || listed.Records[0].AgentID != "good" {
		t.Fatalf("valid record disappeared: %+v", listed.Records)
	}
	if len(listed.Diagnostics) != 2 {
		t.Fatalf("diagnostics = %+v", listed.Diagnostics)
	}
	for _, d := range listed.Diagnostics {
		if d.Reason == "" {
			t.Fatalf("diagnostic without a reason: %+v", d)
		}
	}
}

// A patch body without its sidecar is an incomplete record: it is not listed as a valid
// entry, and the operator can see that it exists and why it is not usable.
func TestOrphanPatchWithoutSidecarIsDiagnosed(t *testing.T) {
	_, s := store(t)
	if err := os.MkdirAll(s.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.PatchPath("agent-1"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}

	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 0 {
		t.Fatalf("an orphan patch became a record: %+v", listed.Records)
	}
	if len(listed.Diagnostics) != 1 || listed.Diagnostics[0].AgentID != "agent-1" {
		t.Fatalf("diagnostics = %+v", listed.Diagnostics)
	}
}

// The proposal directory is part of the trace's run tree. A symlink that redirects it
// elsewhere must not let a write (or a read) escape.
func TestWriteRefusesASymlinkedStoreDirectory(t *testing.T) {
	_, s := store(t)
	traceDir := filepath.Dir(s.Root())
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, s.Root()); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := s.Write(record("agent-1"), []byte("body")); err == nil {
		t.Fatal("a write through a symlinked store directory must be refused")
	}
	if _, err := os.Stat(filepath.Join(outside, "agent-1.patch")); !os.IsNotExist(err) {
		t.Fatalf("the write escaped the trace run tree: %v", err)
	}
	if _, err := s.List(); err == nil {
		t.Fatal("a listing through a symlinked store directory must be refused")
	}
}

// Temporary files are never proposal records, so a crash mid-write cannot surface one.
func TestTemporaryFilesAreNotListings(t *testing.T) {
	_, s := store(t)
	if _, err := s.Write(record("agent-1"), []byte("body")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root(), ".agent-1.999.patch.tmp"), []byte("half"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Records) != 1 {
		t.Fatalf("temp file became a record: %+v", listed)
	}
}

func TestMarkPublicationUpdatesStatusWithoutTouchingContent(t *testing.T) {
	_, s := store(t)
	if _, err := s.Write(record("agent-1"), []byte("body")); err != nil {
		t.Fatal(err)
	}
	confirmed := time.Date(2026, 1, 9, 11, 0, 0, 0, time.UTC)

	if err := s.MarkPublication("agent-1", patches.PublicationUnconfirmed, time.Time{}, ""); err != nil {
		t.Fatal(err)
	}
	mid, _, _ := s.Read("agent-1")
	if mid.Publication != patches.PublicationUnconfirmed || mid.PublishedAt != nil {
		t.Fatalf("unconfirmed state = %+v", mid)
	}

	if err := s.MarkPublication("agent-1", patches.PublicationPublished, confirmed, ""); err != nil {
		t.Fatal(err)
	}
	done, _, _ := s.Read("agent-1")
	if done.Publication != patches.PublicationPublished {
		t.Fatalf("published state = %q", done.Publication)
	}
	if done.PublishedAt == nil || !done.PublishedAt.Equal(confirmed) {
		t.Fatalf("confirmation time = %v", done.PublishedAt)
	}
	if done.CapturedAt.IsZero() || done.Bytes != int64(len("body")) {
		t.Fatalf("content identity changed: %+v", done)
	}

	if err := s.MarkPublication("agent-1", "nonsense", time.Time{}, ""); err == nil {
		t.Fatal("an unknown publication state must be refused")
	}
}

func TestMarkPublicationOnAMissingRecordIsAnError(t *testing.T) {
	_, s := store(t)
	if err := s.MarkPublication("nobody", patches.PublicationPublished, time.Now(), ""); err == nil {
		t.Fatal("expected an error for a record that does not exist")
	}
}

// The preview cap is applied to a stat, not to a read, so an oversized file is never
// allocated whole to discover its size.
func TestReadContentRefusesAnOversizedBodyWithoutReadingIt(t *testing.T) {
	_, s := store(t)
	big := []byte(strings.Repeat("x", 4096))
	if _, err := s.Write(record("agent-1"), big); err != nil {
		t.Fatal(err)
	}

	content, err := s.ReadContent("agent-1", 512)
	if err != nil {
		t.Fatal(err)
	}
	if content.Omitted == "" {
		t.Fatal("an oversized body must be omitted with a reason")
	}
	if content.Bytes != int64(len(big)) {
		t.Fatalf("size = %d", content.Bytes)
	}
	if len(content.Body) != 0 {
		t.Fatal("an omitted body must not carry bytes")
	}
}

// A file can grow between the stat and the read, so the read is bounded too — and the
// caller is told the preview is a prefix rather than being handed a silent truncation.
func TestReadContentStaysBoundedWhenTheFileGrowsAfterStat(t *testing.T) {
	_, s := store(t)
	if _, err := s.Write(record("agent-1"), []byte("first version, well under the cap right now.")); err != nil {
		t.Fatal(err)
	}
	// The sidecar's recorded size no longer matches, which is a legitimate reason for a
	// later reader to disagree; the point under test is that the read itself is bounded.
	if err := os.WriteFile(s.PatchPath("agent-1"), []byte(strings.Repeat("y", 100_000)), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := s.ReadContent("agent-1", 4096)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Body) > 4096 {
		t.Fatalf("read %d bytes past the cap", len(content.Body))
	}
	if content.Bytes != 100_000 {
		t.Fatalf("size after growth = %d", content.Bytes)
	}
}

func TestReadContentSkipsANonRegularRef(t *testing.T) {
	_, s := store(t)
	if err := os.MkdirAll(s.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.PatchPath("agent-1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadContent("agent-1", 1024); err == nil {
		t.Fatal("a directory is not a patch body")
	}
}

// A ref resolves against the colony root, independent of whatever directory the reader
// happens to be standing in.
func TestResolveRefIsColonyRootRelative(t *testing.T) {
	root, s := store(t)
	ref := s.Ref("agent-1")

	abs, err := patches.ResolveRef(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".paseka", "runs", "trace-1", "proposals", "agent-1.patch")
	if abs != want {
		t.Fatalf("abs = %q, want %q", abs, want)
	}
	if !patches.IsArchiveRef(ref) {
		t.Fatalf("%q should parse as an archive ref", ref)
	}
}

// A historical object-store ref and an explicitly authored ref keep their own meaning: they
// are not archive paths, and no reader may turn them into one.
func TestIsArchiveRefRejectsLegacyAndForeignRefs(t *testing.T) {
	for _, ref := range []string{
		"",
		"trace-01a0-agent-01.diff",
		"notes/diff.diff",
		".paseka/runs/trace-1/artifacts/notes.md",
		".paseka/runs/trace-1/proposals/agent-1.json",
		".paseka/runs/trace-1/proposals/nested/agent-1.patch",
		"runs/trace-1/proposals/agent-1.patch",
		"../../etc/passwd",
	} {
		if patches.IsArchiveRef(ref) {
			t.Fatalf("%q must not be treated as an archive ref", ref)
		}
	}
}

// A ref carries its own trail, so resolving another trail's ref reads that trail's store
// and cannot be used to reach a record in this one.
func TestResolveRefRefusesAnotherTrailsRecord(t *testing.T) {
	root, here := store(t)
	if _, err := here.Write(record("agent-1"), []byte("here")); err != nil {
		t.Fatal(err)
	}
	other := patches.NewStore(root, "trace-2")

	ref := other.Ref("agent-1")
	abs, err := patches.ResolveRef(root, ref)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(abs, "trace-1") {
		t.Fatalf("ref resolved into the wrong trail: %s", abs)
	}
	if _, ok, err := other.Read("agent-1"); err != nil || ok {
		t.Fatalf("another trail must not see this record: ok=%v err=%v", ok, err)
	}
}

func TestResolveRefRefusesTraversalAndAbsoluteEscapes(t *testing.T) {
	root, _ := store(t)
	for _, ref := range []string{
		".paseka/runs/trace-1/proposals/../../../../etc/passwd.patch",
		"/etc/passwd",
		".paseka/runs/../runs/trace-1/proposals/../proposals/x.patch",
	} {
		if _, err := patches.ResolveRef(root, ref); err == nil {
			t.Fatalf("%q must be refused", ref)
		}
	}
}

func TestResolveRefRefusesASymlinkEscape(t *testing.T) {
	_, s := store(t)
	if err := os.MkdirAll(s.Root(), 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.patch"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.patch"), s.PatchPath("agent-1")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := s.ReadContent("agent-1", 1024); err == nil {
		t.Fatal("a symlinked body must not be readable through the store")
	}
}

func TestAgentIdCannotEscapeTheStore(t *testing.T) {
	_, s := store(t)
	if _, err := s.Write(record("../../escape"), []byte("body")); err == nil {
		t.Fatal("a traversing agent id must be refused")
	}
	if _, err := s.ReadContent("../../escape", 1024); err == nil {
		t.Fatal("a traversing agent id must not resolve")
	}
}

func TestFileCountFromDiff(t *testing.T) {
	if got := patches.FileCountFromDiff("no headers here"); got != nil {
		t.Fatalf("fileCount = %v, want nil for a diff with no headers", got)
	}
	diff := "diff --git a/a b/a\n+1\ndiff --git a/b b/b\n+2\n"
	got := patches.FileCountFromDiff(diff)
	if got == nil || *got != 2 {
		t.Fatalf("fileCount = %v, want 2", got)
	}
}
