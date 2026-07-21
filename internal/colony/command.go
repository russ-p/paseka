package colony

import (
	"fmt"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Command is an optional bee-level agent invocation (docker-compose style).
// Accepts a shell-like string or a YAML list of strings.
type Command struct {
	argv []string
}

// CommandVars are substituted into bee command templates at dispatch time.
type CommandVars struct {
	Prompt       string
	SystemPrompt string
	SystemFile   string
	CursorPlugin string // deprecated: no longer materialized; kept for command template substitution
	Workspace    string
	TraceID      string
	AgentID      string
	TaskID       string
	ColonyRoot   string
	Result       string // human-readable run summary text
	ResultFile   string // path to summary.md (human-readable run log)
	Meta         string // path to meta.json
	RunDir       string // .paseka/runs/<traceId>/<agentId>/
}

// IsSet reports whether the bee declares a custom command.
func (c Command) IsSet() bool {
	return len(c.argv) > 0
}

// Argv returns the raw command argv from YAML (before variable substitution).
func (c Command) Argv() []string {
	if len(c.argv) == 0 {
		return nil
	}
	out := make([]string, len(c.argv))
	copy(out, c.argv)
	return out
}

// UnmarshalYAML accepts a string or a sequence of strings.
func (c *Command) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		parts, err := splitCommandLine(strings.TrimSpace(value.Value))
		if err != nil {
			return fmt.Errorf("command: %w", err)
		}
		if len(parts) == 0 {
			return fmt.Errorf("command: must not be empty")
		}
		c.argv = parts
		return nil
	case yaml.SequenceNode:
		parts := make([]string, 0, len(value.Content))
		for _, node := range value.Content {
			if node.Kind != yaml.ScalarNode {
				return fmt.Errorf("command: sequence entries must be strings")
			}
			part := strings.TrimSpace(node.Value)
			if part == "" {
				return fmt.Errorf("command: sequence entries must not be empty")
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			return fmt.Errorf("command: must not be empty")
		}
		c.argv = parts
		return nil
	default:
		return fmt.Errorf("command: must be a string or list of strings")
	}
}

// RenderCommand substitutes variables and returns the full process argv.
func (c Command) RenderCommand(vars CommandVars) ([]string, error) {
	if !c.IsSet() {
		return nil, nil
	}
	out := make([]string, len(c.argv))
	for i, arg := range c.argv {
		out[i] = substituteCommandVars(arg, vars)
	}
	return out, nil
}

// HasParams reports whether the bee YAML defines any params entries.
func (b Bee) HasParams() bool {
	return len(b.Params) > 0
}

func substituteCommandVars(s string, vars CommandVars) string {
	replacements := map[string]string{
		"PROMPT":        vars.Prompt,
		"SYSTEM_PROMPT": vars.SystemPrompt,
		"SYSTEM_FILE":   vars.SystemFile,
		"CURSOR_PLUGIN": vars.CursorPlugin,
		"WORKSPACE":     vars.Workspace,
		"TRACE_ID":      vars.TraceID,
		"AGENT_ID":      vars.AgentID,
		"TASK_ID":       vars.TaskID,
		"COLONY_ROOT":   vars.ColonyRoot,
		"RESULT":        vars.Result,
		"RESULT_FILE":   vars.ResultFile,
		"META":          vars.Meta,
		"RUN_DIR":       vars.RunDir,
	}
	for name, val := range replacements {
		s = strings.ReplaceAll(s, "${"+name+"}", val)
	}
	return replaceDollarVars(s, replacements)
}

func replaceDollarVars(s string, vars map[string]string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		if s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteByte(s[i])
				continue
			}
			name := s[i+2 : i+2+end]
			if val, ok := vars[name]; ok {
				b.WriteString(val)
				i += 2 + end
				continue
			}
			b.WriteByte(s[i])
			continue
		}
		j := i + 1
		for j < len(s) && isCommandVarChar(s[j]) {
			j++
		}
		name := s[i+1 : j]
		if val, ok := vars[name]; ok {
			b.WriteString(val)
			i = j - 1
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isCommandVarChar(c byte) bool {
	return c == '_' || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}

// splitCommandLine splits a shell-like command string into argv without invoking a shell.
func splitCommandLine(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var args []string
	var cur strings.Builder
	var quote rune

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		args = append(args, cur.String())
		cur.Reset()
	}

	for i := 0; i < len(s); i++ {
		ch := rune(s[i])
		switch {
		case quote != 0:
			if ch == quote {
				quote = 0
				continue
			}
			cur.WriteRune(ch)
		case ch == '\'' || ch == '"':
			quote = ch
		case unicode.IsSpace(ch):
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unclosed quote")
	}
	flush()
	return args, nil
}
