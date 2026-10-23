package patches

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/russ-p/paseka/internal/protocol"
)

// Record is the sidecar for one proposal attempt: what was captured, from which run, and
// what is known about publication of its synthesized event.
//
// Content identity and capture metadata do not change once the record is installed.
// Publication status may be updated atomically on its own.
type Record struct {
	protocol.Provenance
	SchemaVersion int    `json:"schemaVersion"`
	TraceID       string `json:"traceId"`
	AgentID       string `json:"agentId"`
	Bee           string `json:"bee,omitempty"`
	// Mode is "afk" or "hitl".
	Mode   string `json:"mode,omitempty"`
	TaskID string `json:"taskId,omitempty"`
	// Kind is the normalized proposal kind the run was allowed to auto-publish.
	Kind string `json:"kind,omitempty"`
	// Workspace is the proposal workspace the run used: "isolated" or "root".
	Workspace    string     `json:"workspace,omitempty"`
	WorktreePath string     `json:"worktreePath,omitempty"`
	BaseSha      string     `json:"baseSha,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	Sector       string     `json:"sector,omitempty"`
	Bytes        int64      `json:"bytes"`
	FileCount    *int       `json:"fileCount,omitempty"`
	CapturedAt   time.Time  `json:"capturedAt"`
	Publication  string     `json:"publication"`
	PublishedAt  *time.Time `json:"publishedAt,omitempty"`
	PublishError string     `json:"publishError,omitempty"`
	// Event is the synthesized MUTATION envelope, retained so the record is
	// self-describing on its own after a bus purge. It is not merged into event feeds
	// and no surface should consult it for ordering, attribution, or completeness.
	Event *protocol.Event `json:"event,omitempty"`
}

// Ref returns the colony-root-relative patch path for this record's run.
func (r Record) Ref(colonyRoot string) string {
	return Store{ColonyRoot: colonyRoot, TraceID: r.TraceID}.Ref(r.AgentID)
}

// Content holds a bounded read of a patch body plus the reason any bytes were withheld.
type Content struct {
	Bytes int64
	// Body is at most the cap passed to ReadContent, and is empty whenever Omitted is set.
	Body []byte
	// Truncated is set when Body is a strict prefix of the retained file.
	Truncated bool
	Omitted   string
}

// ListResult is one trail's proposal history plus diagnostics for records that exist on
// disk but are not readable as complete records. A bad entry never hides good ones.
type ListResult struct {
	Records     []Record
	Diagnostics []Diagnostic
}

// Diagnostic explains why something in the store is not an ordinary listing entry.
type Diagnostic struct {
	AgentID string `json:"agentId"`
	Reason  string `json:"reason"`
}

// Write installs one proposal record: the immutable diff body and then its sidecar.
//
// The first installed patch is never overwritten. Retrying the same finalization reuses
// the complete existing record instead of recomputing bytes or event identity, so a retry
// cannot silently replace the history of a run. A conflicting create is an error.
func (s Store) Write(rec Record, body []byte) (Record, error) {
	agentID := validAgentID(rec.AgentID)
	if agentID == "" {
		return Record{}, fmt.Errorf("patches: agent id is required")
	}
	if strings.TrimSpace(s.TraceID) == "" {
		return Record{}, fmt.Errorf("patches: trace id is required")
	}
	rec.SchemaVersion = SchemaVersion
	rec.TraceID = s.TraceID
	rec.AgentID = agentID
	rec.Bytes = int64(len(body))
	if rec.CapturedAt.IsZero() {
		rec.CapturedAt = time.Now().UTC()
	}
	if rec.Publication == "" {
		rec.Publication = PublicationUnpublished
	}

	if existing, ok, err := s.Read(agentID); err == nil && ok {
		// An identical retry keeps the original capture time and event envelope.
		return existing, nil
	} else if err != nil {
		return Record{}, err
	}

	// An orphan patch from an interrupted write is finished by installing its sidecar,
	// never by rewriting its bytes.
	if _, size, ok := s.orphanPatch(agentID); ok {
		if size != rec.Bytes {
			return Record{}, fmt.Errorf("patches: conflicting patch for %s: have %d bytes, want %d", agentID, size, rec.Bytes)
		}
		if err := s.writeMeta(rec); err != nil {
			return Record{}, err
		}
		return rec, nil
	}

	dir, err := s.ensureDir()
	if err != nil {
		return Record{}, err
	}
	tmpPatch := filepath.Join(dir, fmt.Sprintf(".%s.%d%s.tmp", agentID, os.Getpid(), patchSuffix))
	if err := writeNewFile(tmpPatch, body); err != nil {
		return Record{}, err
	}

	patchPath, err := s.resolveAgent(agentID)
	if err != nil {
		_ = os.Remove(tmpPatch)
		return Record{}, err
	}
	if err := linkOrCreate(tmpPatch, patchPath); err != nil {
		_ = os.Remove(tmpPatch)
		return Record{}, fmt.Errorf("patches: install patch for %s: %w", agentID, err)
	}
	_ = os.Remove(tmpPatch)

	if err := s.writeMeta(rec); err != nil {
		return Record{}, err
	}
	return rec, nil
}

// MarkPublication atomically records what is known about the synthesized event for an
// installed record. Content identity is untouched. A read-back failure is returned so a
// caller can report that confirmation could not be persisted.
func (s Store) MarkPublication(agentID, state string, confirmedAt time.Time, publishErr string) error {
	rec, ok, err := s.Read(agentID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("patches: no proposal record for %s", agentID)
	}
	switch state {
	case PublicationUnpublished, PublicationUnconfirmed, PublicationPublished:
	default:
		return fmt.Errorf("patches: unknown publication state %q", state)
	}
	rec.Publication = state
	rec.PublishError = strings.TrimSpace(publishErr)
	if state == PublicationPublished && !confirmedAt.IsZero() {
		rec.PublishedAt = &confirmedAt
	}
	if state != PublicationPublished {
		rec.PublishedAt = nil
	}
	return s.writeMeta(rec)
}

// Read returns one complete record. A record is complete when its sidecar parses and the
// matching regular patch file exists with the recorded size.
func (s Store) Read(agentID string) (Record, bool, error) {
	agentID = validAgentID(agentID)
	if agentID == "" {
		return Record{}, false, nil
	}
	metaPath, err := s.resolveMeta(agentID)
	if err != nil {
		return Record{}, false, err
	}
	rec, err := readMeta(metaPath)
	if err != nil {
		return Record{}, false, err
	}
	if rec == nil {
		return Record{}, false, nil
	}
	patchPath, err := s.resolveAgent(agentID)
	if err != nil {
		return Record{}, false, err
	}
	size, present, err := regularFileSize(patchPath)
	if err != nil {
		return Record{}, false, err
	}
	if !present || size != rec.Bytes {
		return Record{}, false, fmt.Errorf("patches: record %s does not match its patch body", agentID)
	}
	return *rec, true, nil
}

// List returns every complete record for the trail, oldest capture first with agent id as
// tie-breaker. A missing store is an empty list, not an error: trails that predate this
// spec read as absence.
func (s Store) List() (ListResult, error) {
	var out ListResult
	dir, err := s.resolveRoot()
	if err != nil {
		return out, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{
				AgentID: name,
				Reason:  "proposal store contains a subdirectory",
			})
			continue
		}
		if strings.HasSuffix(name, patchSuffix) && !strings.HasPrefix(name, ".") {
			agentID := strings.TrimSuffix(name, patchSuffix)
			if validAgentID(agentID) != agentID {
				continue
			}
			if _, err := os.Stat(s.MetaPath(agentID)); os.IsNotExist(err) {
				out.Diagnostics = append(out.Diagnostics, Diagnostic{
					AgentID: agentID,
					Reason:  "incomplete record: patch has no sidecar",
				})
			}
			continue
		}
		if !strings.HasSuffix(name, metaSuffix) || strings.HasPrefix(name, ".") {
			continue
		}
		agentID := strings.TrimSuffix(name, metaSuffix)
		if validAgentID(agentID) != agentID {
			continue
		}
		rec, ok, err := s.Read(agentID)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{AgentID: agentID, Reason: err.Error()})
			continue
		}
		if !ok {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{
				AgentID: agentID,
				Reason:  "incomplete record: no readable patch body",
			})
			continue
		}
		out.Records = append(out.Records, rec)
	}
	sortRecords(out.Records)
	return out, nil
}

// ReadContent reads at most maxBytes of a patch body, stat-ing first so an oversized file
// is refused without allocating it and using a bounded read even so, because the file can
// grow between the two. UTF-8 validation applies only to what was actually read.
func (s Store) ReadContent(agentID string, maxBytes int64) (Content, error) {
	agentID = validAgentID(agentID)
	if agentID == "" {
		return Content{}, fmt.Errorf("patches: agent id is required")
	}
	abs, err := s.resolveAgent(agentID)
	if err != nil {
		return Content{}, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return Content{}, err
	}
	if !info.Mode().IsRegular() {
		return Content{}, fmt.Errorf("patches: %s is not a regular file", agentID)
	}
	out := Content{Bytes: info.Size()}
	if maxBytes > 0 && info.Size() > maxBytes {
		out.Omitted = fmt.Sprintf("patch too large for inline preview (%d bytes)", info.Size())
		return out, nil
	}

	f, err := os.Open(abs)
	if err != nil {
		return Content{}, err
	}
	defer f.Close()

	limit := info.Size()
	if maxBytes > 0 && limit > maxBytes {
		limit = maxBytes
		out.Truncated = true
	}
	buf := make([]byte, 0, limit)
	if limit > 0 {
		reader := io.LimitReader(f, limit+1)
		chunk, err := io.ReadAll(reader)
		if err != nil {
			return Content{}, err
		}
		if int64(len(chunk)) > limit {
			chunk = chunk[:limit]
			out.Truncated = true
		}
		buf = chunk
	}
	if !IsTextContent(buf) {
		out.Body = nil
		out.Omitted = "binary or invalid UTF-8"
		return out, nil
	}
	out.Body = buf
	return out, nil
}

// IsTextContent reports whether data is valid UTF-8 without NUL bytes. Arbitrary diff
// output is not guaranteed to be valid UTF-8, so text surfaces omit unsuitable content
// with a reason while preserving its size and ref.
func IsTextContent(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	return !strings.Contains(string(data), "\x00") && utf8.Valid(data)
}

// FileCountFromDiff counts the files in a unified diff by its "diff --git" headers. It is
// best effort: a diff the collector truncated or reformatted may not carry every header.
func FileCountFromDiff(diff string) *int {
	count := strings.Count(diff, "diff --git ")
	if count == 0 {
		return nil
	}
	return &count
}

func (s Store) orphanPatch(agentID string) (string, int64, bool) {
	path, err := s.resolveAgent(agentID)
	if err != nil {
		return "", 0, false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, false
	}
	return path, info.Size(), true
}

func (s Store) ensureDir() (string, error) {
	dir, err := s.resolveRoot()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("patches: mkdir %s: %w", dir, err)
	}
	// Re-resolve after creation: a component swapped for a symlink must not be followed.
	return s.resolveRoot()
}

// writeMeta installs the sidecar atomically. The sidecar is the visibility marker, so it
// is written last and replaced whole.
func (s Store) writeMeta(rec Record) error {
	dir, err := s.ensureDir()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	target, err := s.resolveMeta(rec.AgentID)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, fmt.Sprintf(".%s.%d.tmp", rec.AgentID, os.Getpid()))
	if err := writeNewFile(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("patches: install sidecar for %s: %w", rec.AgentID, err)
	}
	return nil
}

func readMeta(path string) (*Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var rec Record
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("patches: malformed sidecar %s: %w", filepath.Base(path), err)
	}
	if validAgentID(rec.AgentID) != rec.AgentID {
		return nil, fmt.Errorf("patches: malformed sidecar %s: agent id", filepath.Base(path))
	}
	if rec.Publication == "" {
		rec.Publication = PublicationUnpublished
	}
	return &rec, nil
}

func regularFileSize(path string) (int64, bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if !info.Mode().IsRegular() {
		return 0, false, nil
	}
	return info.Size(), true, nil
}

func sortRecords(records []Record) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && recordLess(records[j], records[j-1]); j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}

func recordLess(a, b Record) bool {
	if !a.CapturedAt.Equal(b.CapturedAt) {
		return a.CapturedAt.Before(b.CapturedAt)
	}
	if a.AgentID != b.AgentID {
		return a.AgentID < b.AgentID
	}
	return a.TraceID < b.TraceID
}

func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		_ = os.Remove(path)
		return err
	}
	return f.Sync()
}

// linkOrCreate installs src at dst only when dst is free, so two concurrent writers for
// the same run cannot replace each other's bytes. Either name wins; the loser is reported.
func linkOrCreate(src, dst string) error {
	err := os.Link(src, dst)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	return fmt.Errorf("patch already installed at %s", filepath.Base(dst))
}

// containPath reports whether path stays inside root after symlinks are resolved.
func containPath(root, path string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(rootAbs); err == nil {
		rootAbs = resolved
	}
	candidate := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		candidate = resolved
	} else if parent := filepath.Dir(path); parent != path {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			candidate = filepath.Join(resolved, filepath.Base(path))
		}
	}
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("patches: %s escapes the proposal store", filepath.Base(path))
	}
	return nil
}
