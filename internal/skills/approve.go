package skills

import (
	"crypto/sha256"
	"encoding/hex"
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
	"gopkg.in/yaml.v3"
)

// Provenance metadata keys. They live in the skill's own `metadata`, which the
// Agent Skills format defines as a string-to-string map, so an approved
// procedure stays portable: the record travels with the file.
const (
	// ProvenanceProposal marks a skill installed from a trail's proposal.
	ProvenanceProposal = "proposal"

	// MetadataSource is the provenance source: `proposal`, `nuc`, or absent for
	// a skill that was written by hand.
	MetadataSource = "source"
	// MetadataTrail is the trail that proposed the skill.
	MetadataTrail = "trail"
	// MetadataRole is the role that proposed it.
	MetadataRole = "role"
	// MetadataApprovedAt is when the Beekeeper approved it, in RFC 3339.
	MetadataApprovedAt = "approved_at"
	// MetadataProcedureHash is the install-time procedure-content hash. It is
	// recomputed by `doctor` to detect a hand edit to the body or a supporting
	// file; a skill without the key has no recorded hash and is not checked.
	MetadataProcedureHash = "procedure_hash"
)

// ProposalApproval is one installed proposal: the decision that was recorded,
// the exact line appended (so a dry run can preview it), the destination, and
// the repository-relative paths the install wrote.
type ProposalApproval struct {
	Proposal      Proposal
	Decision      ProposalDecision
	Line          []byte
	Target        string
	Files         []string
	Provenance    map[string]string
	Replaced      bool
	Disabled      bool
	DisableReason string
}

// installFile is one file of the planned store entry, relative to the skill
// directory and with the bytes it will be written as.
type installFile struct {
	Rel  string
	Data []byte
}

// installPlan is a fully validated install that has written nothing yet. Every
// bundle is read and hashed before the first byte lands, which is what makes a
// batch all-or-nothing for the failures an operator can cause: an invalid,
// missing, refused, or conflicting reference stops the whole batch in preflight.
type installPlan struct {
	proposal      Proposal
	files         []installFile
	provenance    map[string]string
	decision      ProposalDecision
	line          []byte
	target        string
	repoFiles     []string
	replaced      bool
	disabled      bool
	disableReason string
}

// ApproveProposals installs one or more proposed skills into the repository
// store. A reference is a skill name or `<trail>/<skill>`; a bare name uses
// defaultTrail and an ambiguous bare name is refused by naming every candidate.
//
// The whole batch is planned before anything is written: every bundle is
// resolved, its procedure hash is recomputed, its `SKILL.md` is validated
// against the destination name, and a conflicting or already-refused reference
// fails the batch. An existing store entry is refused without force; a forced
// replacement keeps any disable record, so the skill remains disabled.
//
// An install writes provenance into the skill's own metadata, appends one
// `installed` decision to each source trail's decision log, and never writes a
// commit. dryRun returns the same plan and leaves the store, ledger, and
// decision files byte-identical.
func ApproveProposals(colonyRoot string, raws []string, defaultTrail string, force bool, at time.Time, dryRun bool) ([]ProposalApproval, error) {
	if len(raws) == 0 {
		return nil, fmt.Errorf("skills: approve needs at least one proposal reference")
	}
	if at.IsZero() {
		at = time.Now()
	}
	at = at.UTC()

	inventory, err := ScanProposals(colonyRoot)
	if err != nil {
		return nil, err
	}
	selected, err := selectApprovalProposals(inventory, raws, defaultTrail)
	if err != nil {
		return nil, err
	}

	store, err := ScanColony(colonyRoot)
	if err != nil {
		return nil, err
	}
	ledger, err := LoadLedger(colonyRoot)
	if err != nil {
		return nil, err
	}

	plans := make([]installPlan, 0, len(selected))
	for _, proposal := range selected {
		plan, err := prepareInstallPlan(colonyRoot, store, ledger, proposal, force, at)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}

	if dryRun {
		return approvalResults(plans), nil
	}

	trails := approvalTrails(plans)
	var releases []func()
	for _, trail := range trails {
		release, err := lockProposalDecisions(colonyRoot, trail)
		if err != nil {
			for i := len(releases) - 1; i >= 0; i-- {
				releases[i]()
			}
			return nil, err
		}
		releases = append(releases, release)
	}
	defer func() {
		for i := len(releases) - 1; i >= 0; i-- {
			releases[i]()
		}
	}()

	// Re-read under the locks: a refusal or an install that landed after the
	// inventory scan must decide the batch, not be overwritten by a stale plan.
	if err := recheckApprovalDecisions(colonyRoot, plans, force); err != nil {
		return nil, err
	}
	if err := commitInstallPlans(colonyRoot, plans); err != nil {
		return nil, err
	}
	for _, plan := range plans {
		if err := appendProposalDecisionLocked(colonyRoot, plan.proposal.Trail, plan.decision, plan.line); err != nil {
			return nil, err
		}
	}
	return approvalResults(plans), nil
}

