package export

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/hiveview"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/skills"
)

// maxUsageToolInputRunes lets attribution see a whole provider tool input. The
// Agent log already asks for the same ceiling so its summary key cannot fall
// past the transcript's generic cap; usage reads the raw input under the same
// bound.
const maxUsageToolInputRunes = maxAgentLogToolInputRunes

// SkillUseExport is one procedure a run reached for, with the evidence that
// attributed it and the body from the run's frozen copy.
type SkillUseExport struct {
	Name     string
	Evidence []string
	Body     string
	// Omitted says why the body could not be read from the frozen copy.
	Omitted string
}

// runUsage is the used half of one run's skills export.
type runUsage struct {
	Used    []SkillUseExport
	None    bool
	Omitted string
	// Note qualifies a partial measurement, for example a used set attributed
	// only from load records because the adapter has no transcript capability.
	Note string
}

// resolveRunUsage builds a run's used set as the union of transcript evidence
// and its load records, deduplicated by name. Transcript evidence wins the
// order because it is read in provider order; the first load of a name supplies
// the load evidence and later loads are ignored.
//
// A run whose usage cannot be determined reports the capability's own reason
// instead of an empty set. Absence of evidence is not absence of use.
func resolveRunUsage(colonyRoot, traceID string, run hiveview.RunView, resolved []skills.Skill, configProblem string, lookup TranscriptLookup, timeout time.Duration) runUsage {
	type entry struct {
		name      string
		evidence  []string
		firstLoad *skills.UsageRecord
	}

	entries := make(map[string]*entry)
	var order []string
	ensure := func(name string) *entry {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil
		}
		if existing, ok := entries[name]; ok {
			return existing
		}
		created := &entry{name: name}
		entries[name] = created
		order = append(order, name)
		return created
	}
	addEvidence := func(name, evidence string) {
		if e := ensure(name); e != nil {
			e.evidence = append(e.evidence, evidence)
		}
	}

	transcript := resolveUsageTranscript(run, lookup, timeout)
	if transcript.Omitted == "" {
		for _, input := range adapters.ProjectToolInputs(transcript) {
			for _, skill := range resolved {
				if rel, ok := skillFileInToolInput(input.Input, skill, colonyRoot); ok {
					addEvidence(skill.Name, fmt.Sprintf("transcript: %s read %s", input.Name, rel))
				}
			}
		}
	}

	loads, loadErr := skills.ReadUsage(runs.Dir{
		ColonyRoot: colonyRoot,
		TraceID:    traceID,
		AgentID:    run.AgentID,
	}.SkillsUsedPath())
	if loadErr == nil {
		for i := range loads {
			record := loads[i]
			name := strings.TrimSpace(record.Name)
			if name == "" {
				continue
			}
			e := ensure(name)
			if e == nil || e.firstLoad != nil {
				// The first load of a name wins; a repeat adds no evidence.
				continue
			}
			e.firstLoad = &record
			e.evidence = append(e.evidence, loadEvidence(record))
		}
	}

	byName := make(map[string]skills.Skill, len(resolved))
	for _, skill := range resolved {
		byName[skill.Name] = skill
	}

	out := runUsage{}
	for _, name := range order {
		e := entries[name]
		use := SkillUseExport{Name: name, Evidence: e.evidence}
		skill, ok := byName[name]
		switch {
		case configProblem != "":
			use.Omitted = "the run's frozen configuration is unavailable: " + configProblem
		case !ok:
			use.Omitted = "the run's frozen configuration does not resolve this procedure"
		default:
			body, _, err := skills.Served(skill)
			if err != nil {
				use.Omitted = "body unavailable: " + err.Error()
			} else {
				use.Body = strings.TrimLeft(body, "\n")
			}
		}
		out.Used = append(out.Used, use)
	}

	switch {
	case len(out.Used) > 0:
		var notes []string
		if transcript.Omitted != "" {
			notes = append(notes, fmt.Sprintf("transcript usage unavailable (%s); the used set comes from load records only", transcript.Omitted))
		}
		if loadErr != nil {
			notes = append(notes, fmt.Sprintf("load records unreadable (%v); the used set comes from the transcript only", loadErr))
		}
		out.Note = strings.Join(notes, "; ")
	case loadErr != nil:
		out.Omitted = fmt.Sprintf("load records unreadable: %v", loadErr)
	case transcript.Omitted != "":
		out.Omitted = transcript.Omitted
	case configProblem != "":
		out.Omitted = configProblem
	default:
		out.None = true
	}
	return out
}

