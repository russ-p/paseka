package cues

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

const maxTitleLen = 200

// RenderContext is passed to cue field templates.
type RenderContext struct {
	Text    string
	Title   string
	Body    string
	Source  string
	TraceID string
	Vars    map[string]string
}

// NewRenderContext builds template context from operator input.
func NewRenderContext(text, source, traceID string, vars map[string]string) RenderContext {
	text = strings.TrimSpace(text)
	ctx := RenderContext{
		Text:    text,
		Title:   titleFromText(text),
		Body:    text,
		Source:  strings.TrimSpace(source),
		TraceID: strings.TrimSpace(traceID),
		Vars:    copyStringMap(vars),
	}
	for k, v := range ctx.Vars {
		switch strings.TrimSpace(k) {
		case "Text":
			ctx.Text = v
		case "Title":
			ctx.Title = strings.TrimSpace(v)
		case "Body":
			ctx.Body = v
		case "Source":
			ctx.Source = strings.TrimSpace(v)
		case "TraceID":
			ctx.TraceID = strings.TrimSpace(v)
		}
	}
	if ctx.Title == "" && ctx.Text != "" {
		ctx.Title = titleFromText(ctx.Text)
	}
	if ctx.Body == "" && ctx.Text != "" {
		ctx.Body = ctx.Text
	}
	return ctx
}

// RenderTemplate executes a cue template with fail-closed missing keys.
func RenderTemplate(name, body string, ctx RenderContext) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", nil
	}
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}
	data := templateData(ctx)
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}
	return buf.String(), nil
}

func templateData(ctx RenderContext) map[string]string {
	data := map[string]string{
		"Text":    ctx.Text,
		"Title":   ctx.Title,
		"Body":    ctx.Body,
		"Source":  ctx.Source,
		"TraceID": ctx.TraceID,
	}
	for k, v := range ctx.Vars {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		data[k] = v
	}
	return data
}

func titleFromText(text string) string {
	line := text
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		line = text[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > maxTitleLen {
		return line[:maxTitleLen]
	}
	return line
}

// RequiresText reports whether empty operator text should be rejected.
func (c Cue) RequiresText() bool {
	if c.TitleTemplate != "" || c.BodyTemplate != "" {
		return true
	}
	if c.PayloadTemplate != "" {
		return strings.Contains(c.PayloadTemplate, ".Text") ||
			strings.Contains(c.PayloadTemplate, ".Title") ||
			strings.Contains(c.PayloadTemplate, ".Body")
	}
	return false
}

// ValidateRunText returns a usage error when required text is missing.
func (c Cue) ValidateRunText(ctx RenderContext) error {
	if !c.RequiresText() {
		return nil
	}
	if strings.TrimSpace(ctx.Text) != "" {
		return nil
	}
	return fmt.Errorf("cue %q: text is required", c.ID)
}

// ValidateRenderedFields fails when title/body templates render empty.
func (c Cue) ValidateRenderedFields(title, body string) error {
	if c.TitleTemplate != "" && strings.TrimSpace(title) == "" {
		return fmt.Errorf("cue %q: title is required", c.ID)
	}
	if c.BodyTemplate != "" && strings.TrimSpace(body) == "" {
		return fmt.Errorf("cue %q: body is required", c.ID)
	}
	return nil
}
