package export

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/russ-p/paseka/internal/trailconfig"
)

// maxInlineConfigBytes bounds one inlined configuration file. A configuration
// snapshot is small by nature, but a captured supporting file can still be
// arbitrarily large; oversized entries are listed by metadata and an explicit
// reason instead of making the report unreadable.
const maxInlineConfigBytes = 256 * 1024

// ConfigFileExport is one retained configuration file in a trace export.
type ConfigFileExport struct {
	ConfigRef string
	Path      string
	Mode      uint32
	Bytes     int64
	// Private marks a machine-local input. Its contents are never exported;
	// Omitted says so instead of silently dropping the entry.
	Private bool
	Content string
	Omitted string
}

// ConfigSnapshotExport is one immutable configuration version retained by the
// trail, with the files the export is allowed to show.
type ConfigSnapshotExport struct {
	ConfigRef string
	Roles     []string
	Source    string
	// FileCount is the number of files the retained version holds.
	FileCount int
	// Missing names a retained version whose bytes are unavailable or corrupt.
	Missing string
	Files   []ConfigFileExport
	Omitted int
}

// ConfigExport is the trail-owned configuration evidence carried by an export.
type ConfigExport struct {
	TraceID          string
	Policy           string
	Revision         int
	InitialConfigRef string
	ActiveConfigRef  string
	Snapshots        []ConfigSnapshotExport
	// Missing explains why no configuration binding could be read at all.
	Missing string
	// Omissions lists every file whose contents the report deliberately left
	// out, so the archive never claims to be self-contained while hiding a gap.
	Omissions []string
}

func loadConfigExport(colonyRoot, traceID string) (*ConfigExport, error) {
	manifest, err := trailconfig.Load(colonyRoot, traceID)
	if err != nil {
		if errors.Is(err, trailconfig.ErrNoBinding) {
			return &ConfigExport{
				TraceID: traceID,
				Missing: "configuration was not captured (legacy trail); configuration evidence is unavailable",
			}, nil
		}
		return nil, fmt.Errorf("export: read trail configuration: %w", err)
	}

	config := &ConfigExport{
		TraceID:          manifest.TraceID,
		Policy:           manifest.Policy,
		Revision:         manifest.ConfigRevision,
		InitialConfigRef: manifest.InitialConfigRef,
		ActiveConfigRef:  manifest.ActiveConfigRef,
	}

	roles := snapshotRoles(manifest)
	refs := make([]string, 0, len(roles))
	for ref := range roles {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		config.Snapshots = append(config.Snapshots, loadConfigSnapshot(colonyRoot, traceID, ref, roles[ref], manifest))
	}
	if len(config.Snapshots) == 0 {
		config.Missing = "configuration manifest retains no snapshots"
	}
	return config, nil
}

func loadConfigSnapshot(colonyRoot, traceID, ref string, roles []string, manifest trailconfig.Manifest) ConfigSnapshotExport {
	entry := ConfigSnapshotExport{ConfigRef: ref, Roles: roles}
	for _, snapshot := range manifest.Snapshots {
		if snapshot.ConfigRef == ref {
			if entry.Source == "" {
				entry.Source = snapshot.Source
			}
			if snapshot.Files > entry.FileCount {
				entry.FileCount = snapshot.Files
			}
		}
	}
	inventory, contents, err := trailconfig.ReadSnapshot(colonyRoot, traceID, ref)
	if err != nil {
		entry.Missing = err.Error()
		entry.FileCount = len(inventory.Entries)
		return entry
	}
	entry.FileCount = len(inventory.Entries)
	for _, file := range contents {
		// Classify by path as well as by the stored flag, so an inventory
		// captured before private inputs were classified still fails closed.
		private := file.Private || trailconfig.PrivatePath(file.Path)
		row := ConfigFileExport{
			ConfigRef: ref,
			Path:      file.Path,
			Mode:      file.Mode,
			Bytes:     file.Size,
			Private:   private,
		}
		switch {
		case private:
			row.Omitted = "private configuration input is not exported"
		case !utf8.Valid(file.Data):
			row.Omitted = "binary or invalid UTF-8"
		case file.Size > maxInlineConfigBytes:
			row.Omitted = fmt.Sprintf("configuration file too large for inline export (%d bytes)", file.Size)
		default:
			row.Content = string(file.Data)
		}
		if row.Omitted != "" {
			entry.Omitted++
		}
		entry.Files = append(entry.Files, row)
	}
	return entry
}

func snapshotRoles(manifest trailconfig.Manifest) map[string][]string {
	roles := make(map[string][]string, len(manifest.Snapshots)+2)
	add := func(ref, role string) {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return
		}
		for _, known := range roles[ref] {
			if known == role {
				return
			}
		}
		roles[ref] = append(roles[ref], role)
	}
	add(manifest.InitialConfigRef, "initial")
	add(manifest.ActiveConfigRef, "active")
	for _, snapshot := range manifest.Snapshots {
		add(snapshot.ConfigRef, snapshot.Source)
	}
	return roles
}

// configOmissions flattens the per-file omissions for the report body.
func configOmissions(config *ConfigExport) []string {
	if config == nil {
		return nil
	}
	var out []string
	if config.Missing != "" {
		out = append(out, config.Missing)
	}
	for _, snapshot := range config.Snapshots {
		if snapshot.Missing != "" {
			out = append(out, fmt.Sprintf("%s: %s", snapshot.ConfigRef, snapshot.Missing))
			continue
		}
		for _, file := range snapshot.Files {
			if file.Omitted != "" {
				out = append(out, fmt.Sprintf("%s: %s", file.Path, file.Omitted))
			}
		}
	}
	return out
}

// configOmittedCount reports how many retained files the export left out.
func configOmittedCount(config *ConfigExport) int {
	if config == nil {
		return 0
	}
	total := 0
	for _, snapshot := range config.Snapshots {
		total += snapshot.Omitted
		if snapshot.Missing != "" {
			total += snapshot.FileCount
		}
	}
	return total
}