// resolveUsageTranscript reads the run's transcript for attribution. It shares
// the resolved-transcript contract with the Agent log but asks for the raw tool
// input only, so the Agent log's summarized rows are not involved.
func resolveUsageTranscript(run hiveview.RunView, lookup TranscriptLookup, timeout time.Duration) adapters.Transcript {
	if run.ProviderSessionID == "" {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedNoProviderSessionID}
	}
	if lookup == nil {
		lookup = defaultTranscriptLookup
	}
	resolver := lookup(run.Adapter)
	if resolver == nil {
		return adapters.Transcript{Omitted: adapters.SessionLogOmittedUnsupported}
	}
	if timeout <= 0 {
		timeout = defaultTranscriptTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	limits := adapters.DefaultTranscriptLimits()
	limits.ToolInput = maxUsageToolInputRunes
	return adapters.ResolveTranscript(ctx, resolver, adapters.TranscriptRequest{
		ProviderSessionID: run.ProviderSessionID,
		Adapter:           run.Adapter,
		Workspace:         run.Workspace,
		ColonyRoot:        run.ColonyRoot,
		RunDir:            run.RunDir,
		AgentID:           run.AgentID,
		Include:           adapters.PresetToolCall(),
		Limits:            limits,
		Window:            adapters.WholeTranscriptWindow(),
	})
}

func loadEvidence(record skills.UsageRecord) string {
	when := strings.TrimSpace(record.At)
	hash := strings.TrimSpace(record.ContentHash)
	switch {
	case when != "" && hash != "":
		return fmt.Sprintf("load record: served at %s (sha256 %s)", when, hash)
	case when != "":
		return "load record: served at " + when
	case hash != "":
		return fmt.Sprintf("load record: served (sha256 %s)", hash)
	default:
		return "load record: served"
	}
}

// skillFileInToolInput reports whether a raw tool input names a file under the
// skill's frozen location: its SKILL.md, a scripts/ entry, or a references/ or
// assets/ file. The match is against this run's resolved location only, so one
// run's evidence cannot attribute another run's copy of the same skill.
func skillFileInToolInput(raw string, skill skills.Skill, colonyRoot string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	prefixes := []string{usagePath(filepath.Clean(skill.Dir))}
	if colonyRoot != "" {
		if rel, err := filepath.Rel(colonyRoot, skill.Dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			prefixes = append(prefixes, usagePath(rel))
		}
	}
	for _, prefix := range prefixes {
		if prefix == "" || prefix == "." {
			continue
		}
		if rel, ok := findSkillFile(raw, prefix); ok {
			return rel, true
		}
	}
	return "", false
}

// findSkillFile scans text for one frozen skill directory prefix and returns
// the relative file that follows it. The prefix carries the trail and snapshot
// identifiers, so a hit names this run's copy rather than another run's.
func findSkillFile(text, prefix string) (string, bool) {
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return "", false
	}
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], prefix)
		if index < 0 {
			return "", false
		}
		start := offset + index
		after := start + len(prefix)
		if after < len(text) && text[after] == '/' {
			rest := text[after+1:]
			candidate := quotedUsagePath(text, start, rest)
			if candidate == "" {
				candidate = trimUsagePath(rest)
			}
			if isSkillUsageFile(candidate) {
				return candidate, true
			}
		}
		offset = after
	}
	return "", false
}

// quotedUsagePath returns a path that starts inside a quote or JSON string. A
// supporting file may contain a space, and the quote says where the token ends
// without making the scanner guess.
func quotedUsagePath(text string, start int, rest string) string {
	if start == 0 {
		return ""
	}
	quote := text[start-1]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(rest, quote)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// trimUsagePath keeps the path-shaped prefix of a string and stops at the first
// delimiter, so a JSON value, a shell command, or a quoted argument yields the
// same file token.
func trimUsagePath(s string) string {
	end := 0
	for end < len(s) {
		c := s[end]
		if !isUsagePathByte(c) {
			break
		}
		end++
	}
	return s[:end]
}

func isUsagePathByte(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '/', c == '.', c == '-', c == '_', c == '+', c == '@', c == '~':
		return true
	default:
		return false
	}
}

func isSkillUsageFile(rel string) bool {
	rel = strings.TrimPrefix(path.Clean(strings.TrimPrefix(rel, "/")), "./")
	if rel == "" || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return false
	}
	if rel == skills.ManifestName {
		return true
	}
	first, rest, found := strings.Cut(rel, "/")
	if !found || rest == "" {
		return false
	}
	switch first {
	case "scripts", "references", "assets":
		return true
	default:
		return false
	}
}

func usagePath(p string) string {
	return strings.TrimRight(filepath.ToSlash(p), "/")
}
