package cursor

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/paseka/paseka/internal/adapters"
)

var sessionUUIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type transcriptLine struct {
	Role    string `json:"role"`
	Message *struct {
		Content []transcriptPart `json:"content"`
	} `json:"message"`
}

type transcriptPart struct {
	Type  string          `json:"type"`
	Name  string          `json:"name"`
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

// ResolveSessionLog reads Cursor agent-transcripts jsonl for providerSessionId.
func (a *Adapter) ResolveSessionLog(_ context.Context, req adapters.SessionLogRequest) (adapters.SessionLog, error) {
	id := strings.TrimSpace(req.ProviderSessionID)
	if !sessionUUIDRe.MatchString(id) {
		return adapters.SessionLog{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	path := findTranscriptPath(req.Workspace, id)
	if path == "" {
		return adapters.SessionLog{Omitted: adapters.SessionLogOmittedStoreNotFound}, nil
	}
	calls, err := parseTranscriptToolCalls(path)
	if err != nil {
		return adapters.SessionLog{Omitted: adapters.SessionLogOmittedParseError}, nil
	}
	return adapters.SessionLog{ToolCalls: calls}, nil
}

func cursorDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".cursor")
}

func projectSlug(workspace string) string {
	s := filepath.ToSlash(strings.TrimSpace(workspace))
	s = strings.ReplaceAll(s, "/", "-")
	return strings.Trim(s, "-")
}

func transcriptRel(id string) string {
	return filepath.Join("agent-transcripts", id, id+".jsonl")
}

func findTranscriptPath(workspace, id string) string {
	root := cursorDir()
	if root == "" {
		return ""
	}
	if slug := projectSlug(workspace); slug != "" && !strings.Contains(slug, "..") {
		preferred := filepath.Join(root, "projects", slug, transcriptRel(id))
		if st, err := os.Stat(preferred); err == nil && !st.IsDir() {
			return preferred
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "projects", "*", transcriptRel(id)))
	if err != nil || len(matches) == 0 {
		return ""
	}
	for _, p := range matches {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func parseTranscriptToolCalls(path string) ([]adapters.ToolCall, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var calls []adapters.ToolCall
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var raw transcriptLine
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			return nil, err
		}
		if raw.Message == nil {
			continue
		}
		for _, part := range raw.Message.Content {
			if part.Type != "tool_use" {
				continue
			}
			name := strings.TrimSpace(part.Name)
			if name == "" {
				continue
			}
			calls = append(calls, adapters.ToolCall{
				Name:   name,
				CallID: strings.TrimSpace(part.ID),
				Args:   summarizeToolInput(part.Input),
			})
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if calls == nil {
		calls = []adapters.ToolCall{}
	}
	return calls, nil
}

func summarizeToolInput(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return string(raw)
	}
	for _, key := range []string{"path", "url", "command"} {
		s, _ := obj[key].(string)
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return string(raw)
	}
	return string(b)
}