// selectApprovalProposals resolves the command's references and refuses two
// proposals that would install the same store name. The store is one directory
// per name, so two trails proposing one name is a conflict the batch cannot
// resolve on its own.
func selectApprovalProposals(inventory ProposalInventory, raws []string, defaultTrail string) ([]Proposal, error) {
	seen := map[string]bool{}
	selected := make([]Proposal, 0, len(raws))
	for _, raw := range raws {
		ref, err := ParseProposalReference(raw, defaultTrail)
		if err != nil {
			return nil, err
		}
		matched, err := inventory.Resolve(ref)
		if err != nil {
			return nil, err
		}
		proposal := matched[0]
		key := proposal.Trail + "/" + proposal.Name
		if seen[key] {
			continue
		}
		seen[key] = true
		selected = append(selected, proposal)
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Trail != selected[j].Trail {
			return selected[i].Trail < selected[j].Trail
		}
		return selected[i].Name < selected[j].Name
	})

	byName := map[string]string{}
	for _, proposal := range selected {
		if prior, ok := byName[proposal.Name]; ok {
			return nil, fmt.Errorf(
				"skills: the batch installs two proposals named %q (from %s and %s); one store entry has one name",
				proposal.Name, prior, proposal.Trail,
			)
		}
		byName[proposal.Name] = proposal.Trail
	}
	return selected, nil
}

// prepareInstallPlan validates one proposal against the live store and reads
// its bundle into the bytes that will be written. It never writes.
func prepareInstallPlan(colonyRoot string, store Report, ledger Ledger, proposal Proposal, force bool, at time.Time) (installPlan, error) {
	switch proposal.State {
	case ProposalRefused:
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s was already refused; a refusal is terminal for that exact bundle",
			proposal.Trail, proposal.Name,
		)
	case ProposalInstalled:
		if !force {
			return installPlan{}, fmt.Errorf(
				"skills: proposal %s/%s is already installed into the store; pass --force to replace it",
				proposal.Trail, proposal.Name,
			)
		}
	}
	if proposal.BundleMissing || proposal.State == ProposalMissing {
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s has no bundle at %q; there is nothing to install",
			proposal.Trail, proposal.Name, proposal.Bundle,
		)
	}
	if !proposal.Valid {
		validation := "invalid bundle"
		if len(proposal.Validation) > 0 {
			validation = strings.Join(proposal.Validation, "; ")
		}
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s cannot be approved: %s",
			proposal.Trail, proposal.Name, validation,
		)
	}

	target := filepath.Join(Dir(colonyRoot), proposal.Name)
	replaced, err := storeEntryExists(target)
	if err != nil {
		return installPlan{}, err
	}
	if replaced && !force {
		return installPlan{}, fmt.Errorf(
			"skills: the store already holds a skill named %q; pass --force to replace it",
			proposal.Name,
		)
	}
	for _, skill := range store.Skills {
		if skill.Name == proposal.Name {
			replaced = true
			break
		}
	}

	plan, hash, err := readValidatedBundle(colonyRoot, proposal)
	if err != nil {
		return installPlan{}, err
	}
	plan.target = target
	plan.replaced = replaced

	manifestIndex := -1
	for i, file := range plan.files {
		if file.Rel == ManifestName {
			manifestIndex = i
			break
		}
	}

	plan.provenance = map[string]string{
		MetadataSource:        ProvenanceProposal,
		MetadataTrail:         proposal.Trail,
		MetadataRole:          proposalRole(colonyRoot, proposal),
		MetadataApprovedAt:    at.Format(time.RFC3339),
		MetadataProcedureHash: hash,
	}
	updated, err := writeProvenance(plan.files[manifestIndex].Data, plan.provenance)
	if err != nil {
		return installPlan{}, fmt.Errorf("skills: proposal %s/%s: %w", proposal.Trail, proposal.Name, err)
	}
	plan.files[manifestIndex].Data = updated

	plan.decision = ProposalDecision{
		Skill:         proposal.Name,
		Trail:         proposal.Trail,
		ProcedureHash: hash,
		Destination:   ProposalStoreDestination,
		Decision:      ProposalInstalled,
		At:            at,
	}
	plan.line, err = EncodeProposalDecision(plan.decision)
	if err != nil {
		return installPlan{}, err
	}

	if record, disabled := ledger.Record(proposal.Name); disabled {
		plan.disabled = true
		plan.disableReason = record.Reason
	}

	plan.repoFiles = make([]string, 0, len(plan.files))
	for _, file := range plan.files {
		rel, err := filepath.Rel(colonyRoot, filepath.Join(target, filepath.FromSlash(file.Rel)))
		if err != nil {
			return installPlan{}, fmt.Errorf("skills: resolve destination path for %q: %w", file.Rel, err)
		}
		plan.repoFiles = append(plan.repoFiles, filepath.ToSlash(rel))
	}
	return plan, nil
}

