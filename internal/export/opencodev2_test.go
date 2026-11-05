package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/hiveview"
)

const openCodeV2ExportFixture = `{
  "info": {"id": "ses_v2_export", "location": {"directory": "/tmp/colony"}},
  "messages": [
    {"type": "user", "text": "please read the file", "time": {"created": 1000}},
    {"type": "assistant", "time": {"created": 2000}, "content": [
      {"type": "text", "text": "I will read it."},
      {"type": "tool", "id": "call-1", "name": "Read", "state": {"status": "completed", "input": {"path": "main.go"}, "content": [{"type": "text", "text": "package main"}]}}
    ]},
    {"type": "assistant", "time": {"created": 3000}, "content": [
      {"type": "reasoning", "text": "private thinking must stay out"},
      {"type": "text", "text": "It is small."}
    ]}
  ]
}`

// installOpenCodeV2Export shadows the provider binary with a fixture producer
// so export exercises the same route a finished 2.x run would use.
func installOpenCodeV2Export(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	fixture := filepath.Join(dir, "export.json")
	if err := os.WriteFile(fixture, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncat '" + fixture + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "opencode"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestResolveOpenCodeV2TranscriptUsesSharedProjections(t *testing.T) {
	installOpenCodeV2Export(t, openCodeV2ExportFixture)

	run := hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "opencode",
		ProviderSessionID: "ses_v2_export",
		Workspace:         "/tmp/colony",
	}

	log := resolveAgentLog(run, defaultTranscriptLookup, 0)
	if log.Omitted != "" {
		t.Fatalf("Agent log Omitted = %q", log.Omitted)
	}
	if len(log.ToolCalls) != 1 || log.ToolCalls[0].Name != "Read" || log.ToolCalls[0].Args != "main.go" || log.ToolCalls[0].CallID != "call-1" {
		t.Fatalf("Agent log calls = %+v", log.ToolCalls)
	}

	conversation := resolveConversation(run, defaultTranscriptLookup, 0, false)
	if conversation.Omitted != "" {
		t.Fatalf("conversation Omitted = %q", conversation.Omitted)
	}
	if len(conversation.Turns) != 3 {
		t.Fatalf("conversation turns = %+v", conversation.Turns)
	}
	if conversation.Turns[0].Blocks[0].Text != "please read the file" || conversation.Turns[1].Blocks[0].Text != "I will read it." {
		t.Fatalf("conversation text = %+v", conversation.Turns)
	}
	tools := 0
	for _, turn := range conversation.Turns {
		for _, block := range turn.Blocks {
			if block.Kind != adapters.DialogueBlockTool || block.Tool == nil {
				continue
			}
			tools++
			if block.Tool.Name != "Read" || block.Tool.Status != adapters.ToolStatusOK {
				t.Fatalf("tool block = %+v", block.Tool)
			}
		}
		if strings.Contains(blockText(turn.Blocks), "private thinking must stay out") {
			t.Fatalf("dialogue leaked reasoning: %+v", turn.Blocks)
		}
	}
	if tools != 1 {
		t.Fatalf("dialogue tool blocks = %d, want 1", tools)
	}
}

func TestExportTraceIncludesTranscriptAndAgentLogsForOpenCodeV2(t *testing.T) {
	ctx, repo := setupExportColony(t)
	traceID := "trace-opencode-v2-includes"
	writeCompletedRunCfg(t, repo, traceID, time.Now().UTC().Add(-time.Minute), completedRunCfg{
		Adapter:           "opencode",
		ProviderSessionID: "ses_v2_export",
	})
	installOpenCodeV2Export(t, openCodeV2ExportFixture)

	md := exportWithIncludes(t, ctx, traceID, FormatMarkdown, nil, "transcript", "agent-logs")

	for _, want := range []string{
		"#### Conversation",
		"#### Agent log",
		"please read the file",
		"I will read it.",
		"It is small.",
		"| Read | main.go | call-1 |",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("export missing %q:\n%s", want, md)
		}
	}
	for _, unwanted := range []string{
		"private thinking must stay out",
		"package main",
	} {
		if strings.Contains(md, unwanted) {
			t.Fatalf("export leaked %q:\n%s", unwanted, md)
		}
	}
}

func blockText(blocks []adapters.DialogueBlock) string {
	var out strings.Builder
	for _, block := range blocks {
		out.WriteString(block.Text)
		out.WriteString(block.Reasoning)
	}
	return out.String()
}
