package opencode

import (
	"strings"

	"github.com/paseka/paseka/internal/adapters"
)

func joinPrompt(system, prompt string) string {
	switch {
	case system == "":
		return prompt
	case prompt == "":
		return system
	default:
		return system + "\n" + prompt
	}
}

func resolveModel(p adapters.RunParams) string {
	model := strings.TrimSpace(p.Model)
	if model == "" {
		return ""
	}
	if strings.Contains(model, "/") {
		return model
	}
	if provider := strings.TrimSpace(p.Provider); provider != "" {
		return provider + "/" + model
	}
	return model
}

func runFormat(outputFormat string) string {
	if outputFormat == "text" {
		return "default"
	}
	return "json"
}

func buildArgs(req adapters.RunRequest, prompt string) []string {
	p := req.Params
	args := []string{"run", "--format", runFormat(p.OutputFormat), "--dir", req.Workspace}
	if p.Trust || p.Force {
		args = append(args, "--auto")
	}
	if req.AgentID != "" {
		args = append(args, "--title", req.AgentID)
	}
	if p.Plan {
		args = append(args, "--agent", "plan")
	}
	if model := resolveModel(p); model != "" {
		args = append(args, "--model", model)
	}
	if p.Thinking != "" {
		args = append(args, "--variant", p.Thinking)
	}
	if prompt != "" {
		args = append(args, "--", prompt)
	}
	return args
}

func buildInteractiveArgs(req adapters.SessionRequest, prompt string) []string {
	p := req.Params
	var args []string
	if p.Plan {
		args = append(args, "--agent", "plan")
	}
	if model := resolveModel(p); model != "" {
		args = append(args, "--model", model)
	}
	if p.Thinking != "" {
		args = append(args, "--variant", p.Thinking)
	}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	return args
}