// readValidatedBundle reads a proposal bundle into the files a destination
// would receive and returns the procedure hash the planned bytes carry. The
// planned `SKILL.md` is validated against the destination name, not against
// the directory the bundle happens to live in, and the recomputed hash must
// match the discovery: a changed bundle is a different proposal.
func readValidatedBundle(colonyRoot string, proposal Proposal) (installPlan, string, error) {
	plan, err := readBundlePlan(colonyRoot, proposal)
	if err != nil {
		return installPlan{}, "", err
	}

	manifestIndex := -1
	for i, file := range plan.files {
		if file.Rel == ManifestName {
			manifestIndex = i
			break
		}
	}
	if manifestIndex < 0 {
		return installPlan{}, "", fmt.Errorf(
			"skills: proposal %s/%s bundle %q has no %s", proposal.Trail, proposal.Name, proposal.Bundle, ManifestName,
		)
	}
	doc, body, reason := parseDocument(plan.files[manifestIndex].Data)
	if reason != "" {
		return installPlan{}, "", fmt.Errorf("skills: proposal %s/%s cannot be approved: %s", proposal.Trail, proposal.Name, reason)
	}
	if issues := validateDocument(proposal.Name, doc); len(issues) > 0 {
		reasons := make([]string, 0, len(issues))
		for _, issue := range issues {
			reasons = append(reasons, issue.Reason)
		}
		return installPlan{}, "", fmt.Errorf(
			"skills: proposal %s/%s cannot be approved: %s",
			proposal.Trail, proposal.Name, strings.Join(reasons, "; "),
		)
	}
	hash, err := hashInstallContent(body, plan.files)
	if err != nil {
		return installPlan{}, "", fmt.Errorf("skills: proposal %s/%s: %w", proposal.Trail, proposal.Name, err)
	}
	if hash != strings.TrimSpace(proposal.ProcedureHash) {
		return installPlan{}, "", fmt.Errorf(
			"skills: proposal %s/%s cannot be approved: procedure hash mismatch: discovery says %s, the bundle hashes to %s",
			proposal.Trail, proposal.Name, proposal.ProcedureHash, hash,
		)
	}
	return plan, hash, nil
}

