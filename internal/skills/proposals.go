package skills

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

const (
	// ProposalsDirName is the runtime-owned directory under a trail's run root.
	// It holds the decisions taken about that trail's proposals and nothing a
	// Bee writes. It shares its name with the artifact comb's bundle directory
	// (`artifacts/skill-proposals/`), but the two live under different roots:
	// the comb holds the proposed bytes, this holds the Beekeeper's verdict.
	ProposalsDirName = "skill-proposals"
	// DecisionsFileName is the append-only decision log inside that directory.
	DecisionsFileName = "decisions.ndjson"
	// ProposalStoreDestination names the shared store as a decision's
	// destination. Any other destination is a trail id, used by a trial.
	ProposalStoreDestination = "store"

	// ProposalPending means the discovery has no decision yet.
	ProposalPending = "pending"
	// ProposalInstalled means the bundle was approved into the shared store.
	ProposalInstalled = "installed"
	// ProposalRefused means a Beekeeper turned the exact bundle down.
	ProposalRefused = "refused"
	// ProposalTrialled means the bundle was applied to one trail's candidate
	// configuration, with that trail as the destination.
	ProposalTrialled = "trialled"
	// ProposalMissing means the discovery exists but its bundle is gone.
	ProposalMissing = "missing"

	maxDecisionLineBytes = 1 << 20
)

// ProposalDecision is one line in a trail's decision log. The runtime alone
// writes it, and it is keyed by the trail, the skill, and the bundle's
// procedure hash: a changed bundle is a new proposal, so an earlier decision
// about the same name does not apply to it.
type ProposalDecision struct {
	Skill         string    `json:"skill"`
	Trail         string    `json:"trail"`
	ProcedureHash string    `json:"procedureHash"`
	Destination   string    `json:"destination"`
	Decision      string    `json:"decision"`
	Reason        string    `json:"reason,omitempty"`
	Roles         []string  `json:"roles,omitempty"`
	At            time.Time `json:"at"`
}

// ProposalDecisionsPath returns the machine-local decision log for one trail.
// It lives under `.paseka/runs/`, so it is not shared through git and it
// disappears with the trail.
func ProposalDecisionsPath(colonyRoot, traceID string) string {
	return filepath.Join(colonyRoot, ".paseka", "runs", traceID, ProposalsDirName, DecisionsFileName)
}

// ProposalIssue is one finding from the proposal scan. Trail is set when the
// finding belongs to one trail; Reason is the sentence the operator reads.
type ProposalIssue struct {
	Trail  string
	Reason string
}

// Error renders a finding the way the command prints it. A trail-scoped issue
// names its trail, because a proposal inventory spans many.
func (i ProposalIssue) Error() string {
	if i.Trail == "" {
		return i.Reason
	}
	return fmt.Sprintf("trail %s: %s", i.Trail, i.Reason)
}

// Proposal is one discovery the trails hold, with the verdict its bundle and
// its decisions earned. Valid is true only when the bundle exists, passes the
// one skill rule set, every entry is a regular file, and the recomputed
// procedure hash matches the discovery.
type Proposal struct {
	Trail         string
	Name          string
	Description   string
	ProposedAt    time.Time
	Bundle        string
	ProcedureHash string
	// RecomputedHash is the hash read from the bundle now. It is empty when the
	// bundle is missing or its SKILL.md cannot be parsed.
	RecomputedHash string
	Valid          bool
	Validation     []string
	// BundleMissing reports that the discovery's bundle is no longer on disk.
	BundleMissing bool
	State         string
	Reason        string
	Destination   string
	DecidedAt     time.Time
	// Decisions is the newest decision per destination, so a trial and a later
	// store install are both visible without replaying every line.
	Decisions []ProposalDecision
}

// ProposalInventory is one scan of every trail's proposals.
type ProposalInventory struct {
	Proposals []Proposal
	Issues    []ProposalIssue
}

// ProposalReference names one proposal. A bare name is the skill name and
// matches it in every trail; a `<trail>/<name>` reference names one trail.
type ProposalReference struct {
	Trail string
	Name  string
}

// String returns the reference as it would be written.
func (r ProposalReference) String() string {
	if r.Trail == "" {
		return r.Name
	}
	return r.Trail + "/" + r.Name
}

