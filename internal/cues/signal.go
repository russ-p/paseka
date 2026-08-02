package cues

import (
	"encoding/json"
	"fmt"
	"strings"
)

// BuildSignalPayload renders a SIGNAL payload for a signal cue.
func BuildSignalPayload(cue Cue, ctx RenderContext) (json.RawMessage, error) {
	if cue.Emit != EmitSignal {
		return nil, fmt.Errorf("cue %q: expected emit signal, got %q", cue.ID, cue.Emit)
	}
	if err := cue.ValidateRunText(ctx); err != nil {
		return nil, err
	}

	title, err := RenderTemplate("title", cue.TitleTemplate, ctx)
	if err != nil {
		return nil, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	body, err := RenderTemplate("body", cue.BodyTemplate, ctx)
	if err != nil {
		return nil, fmt.Errorf("cue %q: %w", cue.ID, err)
	}
	if err := cue.ValidateRenderedFields(title, body); err != nil {
		return nil, err
	}

	payload := map[string]any{
		"kind": cue.SignalKind,
	}
	for k, v := range cue.Static {
		payload[k] = v
	}
	if cue.TitleTemplate != "" {
		payload["title"] = title
	}
	if cue.BodyTemplate != "" {
		payload["body"] = body
	}
	payload["source"] = ctx.Source

	if cue.PayloadTemplate != "" {
		rendered, err := RenderTemplate("payload", cue.PayloadTemplate, ctx)
		if err != nil {
			return nil, fmt.Errorf("cue %q: %w", cue.ID, err)
		}
		rendered = strings.TrimSpace(rendered)
		if rendered == "" {
			return nil, fmt.Errorf("cue %q: payload_template rendered empty", cue.ID)
		}
		var extra map[string]any
		if err := json.Unmarshal([]byte(rendered), &extra); err != nil {
			return nil, fmt.Errorf("cue %q: payload_template must render valid JSON: %w", cue.ID, err)
		}
		for k, v := range extra {
			if _, reserved := payload[k]; reserved {
				continue
			}
			payload[k] = v
		}
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cue %q: marshal payload: %w", cue.ID, err)
	}
	return raw, nil
}
