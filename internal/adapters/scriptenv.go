package adapters

import (
	"fmt"

	"github.com/russ-p/paseka/internal/runs"
)

// ScriptEnv returns process environment for script adapter runs, layering the
// PASEKA_* contract onto base. The launch resolver passes the inherited
// environment as base; a direct adapter call may pass nil for a self-contained
// PASEKA_*-only environment.
func ScriptEnv(base []string, req RunRequest, runDir runs.Dir) []string {
	env := append([]string(nil), base...)
	set := func(key, val string) {
		if val == "" {
			return
		}
		env = append(env, key+"="+val)
	}
	set("PASEKA_TRACE_ID", req.TraceID)
	set("PASEKA_AGENT_ID", req.AgentID)
	set("PASEKA_TASK_ID", req.TaskID)
	set("PASEKA_ATTEMPT_ID", req.AttemptID)
	set("PASEKA_CONFIG_REF", req.ConfigRef)
	if req.ConfigRevision > 0 {
		set("PASEKA_CONFIG_REVISION", fmt.Sprintf("%d", req.ConfigRevision))
	}
	set("PASEKA_WORKSPACE", req.Workspace)
	set("PASEKA_COLONY_ROOT", req.ColonyRoot)
	set("PASEKA_RUN_DIR", runDir.Root())
	set("PASEKA_BEE", req.Bee)
	set("PASEKA_EVENT_LOG", runDir.EventsPath())
	set("PASEKA_RESULT_FILE", runDir.ResultPath())
	set("PASEKA_PROMPT_FILE", runDir.PromptPath())
	set("PASEKA_SYSTEM_FILE", runDir.SystemPath())
	return env
}