// ParseProposalReference validates a proposal reference. defaultTrail supplies
// the trail for a bare name; a qualified reference that names a different trail
// is refused rather than silently preferring one of the two.
func ParseProposalReference(raw, defaultTrail string) (ProposalReference, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ProposalReference{}, fmt.Errorf(
			"invalid proposal reference %q: a proposal reference is <skill> or <trail>/<skill>", raw,
		)
	}
	var ref ProposalReference
	if strings.Contains(text, "/") {
		parts := strings.Split(text, "/")
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return ProposalReference{}, fmt.Errorf(
				"invalid proposal reference %q: a proposal reference is <skill> or <trail>/<skill>", raw,
			)
		}
		trail := strings.TrimSpace(parts[0])
		if !validTrailToken(trail) {
			return ProposalReference{}, fmt.Errorf(
				"invalid proposal reference %q: %q is not a trail id", raw, trail,
			)
		}
		if defaultTrail != "" && defaultTrail != trail {
			return ProposalReference{}, fmt.Errorf(
				"invalid proposal reference %q: it names trail %s but --trail is %s", raw, trail, defaultTrail,
			)
		}
		ref.Trail = trail
		text = strings.TrimSpace(parts[1])
	} else {
		ref.Trail = defaultTrail
	}
	name, err := ParseReference(text)
	if err != nil {
		return ProposalReference{}, fmt.Errorf("invalid proposal reference %q: %w", raw, err)
	}
	if name.IsPattern() {
		return ProposalReference{}, fmt.Errorf(
			"invalid proposal reference %q: proposals are named one at a time, not by pattern", raw,
		)
	}
	ref.Name = name.String()
	return ref, nil
}

// Resolve returns the rows one reference names. A bare name that matches more
// than one trail is an error naming the candidates, because a proposal is a
// trail's output and choosing between two trails is not a decision a listing
// can make.
func (inv ProposalInventory) Resolve(ref ProposalReference) ([]Proposal, error) {
	var matched []Proposal
	for _, proposal := range inv.Proposals {
		if proposal.Name != ref.Name {
			continue
		}
		if ref.Trail != "" && proposal.Trail != ref.Trail {
			continue
		}
		matched = append(matched, proposal)
	}
	if len(matched) == 0 {
		if ref.Trail != "" {
			return nil, fmt.Errorf("skills: no proposal %q in trail %s", ref.Name, ref.Trail)
		}
		return nil, fmt.Errorf("skills: no proposal %q in any trail", ref.Name)
	}
	if ref.Trail == "" && len(matched) > 1 {
		trails := make([]string, 0, len(matched))
		qualified := make([]string, 0, len(matched))
		for _, proposal := range matched {
			trails = append(trails, proposal.Trail)
			qualified = append(qualified, proposal.Trail+"/"+proposal.Name)
		}
		sort.Strings(trails)
		sort.Strings(qualified)
		return nil, fmt.Errorf(
			"skills: proposal %q is in %d trails (%s); use one of %s, or --trail",
			ref.Name, len(trails), strings.Join(trails, ", "), strings.Join(qualified, ", "),
		)
	}
	return matched, nil
}

// ScanProposals reads every trail's proposal discoveries and decisions.
//
// A discovery is an `INSIGHT/skill.proposed` event; a discovery with no
// applicable decision is pending, a decision makes it installed, refused, or
// trialled, and a bundle that has disappeared is missing rather than pending.
// An unreadable trail, event record, or decision file is reported, never
// silently skipped. The scan reads the filesystem only: no broker is required.
func ScanProposals(colonyRoot string) (ProposalInventory, error) {
	runsRoot := filepath.Join(colonyRoot, ".paseka", "runs")
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return ProposalInventory{}, nil
		}
		return ProposalInventory{}, fmt.Errorf("skills: read trails in %s: %w", runsRoot, err)
	}

	inventory := ProposalInventory{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		traceID := entry.Name()
		inventory.Proposals = append(inventory.Proposals, scanTrailProposals(colonyRoot, traceID, &inventory.Issues)...)
	}
	sort.SliceStable(inventory.Proposals, func(i, j int) bool {
		if inventory.Proposals[i].Trail != inventory.Proposals[j].Trail {
			return inventory.Proposals[i].Trail < inventory.Proposals[j].Trail
		}
		return inventory.Proposals[i].Name < inventory.Proposals[j].Name
	})
	sort.SliceStable(inventory.Issues, func(i, j int) bool {
		if inventory.Issues[i].Trail != inventory.Issues[j].Trail {
			return inventory.Issues[i].Trail < inventory.Issues[j].Trail
		}
		return inventory.Issues[i].Reason < inventory.Issues[j].Reason
	})
	return inventory, nil
}