// readBundlePlan reads a proposal bundle into the files an install would write.
// It re-applies the portability rules the scan applies, because the bytes are
// about to be copied: a symlink, a non-regular file, or a path that leaves the
// bundle is refused rather than materialized in the store.
func readBundlePlan(colonyRoot string, proposal Proposal) (installPlan, error) {
	clean, ok := protocol.CombRelativeBundle(proposal.Bundle)
	if !ok {
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s bundle %q is not a path inside the trail artifact comb",
			proposal.Trail, proposal.Name, proposal.Bundle,
		)
	}
	trailRoot := runs.Trail{ColonyRoot: colonyRoot, TraceID: proposal.Trail}.Root()
	dir := filepath.Join(trailRoot, "artifacts", filepath.FromSlash(clean))
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return installPlan{}, fmt.Errorf(
				"skills: proposal %s/%s has no bundle at %q; there is nothing to install",
				proposal.Trail, proposal.Name, proposal.Bundle,
			)
		}
		return installPlan{}, fmt.Errorf("skills: inspect proposal bundle %s: %w", dir, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s bundle is a symbolic link; a bundle is a directory of regular files",
			proposal.Trail, proposal.Name,
		)
	}
	if !info.IsDir() {
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s bundle %s is not a directory", proposal.Trail, proposal.Name, dir,
		)
	}

	plan := installPlan{proposal: proposal}
	walkErr := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("read bundle entry %s: %w", path, err)
		}
		if path == dir {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("resolve bundle entry %s: %w", path, err)
		}
		rel = filepath.ToSlash(rel)
		if rel == ".." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			return fmt.Errorf("bundle entry %q escapes the bundle", rel)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("bundle entry %q is a symbolic link; a bundle holds regular files only", rel)
		}
		if entry.IsDir() {
			return nil
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return fmt.Errorf("inspect bundle entry %q: %w", rel, err)
		}
		if !entryInfo.Mode().IsRegular() {
			return fmt.Errorf("bundle entry %q is not a regular file", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read bundle entry %q: %w", rel, err)
		}
		plan.files = append(plan.files, installFile{Rel: rel, Data: data})
		return nil
	})
	if walkErr != nil {
		return installPlan{}, fmt.Errorf("skills: proposal %s/%s: %w", proposal.Trail, proposal.Name, walkErr)
	}
	if len(plan.files) == 0 {
		return installPlan{}, fmt.Errorf(
			"skills: proposal %s/%s bundle %q is empty", proposal.Trail, proposal.Name, proposal.Bundle,
		)
	}
	sort.SliceStable(plan.files, func(i, j int) bool { return plan.files[i].Rel < plan.files[j].Rel })
	return plan, nil
}

// hashInstallContent computes the procedure hash from the planned bytes, using
// the same order `ProcedureHash` uses: the body with every line newline
// terminated, then every supporting file in byte order of its `./`-prefixed
// path. Frontmatter is excluded, so writing provenance cannot change it.
func hashInstallContent(body string, files []installFile) (string, error) {
	supporting := make([]installFile, 0, len(files))
	for _, file := range files {
		if file.Rel == ManifestName {
			continue
		}
		supporting = append(supporting, file)
	}
	sort.SliceStable(supporting, func(i, j int) bool {
		return "./"+supporting[i].Rel < "./"+supporting[j].Rel
	})

	h := sha256.New()
	writeProcedureBody(h, body)
	for _, file := range supporting {
		_, _ = io.WriteString(h, file.Rel)
		_, _ = io.WriteString(h, "\n")
		if _, err := h.Write(file.Data); err != nil {
			return "", fmt.Errorf("hash %s: %w", file.Rel, err)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeProvenance adds or replaces the provenance keys in a skill's metadata
// while preserving every other frontmatter key and the body byte-for-byte.
func writeProvenance(manifest []byte, provenance map[string]string) ([]byte, error) {
	return writeProvenanceEdits(manifest, provenance, nil)
}

// writeProvenanceEdits sets and removes metadata keys while preserving every
// other frontmatter key and the body byte-for-byte. Removal exists for a nuc
// install: a carried proposal's trail, role, and approval time describe the
// colony it came from, not this one.
func writeProvenanceEdits(manifest []byte, provenance map[string]string, remove []string) ([]byte, error) {
	front, body, ok := splitFrontmatter(string(manifest))
	if !ok {
		return nil, fmt.Errorf("%s must begin with a --- frontmatter block closed by ---", ManifestName)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid frontmatter: the document is not a mapping")
	}
	root := doc.Content[0]
	metadata := mappingChild(root, "metadata")
	if metadata == nil {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "metadata"},
			&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
		)
		metadata = root.Content[len(root.Content)-1]
	}
	if metadata.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("invalid frontmatter: metadata must be a mapping")
	}

	keys := make([]string, 0, len(provenance))
	for key := range provenance {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range remove {
		removeMappingKey(metadata, key)
	}
	for _, key := range keys {
		setMappingString(metadata, key, provenance[key])
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.Write(out)
	builder.WriteString("---\n")
	builder.WriteString(body)
	return []byte(builder.String()), nil
}

// mappingChild returns the value node for key, or nil when the mapping has no
// such key. A scalar value is returned as-is; the caller decides whether the
// shape is acceptable.
func mappingChild(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// removeMappingKey drops a mapping entry when it is present.
func removeMappingKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return
		}
	}
}

