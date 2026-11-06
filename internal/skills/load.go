package skills

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UsedFileName is the per-run evidence log a load appends to. It lives beside
// the run's other records, so the trail's cleanup owns it.
const UsedFileName = "skills-used.ndjson"

// UsageRecord is one observed load. ContentHash is the SHA-256 of the served
// SKILL.md bytes — frontmatter included — so it identifies the file as served.
// It is deliberately not the install-time procedure hash of the skill's
// provenance: one identifies what a run opened, the other what was approved.
type UsageRecord struct {
	Name        string `json:"name"`
	ContentHash string `json:"contentHash"`
	At          string `json:"at"`
}

// NotOfferedError reports a reference the run's role does not carry. A pattern
// lands here too: it names a set, and load serves one procedure. The command
// layer maps this error to exit 2 so an agent can tell "you were not offered
// this" from "the colony is broken".
type NotOfferedError struct {
	Reference string
	Reason    string
	Role      string
}

func (e *NotOfferedError) Error() string {
	role := ""
	if e.Role != "" {
		role = fmt.Sprintf(" for role %q", e.Role)
	}
	return fmt.Sprintf(
		"skill_not_offered: %q%s: %s",
		e.Reference, role, e.Reason,
	)
}

// Offered resolves one reference against a run's effective set. It never reads
// the store: what a run was offered is the frozen resolution, so loading cannot
// disagree with the advertisement the run received. An invalid reference, a
// pattern, and a name outside the set are all refusals.
func Offered(run RunResolution, raw string) (Skill, error) {
	ref, err := ParseReference(raw)
	if err != nil {
		return Skill{}, &NotOfferedError{
			Reference: raw,
			Reason:    err.Error(),
			Role:      run.Role,
		}
	}
	if ref.IsPattern() {
		return Skill{}, &NotOfferedError{
			Reference: raw,
			Reason:    "a pattern names a set of procedures; load serves one",
			Role:      run.Role,
		}
	}
	for _, skill := range run.Skills {
		if skill.Name == ref.String() {
			return skill, nil
		}
	}
	return Skill{}, &NotOfferedError{
		Reference: raw,
		Reason:    "the run's role was not offered this procedure",
		Role:      run.Role,
	}
}

// Served reads one skill's SKILL.md as it is on disk, returns its body without
// frontmatter, and hashes the bytes it served. The body and the hash come from
// the same read, so a record cannot describe content the caller did not get.
func Served(skill Skill) (body, contentHash string, err error) {
	data, err := os.ReadFile(skill.Path)
	if err != nil {
		return "", "", fmt.Errorf("skills: read %s: %w", skill.Path, err)
	}
	_, body, reason := parseDocument(data)
	if reason != "" {
		return "", "", fmt.Errorf("skills: serve %s: %s", skill.Path, reason)
	}
	sum := sha256.Sum256(data)
	return body, hex.EncodeToString(sum[:]), nil
}

// AppendUsage adds one line to a run's load log. The line is written with a
// single append, so two concurrent loads cannot interleave bytes within a
// record; the caller owns the run directory.
func AppendUsage(path string, record UsageRecord) error {
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("skills: usage record needs a name")
	}
	if record.At == "" {
		return fmt.Errorf("skills: usage record needs a timestamp")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("skills: create run record directory: %w", err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("skills: encode usage record: %w", err)
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("skills: append %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("skills: append %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("skills: append %s: %w", path, err)
	}
	return nil
}

// ReadUsage reads a run's load log. A missing log is no loads, not an error.
// Every line is one record, so a damaged line names where it was found rather
// than silently dropping a load.
func ReadUsage(path string) ([]UsageRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skills: read %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	var records []UsageRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var record UsageRecord
		if err := json.Unmarshal([]byte(text), &record); err != nil {
			return nil, fmt.Errorf("skills: %s line %d is corrupt: %w", path, line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("skills: read %s: %w", path, err)
	}
	return records, nil
}

// UsedNames folds load records into the run's used set: deduplicated by name,
// with the first load winning. The first occurrence also fixes the order, so
// the recorded set is stable for a given run.
func UsedNames(records []UsageRecord) []string {
	seen := make(map[string]bool, len(records))
	names := make([]string, 0, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}