// scanTrailProposals reads one trail and appends its findings to issues.
func scanTrailProposals(colonyRoot, traceID string, issues *[]ProposalIssue) []Proposal {
	events, readIssues, err := runs.ReadTraceEventsReport(colonyRoot, traceID)
	if err != nil {
		*issues = append(*issues, ProposalIssue{
			Trail:  traceID,
			Reason: fmt.Sprintf("read trail: %v", err),
		})
		return nil
	}
	for _, issue := range readIssues {
		*issues = append(*issues, ProposalIssue{
			Trail:  traceID,
			Reason: fmt.Sprintf("%s: %s", issue.Kind, issue.Detail),
		})
	}

	discoveries := map[string]discovery{}
	for _, event := range events {
		if event.Type != protocol.EventInsight || protocol.PayloadKind(event.Payload) != string(protocol.InsightSkillProposed) {
			continue
		}
		var payload protocol.SkillProposedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			*issues = append(*issues, ProposalIssue{
				Trail:  traceID,
				Reason: fmt.Sprintf("skill.proposed event %s: %v", event.ID, err),
			})
			continue
		}
		name := strings.TrimSpace(payload.Name)
		if name == "" {
			*issues = append(*issues, ProposalIssue{
				Trail:  traceID,
				Reason: fmt.Sprintf("skill.proposed event %s has no skill name", event.ID),
			})
			continue
		}
		if strings.TrimSpace(payload.Bundle) == "" {
			*issues = append(*issues, ProposalIssue{
				Trail:  traceID,
				Reason: fmt.Sprintf("skill.proposed event %s for %q has no bundle", event.ID, name),
			})
			continue
		}
		existing, ok := discoveries[name]
		// A bundle is addressed by the skill name inside the comb, so a
		// re-proposal overwrites it: the newest discovery is the proposal that
		// bundle now represents.
		if ok && event.CreatedAt.Before(existing.at) {
			continue
		}
		discoveries[name] = discovery{
			payload: payload,
			name:    name,
			at:      event.CreatedAt,
		}
	}

	decisions, decisionIssues, err := ReadProposalDecisions(ProposalDecisionsPath(colonyRoot, traceID), traceID)
	for _, reason := range decisionIssues {
		*issues = append(*issues, ProposalIssue{Trail: traceID, Reason: reason})
	}
	if err != nil {
		*issues = append(*issues, ProposalIssue{
			Trail:  traceID,
			Reason: fmt.Sprintf("read decisions: %v", err),
		})
		decisions = nil
	}

	trailRoot := runs.Trail{ColonyRoot: colonyRoot, TraceID: traceID}.Root()
	names := make([]string, 0, len(discoveries))
	for name := range discoveries {
		names = append(names, name)
	}
	sort.Strings(names)

	proposals := make([]Proposal, 0, len(names))
	for _, name := range names {
		discovered := discoveries[name]
		proposal := Proposal{
			Trail:         traceID,
			Name:          name,
			Description:   strings.TrimSpace(discovered.payload.Description),
			ProposedAt:    discovered.at,
			Bundle:        discovered.payload.Bundle,
			ProcedureHash: strings.TrimSpace(discovered.payload.ProcedureHash),
		}

		verdict := evaluateProposalBundle(trailRoot, proposal.Bundle)
		proposal.RecomputedHash = verdict.hash
		proposal.Validation = verdict.issues
		proposal.BundleMissing = !verdict.exists
		if verdict.exists && verdict.hash != "" {
			if proposal.ProcedureHash == verdict.hash {
				proposal.Valid = len(verdict.issues) == 0
			} else {
				proposal.Validation = append(proposal.Validation, fmt.Sprintf(
					"procedure hash mismatch: discovery says %s, the bundle hashes to %s",
					proposal.ProcedureHash, verdict.hash,
				))
			}
		}

		applicable := applicableDecisions(decisions, proposal)
		proposal.Decisions = newestDecisionPerDestination(applicable)
		proposal.State, proposal.Reason, proposal.Destination, proposal.DecidedAt = proposalState(
			applicable, proposal.Decisions, !verdict.exists,
		)
		proposals = append(proposals, proposal)
	}
	return proposals
}