// setMappingString replaces a mapping entry's value with a string scalar, or
// appends the pair when the key is absent.
func setMappingString(mapping *yaml.Node, key, value string) {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = node
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		node,
	)
}

// proposalRole resolves the proposing bee's role from the discovery's producer.
// A run record wins; when none exists the producer id is the only identity the
// record carries, so it is kept rather than invented.
func proposalRole(colonyRoot string, proposal Proposal) string {
	producer := runs.ProducerForEvent(colonyRoot, proposal.Trail, proposal.AgentID, nil)
	if role := strings.TrimSpace(producer.Bee); role != "" {
		return role
	}
	if role := strings.TrimSpace(proposal.AgentID); role != "" {
		return role
	}
	return "unknown"
}

// storeEntryExists reports whether the store already has an entry with the
// target name. It uses Lstat so a symlink or a stray file is an existing entry
// that --force must replace rather than something an install writes through.
func storeEntryExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("skills: inspect store entry %s: %w", path, err)
}

// approvalTrails returns the sorted unique source trails of a batch. Locks are
// always taken in this order, so two batches that share trails cannot deadlock.
func approvalTrails(plans []installPlan) []string {
	seen := map[string]bool{}
	var trails []string
	for _, plan := range plans {
		if seen[plan.proposal.Trail] {
			continue
		}
		seen[plan.proposal.Trail] = true
		trails = append(trails, plan.proposal.Trail)
	}
	sort.Strings(trails)
	return trails
}

// recheckApprovalDecisions re-reads every source trail's decision log while the
// batch holds its locks. A refusal is terminal; an install is a conflict unless
// the caller forced a replacement.
func recheckApprovalDecisions(colonyRoot string, plans []installPlan, force bool) error {
	byTrail := map[string][]ProposalDecision{}
	for _, plan := range plans {
		trail := plan.proposal.Trail
		if _, ok := byTrail[trail]; ok {
			continue
		}
		decisions, _, err := ReadProposalDecisions(ProposalDecisionsPath(colonyRoot, trail), trail)
		if err != nil {
			return fmt.Errorf("skills: read decisions for trail %s: %w", trail, err)
		}
		byTrail[trail] = decisions
	}
	for _, plan := range plans {
		for _, decision := range applicableDecisions(byTrail[plan.proposal.Trail], plan.proposal) {
			switch decision.Decision {
			case ProposalRefused:
				return fmt.Errorf(
					"skills: proposal %s/%s was already refused; a refusal is terminal for that exact bundle",
					plan.proposal.Trail, plan.proposal.Name,
				)
			case ProposalInstalled:
				if !force {
					return fmt.Errorf(
						"skills: proposal %s/%s is already installed into the store; pass --force to replace it",
						plan.proposal.Trail, plan.proposal.Name,
					)
				}
			}
		}
	}
	return nil
}

