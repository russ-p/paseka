package prompts

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const promptsSubdir = ".paseka/prompts"

// Context is passed to prompt templates at dispatch time.
type Context struct {
	Bee        string
	TraceID    string
	AgentID    string
	ColonyRoot string
	Workspace  string
	Task       string
	Insights   []string
	ResultFile string
}

// ResolveInput encodes template override precedence (highest wins).
type ResolveInput struct {
	InlinePrompt     string
	BeeLocalTemplate string
	BeeTemplate      string
	DefaultTemplate  string
}

// Resolve picks a template file or inline prompt body.
func Resolve(input ResolveInput) (templateFile string, inline string, err error) {
	if strings.TrimSpace(input.InlinePrompt) != "" {
		return "", input.InlinePrompt, nil
	}
	if t := strings.TrimSpace(input.BeeLocalTemplate); t != "" {
		return t, "", nil
	}
	if t := strings.TrimSpace(input.BeeTemplate); t != "" {
		return t, "", nil
	}
	if t := strings.TrimSpace(input.DefaultTemplate); t != "" {
		return t, "", nil
	}
	return "", "", fmt.Errorf("prompts: no template configured")
}

// Loader reads templates from .paseka/prompts/ under a colony root.
type Loader struct {
	colonyRoot string
	promptsDir string
}

// NewLoader creates a loader for colonyRoot/.paseka/prompts/.
func NewLoader(colonyRoot string) (*Loader, error) {
	if colonyRoot == "" {
		return nil, fmt.Errorf("prompts: colony root is required")
	}
	abs, err := filepath.Abs(colonyRoot)
	if err != nil {
		return nil, fmt.Errorf("prompts: colony root: %w", err)
	}
	return &Loader{
		colonyRoot: abs,
		promptsDir: filepath.Join(abs, promptsSubdir),
	}, nil
}

// PromptsDir returns the absolute prompts directory.
func (l *Loader) PromptsDir() string {
	return l.promptsDir
}

// RenderResolved applies override precedence then renders.
func (l *Loader) RenderResolved(input ResolveInput, ctx Context) (string, error) {
	file, inline, err := Resolve(input)
	if err != nil {
		return "", err
	}
	if inline != "" {
		return l.RenderString(inline, ctx)
	}
	return l.Render(file, ctx)
}

// Render executes a template file relative to .paseka/prompts/.
func (l *Loader) Render(templateFile string, ctx Context) (string, error) {
	path, err := l.resolveTemplatePath(templateFile)
	if err != nil {
		return "", err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("prompts: read %s: %w", templateFile, err)
	}
	name := templateNameFromFile(templateFile)
	return l.renderContent(name, string(content), ctx)
}

// RenderString executes an inline template body (still loads partials).
func (l *Loader) RenderString(body string, ctx Context) (string, error) {
	return l.renderContent("inline", body, ctx)
}

func (l *Loader) renderContent(name, body string, ctx Context) (string, error) {
	root, err := l.parseTemplates(name, body)
	if err != nil {
		return "", err
	}
	tmpl := root.Lookup(name)
	if tmpl == nil {
		return "", fmt.Errorf("prompts: template %q not found after parse", name)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("prompts: execute %q: %w", name, err)
	}
	return buf.String(), nil
}

func (l *Loader) parseTemplates(mainName, mainBody string) (*template.Template, error) {
	root := template.New("root")
	if err := l.loadPartials(root); err != nil {
		return nil, err
	}
	if _, err := root.New(mainName).Parse(mainBody); err != nil {
		return nil, fmt.Errorf("prompts: parse %q: %w", mainName, err)
	}
	return root, nil
}

func (l *Loader) loadPartials(root *template.Template) error {
	partialsDir := filepath.Join(l.promptsDir, "_partials")
	entries, err := os.ReadDir(partialsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("prompts: read partials: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		path := filepath.Join(partialsDir, name)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("prompts: read partial %s: %w", name, err)
		}
		partialName := strings.TrimSuffix(name, ".md")
		if _, err := root.New(partialName).Parse(string(content)); err != nil {
			return fmt.Errorf("prompts: parse partial %q: %w", partialName, err)
		}
	}
	return nil
}

func (l *Loader) resolveTemplatePath(templateFile string) (string, error) {
	if err := validateTemplateRef(templateFile); err != nil {
		return "", err
	}
	path := filepath.Join(l.promptsDir, filepath.FromSlash(templateFile))
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	promptsAbs, err := filepath.Abs(l.promptsDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(promptsAbs, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("prompts: template %q escapes prompts directory", templateFile)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("prompts: template %q: %w", templateFile, err)
	}
	return abs, nil
}

func validateTemplateRef(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return fmt.Errorf("prompts: empty template name")
	}
	if filepath.IsAbs(ref) {
		return fmt.Errorf("prompts: absolute template path not allowed")
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("prompts: template path must not contain ..")
	}
	return nil
}

func templateNameFromFile(templateFile string) string {
	base := filepath.Base(templateFile)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
