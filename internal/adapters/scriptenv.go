package adapters

import (
	"os"

	"github.com/paseka/paseka/internal/runs"
)

// ScriptEnv returns process environment for script adapter runs.
func ScriptEnv(req RunRequest, runDir runs.Dir) []string {
	env := os.Environ()
	set := func(key, val string) {
		if val == "" {
			return
		}
		env = append(env, key+"="+val)
	}
	set("PASEKA_TRACE_ID", req.TraceID)
	set("PASEKA_AGENT_ID", req.AgentID)
	set("PASEKA_TASK_ID", req.TaskID)
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