// discovery is one proposal event, normalized.
type discovery struct {
	payload protocol.SkillProposedPayload
	name    string
	at      time.Time
}

// applicableDecisions returns the decision lines that name this exact proposal:
// same trail, same skill, same procedure hash. A changed bundle is a new
// proposal, so a decision about the earlier bytes does not apply to it.
func applicableDecisions(decisions []ProposalDecision, proposal Proposal) []ProposalDecision {
	var out []ProposalDecision
	for _, decision := range decisions {
		if decision.Trail != proposal.Trail || decision.Skill != proposal.Name {
			continue
		}
		if decision.ProcedureHash != proposal.ProcedureHash {
			continue
		}
		out = append(out, decision)
	}
	return out
}

// newestDecisionPerDestination keeps the newest line for each destination, so
// a proposal trialled on one trail and later installed into the store shows
// both facts rather than only the last line in the file.
func newestDecisionPerDestination(decisions []ProposalDecision) []ProposalDecision {
	byDestination := map[string]ProposalDecision{}
	for _, decision := range decisions {
		current, ok := byDestination[decision.Destination]
		if !ok || !decision.At.Before(current.At) {
			byDestination[decision.Destination] = decision
		}
	}
	out := make([]ProposalDecision, 0, len(byDestination))
	for _, decision := range byDestination {
		out = append(out, decision)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Destination < out[j].Destination })
	return out
}

// proposalState resolves the row's state. A refusal is terminal for the exact
// bundle, so it wins over any later line; otherwise the store install wins over
// a trial, and a discovery with no applicable decision is missing when its
// bundle is gone and pending when it is present.
func proposalState(applicable, newest []ProposalDecision, bundleMissing bool) (state, reason, destination string, decidedAt time.Time) {
	var refusal *ProposalDecision
	for i := range applicable {
		decision := applicable[i]
		if decision.Decision != ProposalRefused {
			continue
		}
		if refusal == nil || !decision.At.Before(refusal.At) {
			refusal = &applicable[i]
		}
	}
	if refusal != nil {
		return ProposalRefused, refusal.Reason, refusal.Destination, refusal.At
	}
	for _, decision := range newest {
		if decision.Decision == ProposalInstalled {
			return ProposalInstalled, "", decision.Destination, decision.At
		}
	}
	var trial *ProposalDecision
	for i := range newest {
		decision := newest[i]
		if decision.Decision != ProposalTrialled {
			continue
		}
		if trial == nil || !decision.At.Before(trial.At) {
			trial = &newest[i]
		}
	}
	if trial != nil {
		return ProposalTrialled, trial.Reason, trial.Destination, trial.At
	}
	if bundleMissing {
		return ProposalMissing, "", "", time.Time{}
	}
	return ProposalPending, "", "", time.Time{}
}

// bundleVerdict is the scan's answer about a proposal's bundle.
type bundleVerdict struct {
	exists bool
	hash   string
	issues []string
}

// evaluateProposalBundle validates a bundle and recomputes its procedure hash.
// A missing bundle is reported as such rather than as an invalid one: absence
// and corruption are different facts.
func evaluateProposalBundle(trailRoot, bundle string) bundleVerdict {
	clean, ok := protocol.CombRelativeBundle(bundle)
	if !ok {
		return bundleVerdict{exists: true, issues: []string{
			fmt.Sprintf("bundle %q is not a path inside the trail artifact comb", bundle),
		}}
	}
	dir := filepath.Join(trailRoot, "artifacts", filepath.FromSlash(clean))
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return bundleVerdict{}
		}
		return bundleVerdict{exists: true, issues: []string{fmt.Sprintf("inspect bundle: %v", err)}}
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return bundleVerdict{exists: true, issues: []string{
			"bundle is a symbolic link; a proposal bundle is a directory of regular files",
		}}
	}
	if !info.IsDir() {
		return bundleVerdict{exists: true, issues: []string{"bundle is not a directory"}}
	}

	verdict := bundleVerdict{exists: true}
	skill, _ := scanSkill(dir, filepath.Base(dir))
	for _, issue := range skill.Issues {
		verdict.issues = append(verdict.issues, issue.Reason)
	}
	verdict.issues = append(verdict.issues, bundleEntryIssues(dir)...)

	// The hash is recomputed only when the manifest can be parsed; a missing or
	// frontmatter-less SKILL.md already has its reason above, and hashing the
	// wrong bytes would report a second, misleading problem.
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err == nil {
		if _, body, ok := splitFrontmatter(string(data)); ok {
			hash, hashErr := procedureHash(dir, body)
			if hashErr != nil {
				verdict.issues = append(verdict.issues, fmt.Sprintf("recompute procedure hash: %v", hashErr))
			} else {
				verdict.hash = hash
			}
		}
	}
	verdict.issues = dedupeStrings(verdict.issues)
	return verdict
}

