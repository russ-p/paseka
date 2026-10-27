package opencodev1

import (
	"github.com/russ-p/paseka/internal/adapters"
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

// runFormat returns OpenCode's runtime-owned output format: json by default,
// default (human text) only for a --raw invocation.
func runFormat(raw bool) string {
	if raw {
		return "default"
	}
	return "json"
}

func buildArgs(req adapters.RunRequest, prompt string) []string {
	p := req.Params
	args := []string{"run", "--format", runFormat(req.Raw), "--dir", req.Workspace}
	// plan wins over yolo: --auto would conflict with the read-only plan agent.
	if (p.Trust || p.Force) && !p.Plan {
		args = append(args, "--auto")
	}
	if req.AgentID != "" {
		args = append(args, "--title", req.AgentID)
	}
	if p.Plan {
		args = append(args, "--agent", "plan")
	}
	// The launch resolver already folded any provider into params.model, so
	// OpenCode receives the vendor-qualified id natively.
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Reasoning != "" {
		args = append(args, "--variant", p.Reasoning)
	}
	if prompt != "" {
		args = append(args, "--", prompt)
	}
	return args
}

func buildInteractiveArgs(req adapters.SessionRequest, prompt, resumeID string) []string {
	p := req.Params
	var args []string
	if p.Plan {
		args = append(args, "--agent", "plan")
	}
	if p.Model != "" {
		args = append(args, "--model", p.Model)
	}
	if p.Reasoning != "" {
		args = append(args, "--variant", p.Reasoning)
	}
	if resumeID != "" {
		args = append(args, "--session", resumeID)
	}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	return args
}
