package opencodev2

import (
	"strings"

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

// buildArgs builds the V2 AFK argv. `--dir` is gone: the launch plan already
// makes the workspace the process working directory. The reasoning variant
// rides on the model reference instead of a separate flag.
func buildArgs(req adapters.RunRequest, prompt string) []string {
	p := req.Params
	args := []string{"run", "--format", runFormat(req.Raw)}
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
	if ref := modelReference(p.Model, p.Reasoning); ref != "" {
		args = append(args, "--model", ref)
	}
	if prompt != "" {
		args = append(args, "--", prompt)
	}
	return args
}

// buildInteractiveArgs builds the V2 TUI argv. The V2 TUI rejects --model,
// --agent, --variant, --port, and --hostname: model, agent, and variant belong
// to the session object created over the API, and the server is named outright.
func buildInteractiveArgs(serverURL, sessionID, prompt string, auto bool) []string {
	var args []string
	if serverURL != "" {
		args = append(args, "--server", serverURL)
	}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	if auto {
		args = append(args, "--auto")
	}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	return args
}

// modelReference folds the reasoning variant into the model id as
// `provider/model#variant`.
func modelReference(model, variant string) string {
	model = strings.TrimSpace(model)
	variant = strings.TrimSpace(variant)
	if model == "" {
		return ""
	}
	if variant == "" {
		return model
	}
	return model + "#" + variant
}

// splitModel turns a resolved `provider/model` (or bare `model`) into its parts.
func splitModel(model string) (provider, id string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", ""
	}
	if i := strings.IndexByte(model, '/'); i >= 0 {
		return model[:i], model[i+1:]
	}
	return "", model
}

// sessionModelFor builds the session object's model reference. The variant is a
// field here rather than part of the id because the API takes the object.
func sessionModelFor(model, variant string) *sessionModel {
	provider, id := splitModel(model)
	variant = strings.TrimSpace(variant)
	if provider == "" && id == "" && variant == "" {
		return nil
	}
	return &sessionModel{ProviderID: provider, ModelID: id, Variant: variant}
}

// firstFlagValue returns the first value found for any of the given flags.
func firstFlagValue(argv []string, flags ...string) string {
	for _, flag := range flags {
		if v := adapters.FlagValue(argv, flag); v != "" {
			return v
		}
	}
	return ""
}