// commitInstallPlans stages every planned entry outside the store and then
// renames each into place, so a write that fails before the commit leaves the
// store untouched. A replaced entry is moved aside first and restored if the
// rename fails.
func commitInstallPlans(colonyRoot string, plans []installPlan) error {
	storeDir := Dir(colonyRoot)
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		return fmt.Errorf("skills: create store %s: %w", storeDir, err)
	}
	stagingRoot, err := os.MkdirTemp(filepath.Dir(storeDir), ".skills-approve-*")
	if err != nil {
		return fmt.Errorf("skills: create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingRoot) }()

	staged := make([]string, len(plans))
	for i, plan := range plans {
		dir := filepath.Join(stagingRoot, plan.proposal.Name)
		for _, file := range plan.files {
			path := filepath.Join(dir, filepath.FromSlash(file.Rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("skills: stage %s: %w", file.Rel, err)
			}
			if err := os.WriteFile(path, file.Data, 0o644); err != nil {
				return fmt.Errorf("skills: stage %s: %w", file.Rel, err)
			}
		}
		hash, err := ProcedureHash(dir)
		if err != nil {
			return fmt.Errorf("skills: verify staged %s: %w", plan.proposal.Name, err)
		}
		if hash != plan.proposal.ProcedureHash {
			return fmt.Errorf(
				"skills: staged %s hashes to %s, not %s; refusing to install",
				plan.proposal.Name, hash, plan.proposal.ProcedureHash,
			)
		}
		staged[i] = dir
	}

	for i, plan := range plans {
		backup := ""
		exists, err := storeEntryExists(plan.target)
		if err != nil {
			return err
		}
		if exists {
			backup = filepath.Join(stagingRoot, ".replaced-"+plan.proposal.Name)
			if err := os.Rename(plan.target, backup); err != nil {
				return fmt.Errorf("skills: move aside %s: %w", plan.target, err)
			}
		}
		if err := os.Rename(staged[i], plan.target); err != nil {
			if backup != "" {
				_ = os.Rename(backup, plan.target)
			}
			return fmt.Errorf("skills: install %s: %w", plan.target, err)
		}
		if backup != "" {
			_ = os.RemoveAll(backup)
		}
	}
	return nil
}

// approvalResults maps plans onto the command-facing result.
func approvalResults(plans []installPlan) []ProposalApproval {
	results := make([]ProposalApproval, 0, len(plans))
	for _, plan := range plans {
		provenance := make(map[string]string, len(plan.provenance))
		for key, value := range plan.provenance {
			provenance[key] = value
		}
		target := plan.target
		results = append(results, ProposalApproval{
			Proposal:      plan.proposal,
			Decision:      plan.decision,
			Line:          plan.line,
			Target:        target,
			Files:         append([]string(nil), plan.repoFiles...),
			Provenance:    provenance,
			Replaced:      plan.replaced,
			Disabled:      plan.disabled,
			DisableReason: plan.disableReason,
		})
	}
	return results
}

// ProvenanceIssues recomputes the procedure hash of every skill that records
// one at install time and reports a mismatch. A hand-authored skill has no
// recorded hash, so it is not checked: there is no approved identity to
// disagree with. The finding is what `doctor` prints when a body or a
// supporting file was edited after approval.
func (r Report) ProvenanceIssues() []Issue {
	var issues []Issue
	for _, skill := range r.Skills {
		recorded := strings.TrimSpace(skill.Metadata[MetadataProcedureHash])
		if recorded == "" {
			continue
		}
		recomputed, err := ProcedureHash(skill.Dir)
		if err != nil {
			issues = append(issues, Issue{Skill: skill.Name, Reason: fmt.Sprintf(
				"cannot recompute the installed procedure hash: %v", err,
			)})
			continue
		}
		if recomputed != recorded {
			issues = append(issues, Issue{Skill: skill.Name, Reason: fmt.Sprintf(
				"procedure hash mismatch: the installed hash is %s, but the body and supporting files hash to %s",
				recorded, recomputed,
			)})
		}
	}
	sortIssues(issues)
	return issues
}