// bundleEntryIssues reports bundle entries that a procedure cannot consist of.
// The skill scanner follows a symlink to keep the store's rules, but a proposal
// bundle is a portable directory: a non-regular entry makes it non-portable and
// would be copied as a link rather than as content.
func bundleEntryIssues(dir string) []string {
	var issues []string
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			issues = append(issues, fmt.Sprintf("read %s: %v", displayBundlePath(dir, path), err))
			return nil
		}
		if path == dir {
			return nil
		}
		rel := displayBundlePath(dir, path)
		if entry.Type()&fs.ModeSymlink != 0 {
			issues = append(issues, fmt.Sprintf(
				"%q is a symbolic link; a proposal bundle holds regular files only", rel,
			))
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			issues = append(issues, fmt.Sprintf("%q is not a regular file", rel))
		}
		return nil
	})
	if walkErr != nil {
		issues = append(issues, fmt.Sprintf("walk bundle: %v", walkErr))
	}
	return issues
}

func displayBundlePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

// ProcedureHash recomputes a proposal bundle's procedure-content hash. It is
// the hash the Archivist's prompt tells the role to print: the SKILL.md body
// (frontmatter excluded, each line newline-terminated) followed by every
// supporting regular file in byte-sorted path order, each preceded by its
// slash-form path relative to the bundle and a newline.
//
// The same function is the install-time identity: writing provenance into the
// frontmatter cannot change it, while editing the body or a supporting file
// does. A symlink is not part of the hash, because it is not a regular file.
func ProcedureHash(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", ManifestName, err)
	}
	_, body, ok := splitFrontmatter(string(data))
	if !ok {
		return "", fmt.Errorf("%s must begin with a --- frontmatter block closed by ---", ManifestName)
	}
	return procedureHash(dir, body)
}

