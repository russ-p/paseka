package adapters_test

import (
	"encoding/json"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/runs"
)

func testRunDir() runs.Dir {
	return runs.Dir{ColonyRoot: "/colony", TraceID: "trace-1", AgentID: "agent-1"}
}

func TestScriptEnvCarriesSkillManifest(t *testing.T) {
	refs := []adapters.SkillRef{
		{
			Name:        "paseka-release",
			Description: "Run the colony release procedure.",
			Path:        "/colony/.paseka/skills/paseka release/SKILL.md",
		},
		{
			Name:        "docs-style",
			Description: "Match the colony's documentation voice.",
			Path:        "/colony/.paseka/skills/docs-style/SKILL.md",
		},
	}
	env := adapters.ScriptEnv([]string{"PATH=/bin"}, adapters.RunRequest{
		Bee:       "oracle-guard",
		TraceID:   "trace-1",
		AgentID:   "agent-1",
		SkillRefs: refs,
	}, testRunDir())

	value, ok := adapters.EnvMap(env)[adapters.SkillManifestEnv]
	if !ok {
		t.Fatalf("%s missing from env: %v", adapters.SkillManifestEnv, env)
	}

	// The value is data, not argv: it parses without a shell, so the space in
	// the first path survives intact.
	var got []adapters.SkillRef
	if err := json.Unmarshal([]byte(value), &got); err != nil {
		t.Fatalf("manifest is not JSON: %v\n%s", err, value)
	}
	if len(got) != len(refs) {
		t.Fatalf("manifest entries = %d, want %d: %+v", len(got), len(refs), got)
	}
	for i := range refs {
		if got[i] != refs[i] {
			t.Fatalf("entry[%d] = %+v, want %+v", i, got[i], refs[i])
		}
	}
	if got[0].Path != "/colony/.paseka/skills/paseka release/SKILL.md" {
		t.Fatalf("path with a space was altered: %q", got[0].Path)
	}
}

func TestScriptEnvOmitsEmptySkillManifest(t *testing.T) {
	env := adapters.ScriptEnv(nil, adapters.RunRequest{
		Bee:     "oracle-guard",
		TraceID: "trace-1",
		AgentID: "agent-1",
	}, testRunDir())

	if _, ok := adapters.EnvMap(env)[adapters.SkillManifestEnv]; ok {
		t.Fatalf("%s must be absent for an empty set: %v", adapters.SkillManifestEnv, env)
	}
}

func TestScriptEnvKeepsBaseAndExistingContract(t *testing.T) {
	env := adapters.ScriptEnv([]string{"PATH=/bin", "CUSTOM=1"}, adapters.RunRequest{
		Bee:       "oracle-guard",
		TraceID:   "trace-1",
		AgentID:   "agent-1",
		SkillRefs: []adapters.SkillRef{{Name: "one", Description: "One.", Path: "/s/one/SKILL.md"}},
	}, testRunDir())

	got := adapters.EnvMap(env)
	for key, want := range map[string]string{
		"PATH":           "/bin",
		"CUSTOM":         "1",
		"PASEKA_BEE":     "oracle-guard",
		"PASEKA_RUN_DIR": "/colony/.paseka/runs/trace-1/agent-1",
	} {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q", key, got[key], want)
		}
	}
	if _, ok := got[adapters.SkillManifestEnv]; !ok {
		t.Fatalf("%s missing: %v", adapters.SkillManifestEnv, env)
	}
}
