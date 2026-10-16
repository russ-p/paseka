package runtime

import (
	"context"
	"strings"

	"github.com/russ-p/paseka/internal/adapters"
	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/runs"
)

func postExecVars(prompt, workspace, resultText string, runDir runs.Dir, traceID, agentID, taskID, colonyRoot string, env []string) colony.CommandVars {
	return colony.CommandVars{
		Prompt:     prompt,
		Workspace:  workspace,
		TraceID:    traceID,
		AgentID:    agentID,
		TaskID:     taskID,
		ColonyRoot: colonyRoot,
		Result:     strings.TrimSpace(resultText),
		ResultFile: runDir.ResultPath(),
		Meta:       runDir.MetaPath(),
		RunDir:     runDir.Root(),
		Env:        adapters.EnvMap(env),
	}
}

func resultText(result *adapters.RunResult) string {
	if result == nil {
		return ""
	}
	if s := strings.TrimSpace(result.Summary); s != "" {
		return s
	}
	return strings.TrimSpace(result.Output)
}

func (d *Dispatcher) runPostExec(
	ctx context.Context,
	bee colony.Bee,
	prompt, workspace string,
	runDir runs.Dir,
	taskID string,
	result *adapters.RunResult,
	env []string,
) {
	if !bee.PostExec.IsSet() {
		return
	}
	vars := postExecVars(prompt, workspace, resultText(result), runDir, runDir.TraceID, runDir.AgentID, taskID, runDir.ColonyRoot, env)
	if err := colony.RunPostExec(ctx, bee.PostExec, vars, workspace, env); err != nil {
		runtimeLog.Warn("post_exec failed",
			logging.F("bee", bee.Role),
			logging.F("trace", runDir.TraceID),
			logging.F("agent", runDir.AgentID),
			logging.F("error", err.Error()),
		)
	}
}