func procedureHash(dir, body string) (string, error) {
	files, err := supportingFiles(dir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	writeProcedureBody(h, body)
	for _, rel := range files {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			return "", fmt.Errorf("read %s: %w", rel, err)
		}
		_, _ = io.WriteString(h, rel)
		_, _ = io.WriteString(h, "\n")
		_, _ = h.Write(content)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeProcedureBody reproduces the shell pipeline's `awk` step: every body
// line is written with a trailing newline, including a final line that has none
// in the file.
func writeProcedureBody(w io.Writer, body string) {
	for len(body) > 0 {
		end := strings.IndexByte(body, '\n')
		if end < 0 {
			_, _ = io.WriteString(w, body)
			_, _ = io.WriteString(w, "\n")
			return
		}
		_, _ = io.WriteString(w, body[:end])
		_, _ = io.WriteString(w, "\n")
		body = body[end+1:]
	}
}

// supportingFiles lists the bundle's regular files other than its root
// SKILL.md, in the order the documented shell pipeline hashes them: byte order
// of the `./`-prefixed relative path.
func supportingFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir || entry.IsDir() {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ManifestName {
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(files, func(i, j int) bool {
		return "./"+files[i] < "./"+files[j]
	})
	return files, nil
}

// AppendProposalDecision validates and appends one decision line. The runtime
// is its only writer; a refusal, an install, and a trial all go through this
// function so the file has one shape.
func AppendProposalDecision(colonyRoot, traceID string, decision ProposalDecision) error {
	if reason := validateProposalDecision(decision, traceID); reason != "" {
		return fmt.Errorf("skills: cannot record proposal decision: %s", reason)
	}
	path := ProposalDecisionsPath(colonyRoot, traceID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("skills: create decision directory: %w", err)
	}
	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("skills: encode proposal decision: %w", err)
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

// ReadProposalDecisions reads one trail's decision log. The second result names
// every line that could not be used; a damaged line is reported and the rest of
// the file still counts, because the log is append-only and one bad line must
// not hide a Beekeeper's earlier decisions.
func ReadProposalDecisions(path, traceID string) ([]ProposalDecision, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	defer func() { _ = f.Close() }()

	var decisions []ProposalDecision
	var issues []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), maxDecisionLineBytes)
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var raw struct {
			Skill         string   `json:"skill"`
			Trail         string   `json:"trail"`
			ProcedureHash string   `json:"procedureHash"`
			Destination   string   `json:"destination"`
			Decision      string   `json:"decision"`
			Reason        string   `json:"reason"`
			Roles         []string `json:"roles"`
			At            string   `json:"at"`
		}
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			issues = append(issues, fmt.Sprintf("%s line %d: malformed decision line: %v", path, line, err))
			continue
		}
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.At))
		if err != nil {
			issues = append(issues, fmt.Sprintf("%s line %d: %q is not an RFC 3339 time", path, line, raw.At))
			continue
		}
		decision := ProposalDecision{
			Skill:         strings.TrimSpace(raw.Skill),
			Trail:         strings.TrimSpace(raw.Trail),
			ProcedureHash: strings.TrimSpace(raw.ProcedureHash),
			Destination:   strings.TrimSpace(raw.Destination),
			Decision:      strings.TrimSpace(raw.Decision),
			Reason:        strings.TrimSpace(raw.Reason),
			Roles:         raw.Roles,
			At:            at,
		}
		if reason := validateProposalDecision(decision, traceID); reason != "" {
			issues = append(issues, fmt.Sprintf("%s line %d: %s", path, line, reason))
			continue
		}
		decisions = append(decisions, decision)
	}
	if err := scanner.Err(); err != nil {
		return nil, issues, err
	}
	return decisions, issues, nil
}

// validateProposalDecision is the one rule set for a decision line. traceID is
// the trail whose file holds the line: a decision that names another trail is
// misplaced, not a decision about this one.
func validateProposalDecision(decision ProposalDecision, traceID string) string {
	ref, err := ParseReference(decision.Skill)
	if err != nil {
		return fmt.Sprintf("skill: %v", err)
	}
	if ref.IsPattern() {
		return fmt.Sprintf("skill %q is a pattern; a decision names one procedure", decision.Skill)
	}
	if decision.Trail == "" {
		return "trail is required"
	}
	if traceID != "" && decision.Trail != traceID {
		return fmt.Sprintf("trail %q does not match the trail that holds the decision (%s)", decision.Trail, traceID)
	}
	if decision.ProcedureHash == "" {
		return "procedureHash is required"
	}
	if decision.Destination == "" {
		return "destination is required"
	}
	if decision.Destination != ProposalStoreDestination && !validTrailToken(decision.Destination) {
		return fmt.Sprintf("destination %q is neither %q nor a trail id", decision.Destination, ProposalStoreDestination)
	}
	switch decision.Decision {
	case ProposalInstalled:
		if decision.Destination != ProposalStoreDestination {
			return fmt.Sprintf("an install's destination is %q, not %q", ProposalStoreDestination, decision.Destination)
		}
	case ProposalTrialled:
		if decision.Destination == ProposalStoreDestination {
			return "a trial needs a destination trail, not the store"
		}
	case ProposalRefused:
	default:
		return fmt.Sprintf("decision %q is not one of %s, %s, or %s",
			decision.Decision, ProposalInstalled, ProposalRefused, ProposalTrialled)
	}
	if decision.At.IsZero() {
		return "at is required"
	}
	return ""
}

// validTrailToken reports whether a string can be a trail id in a reference or
// a decision. Trail ids are `trace-` plus a lowercase hex identity, but older
// and imported records use letters and underscores too; the check is the
// narrowest one that keeps a path or a pattern out.
func validTrailToken(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			continue
		default:
			return false
		}
	}
	return true
}
