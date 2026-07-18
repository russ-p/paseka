package nuc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paseka/paseka/internal/colony"
)

type fileAction string

const (
	actionCreated     fileAction = "created"
	actionSkipped     fileAction = "skipped"
	actionOverwritten fileAction = "overwritten"
)

// Import applies a nuc document to a colony directory.
func Import(doc Document, opts ImportOptions) (ImportResult, error) {
	if err := doc.Validate(); err != nil {
		return ImportResult{}, err
	}
	if opts.ColonyRoot == "" {
		return ImportResult{}, fmt.Errorf("nuc: colony root is required")
	}

	var res ImportResult
	beesDir := colony.BeesDir(opts.ColonyRoot)
	promptsDir := colony.PasekaPath(opts.ColonyRoot, "prompts")

	roles := make([]string, 0, len(doc.Spec.Bees))
	for role := range doc.Spec.Bees {
		roles = append(roles, role)
	}
	sort.Strings(roles)

	for _, role := range roles {
		body := doc.Spec.Bees[role]
		dest := filepath.Join(beesDir, role+".yaml")
		action, err := writeColonyFile(dest, []byte(body), opts.Force, opts.DryRun)
		if err != nil {
			return res, fmt.Errorf("nuc: bee %q: %w", role, err)
		}
		appendAction(&res, action, dest)
	}

	promptPaths := make([]string, 0, len(doc.Spec.Prompts))
	for path := range doc.Spec.Prompts {
		promptPaths = append(promptPaths, path)
	}
	sort.Strings(promptPaths)

	for _, ref := range promptPaths {
		content := doc.Spec.Prompts[ref]
		dest := filepath.Join(promptsDir, filepath.FromSlash(ref))
		action, err := writeColonyFile(dest, []byte(content), opts.Force, opts.DryRun)
		if err != nil {
			return res, fmt.Errorf("nuc: prompt %q: %w", ref, err)
		}
		appendAction(&res, action, dest)
	}

	if !opts.DryRun {
		for _, role := range roles {
			if _, _, err := colony.LoadBee(opts.ColonyRoot, role); err != nil {
				return res, fmt.Errorf("nuc: validate bee %q after import: %w", role, err)
			}
		}
	}

	return res, nil
}

func writeColonyFile(path string, data []byte, force, dryRun bool) (fileAction, error) {
	_, err := os.Stat(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}

	if exists && !force {
		return actionSkipped, nil
	}

	if dryRun {
		if exists {
			return actionOverwritten, nil
		}
		return actionCreated, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	if exists {
		return actionOverwritten, nil
	}
	return actionCreated, nil
}

func appendAction(res *ImportResult, action fileAction, path string) {
	switch action {
	case actionCreated:
		res.Created = append(res.Created, path)
	case actionSkipped:
		res.Skipped = append(res.Skipped, path)
	case actionOverwritten:
		res.Overwritten = append(res.Overwritten, path)
	}
}

// FormatImportSummary returns a human-readable import summary.
func FormatImportSummary(res ImportResult, verbose bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "created: %d, skipped: %d, overwritten: %d\n", len(res.Created), len(res.Skipped), len(res.Overwritten))
	if verbose {
		writePaths(&b, "created", res.Created)
		writePaths(&b, "skipped", res.Skipped)
		writePaths(&b, "overwritten", res.Overwritten)
	}
	return b.String()
}

func writePaths(b *strings.Builder, label string, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(b, "%s:\n", label)
	for _, p := range paths {
		fmt.Fprintf(b, "  %s\n", p)
	}
}
