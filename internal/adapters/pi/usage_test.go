package pi

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
)

const piUsageTurn = `{"input":10,"output":5,"cacheRead":2,"cacheWrite":1,"totalTokens":18,"cost":{"input":0.01,"output":0.02,"cacheRead":0,"cacheWrite":0,"total":0.03}}`

func TestUsageFromStdoutSumsAssistantTurnsOnly(t *testing.T) {
	stdout := `{"type":"session","id":"agent-1"}
{"type":"message_start","message":{"role":"assistant","content":[]}}
{"type":"message_update","usage":{"input":1,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":1,"cost":{"total":0}},"assistantMessageEvent":{"type":"text_delta","delta":"hi"}}
{"type":"message_update","usage":{"input":1,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":1,"cost":{"total":0}},"assistantMessageEvent":{"type":"text_delta","delta":" there"}}
{"type":"message_end","message":{"role":"assistant","usage":` + piUsageTurn + `}}
{"type":"message_start","message":{"role":"toolResult","toolCallId":"t1","content":"ok"}}
{"type":"message_end","message":{"role":"toolResult","toolCallId":"t1","usage":{"input":999,"output":999,"cacheRead":999,"cacheWrite":999,"totalTokens":3996,"cost":{"total":9}}}}
{"type":"message_end","message":{"role":"assistant","usage":{"input":20,"output":7,"cacheRead":0,"cacheWrite":0,"totalTokens":27,"cost":{"total":0.05}}}}`

	got := usageFromStdout(stdout)
	if got == nil {
		t.Fatal("expected usage from json event stream")
	}
	if got.InputTokens != 30 || got.OutputTokens != 12 {
		t.Fatalf("input/output = %d/%d, want 30/12", got.InputTokens, got.OutputTokens)
	}
	if got.CacheReadTokens != 2 || got.CacheWriteTokens != 1 {
		t.Fatalf("cache = %d/%d, want 2/1", got.CacheReadTokens, got.CacheWriteTokens)
	}
	if got.Source != protocol.UsageSourcePiPrintJSON {
		t.Fatalf("source = %q, want %q", got.Source, protocol.UsageSourcePiPrintJSON)
	}
}

func TestUsageFromStdoutIgnoresNonEventOutput(t *testing.T) {
	tests := map[string]string{
		"text mode":   "plain assistant prose\n",
		"empty":       "",
		"rpc frame":   `{"jsonrpc":"2.0","id":1,"result":{"type":"message_end"}}`,
		"no usage":    `{"type":"message_end","message":{"role":"assistant","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0,"totalTokens":0,"cost":{"total":0}}}}`,
		"broken json": `{"type":"message_end","message":`,
	}
	for name, stdout := range tests {
		t.Run(name, func(t *testing.T) {
			if got := usageFromStdout(stdout); got != nil {
				t.Fatalf("expected no usage, got %+v", got)
			}
		})
	}
}

func TestUsageFromSessionJSONLCountsStandaloneSpend(t *testing.T) {
	data := `{"type":"session","id":"agent-1","cwd":"/repo"}
{"type":"message","message":{"role":"user","content":"go"}}
{"type":"message","message":{"role":"assistant","usage":` + piUsageTurn + `}}
{"type":"usage","kind":"cache_warm","provider":"anthropic","model":"claude","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":40,"totalTokens":40,"cost":{"total":0.02}}}
{"type":"compaction","summary":"trimmed","tokensBefore":90000,"usage":{"input":12000,"output":900,"cacheRead":0,"cacheWrite":0,"totalTokens":12900,"cost":{"total":1.5}}}
{"type":"message","message":{"role":"toolResult","toolCallId":"t1","usage":{"input":999,"output":999,"cacheRead":999,"cacheWrite":999,"totalTokens":3996,"cost":{"total":9}}}}`

	got := usageFromSessionJSONL(data)
	if got == nil {
		t.Fatal("expected usage from session jsonl")
	}
	if got.InputTokens != 12010 || got.OutputTokens != 905 {
		t.Fatalf("input/output = %d/%d, want 12010/905", got.InputTokens, got.OutputTokens)
	}
	if got.CacheReadTokens != 2 || got.CacheWriteTokens != 41 {
		t.Fatalf("cache = %d/%d, want 2/41", got.CacheReadTokens, got.CacheWriteTokens)
	}
	if got.Source != protocol.UsageSourcePiSessionJSONL {
		t.Fatalf("source = %q, want %q", got.Source, protocol.UsageSourcePiSessionJSONL)
	}
}

func TestUsageFromSessionDirReadsOnlySessionFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "20260101_120000_agent-pi-1.jsonl"),
		`{"type":"message","message":{"role":"assistant","usage":`+piUsageTurn+`}}`+"\n")
	writeFile(t, filepath.Join(dir, "notes.txt"), "ignored\n")
	if err := os.Mkdir(filepath.Join(dir, "nested.jsonl"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := usageFromSessionDir(dir)
	if got == nil || got.InputTokens != 10 || got.OutputTokens != 5 {
		t.Fatalf("usage = %+v", got)
	}
	if usageFromSessionDir(filepath.Join(dir, "missing")) != nil {
		t.Fatal("expected nil usage for missing session dir")
	}
	if usageFromSessionDir("  ") != nil {
		t.Fatal("expected nil usage for empty session dir")
	}
}

func TestAdapterRunReportsUsageFromStdout(t *testing.T) {
	repo := initPiRepo(t)
	fakePi := writeFakePi(t, `{"type":"message_end","message":{"role":"assistant","usage":`+piUsageTurn+`}}`+"\n"+
		`{"type":"message_end","message":{"role":"assistant","usage":{"input":20,"output":7,"cacheRead":0,"cacheWrite":0,"totalTokens":27,"cost":{"total":0.05}}}}`)

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "worker",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
		Params:     adapters.RunParams{Binary: fakePi, OutputFormat: "json"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage == nil || result.Usage.InputTokens != 30 || result.Usage.OutputTokens != 12 {
		t.Fatalf("run usage = %+v", result.Usage)
	}

	runDir := runs.Dir{ColonyRoot: repo, TraceID: "trace-pi-1", AgentID: "agent-pi-1"}
	protoResult, err := runDir.ReadResultJSON()
	if err != nil {
		t.Fatal(err)
	}
	if protoResult.Usage == nil || protoResult.Usage.InputTokens != 30 {
		t.Fatalf("result.json usage = %+v", protoResult.Usage)
	}
	if protoResult.Usage.Source != protocol.UsageSourcePiPrintJSON {
		t.Fatalf("result.json usage source = %q", protoResult.Usage.Source)
	}
}

func TestAdapterRunFallsBackToSessionFileInTextMode(t *testing.T) {
	repo := initPiRepo(t)
	fakePi := writeFakePiWritingSession(t, "done", `{"type":"message","message":{"role":"assistant","usage":`+piUsageTurn+`}}`+"\n")

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "worker",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
		Params:     adapters.RunParams{Binary: fakePi, OutputFormat: "text"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage == nil || result.Usage.InputTokens != 10 || result.Usage.CacheWriteTokens != 1 {
		t.Fatalf("run usage = %+v", result.Usage)
	}
	if result.Usage.Source != protocol.UsageSourcePiSessionJSONL {
		t.Fatalf("usage source = %q, want %q", result.Usage.Source, protocol.UsageSourcePiSessionJSONL)
	}
}

func TestAdapterRunSkipsSessionFallbackForCustomCommand(t *testing.T) {
	repo := initPiRepo(t)
	fakePi := writeFakePiWritingSession(t, "done", `{"type":"message","message":{"role":"assistant","usage":`+piUsageTurn+`}}`+"\n")

	result, err := New().Run(context.Background(), adapters.RunRequest{
		Bee:        "worker",
		Prompt:     "do the task",
		ColonyRoot: repo,
		Workspace:  repo,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
		Command:    []string{fakePi},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Usage != nil {
		t.Fatalf("custom command must not claim token usage, got %+v", result.Usage)
	}
}

func TestSessionUsageReadsRunScopedSessionDir(t *testing.T) {
	colonyRoot := t.TempDir()
	runDir := runs.Dir{ColonyRoot: colonyRoot, TraceID: "trace-pi-1", AgentID: "agent-pi-1"}
	sessionFile := filepath.Join(runDir.Root(), piSessionsDir, "20260101_120000_agent-pi-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, sessionFile, `{"type":"message","message":{"role":"assistant","usage":`+piUsageTurn+`}}`+"\n"+
		`{"type":"usage","kind":"cache_warm","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":15,"totalTokens":15,"cost":{"total":0.01}}}`+"\n")

	got := NewSession().SessionUsage(context.Background(), adapters.SessionHandle{
		ColonyRoot: colonyRoot,
		TraceID:    "trace-pi-1",
		AgentID:    "agent-pi-1",
	})
	if got == nil || got.InputTokens != 10 || got.CacheWriteTokens != 16 {
		t.Fatalf("session usage = %+v", got)
	}
	if got.Source != protocol.UsageSourcePiSessionJSONL {
		t.Fatalf("usage source = %q", got.Source)
	}

	if got := NewSession().SessionUsage(context.Background(), adapters.SessionHandle{}); got != nil {
		t.Fatalf("expected nil usage for incomplete handle, got %+v", got)
	}
}

func writeFakePiWritingSession(t *testing.T, stdout, sessionJSONL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-pi")
	script := "#!/bin/sh\n" +
		"session_dir=''\n" +
		"session_id=''\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  case \"$1\" in\n" +
		"    --session-dir) session_dir=\"$2\"; shift 2 ;;\n" +
		"    --session-id) session_id=\"$2\"; shift 2 ;;\n" +
		"    *) shift ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ -n \"$session_dir\" ]; then\n" +
		"  mkdir -p \"$session_dir\"\n" +
		"  printf '%s' " + shellQuote(sessionJSONL) + " >\"$session_dir/20260101_120000_${session_id:-unknown}.jsonl\"\n" +
		"fi\n" +
		"printf '%s\\n' " + shellQuote(stdout) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
