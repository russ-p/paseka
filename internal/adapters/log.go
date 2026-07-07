package adapters

import (
	"strings"
	"time"

	"github.com/paseka/paseka/internal/logging"
)

// LogAgentLaunch logs adapter process start.
func LogAgentLaunch(log *logging.Logger, adapter, binary string, req RunRequest, args []string) {
	if log == nil {
		log = logging.Component("adapter")
	}
	log.Info("agent launch",
		logging.F("adapter", adapter),
		logging.F("bee", req.Bee),
		logging.F("trace", req.TraceID),
		logging.F("agent", req.AgentID),
		logging.F("binary", binary),
		logging.F("workspace", req.Workspace),
	)
	log.Debug("agent args",
		logging.F("adapter", adapter),
		logging.F("trace", req.TraceID),
		logging.F("agent", req.AgentID),
		logging.F("args", strings.Join(RedactArgs(args), " ")),
	)
}

// LogAgentDone logs adapter process completion.
func LogAgentDone(log *logging.Logger, adapter, binary string, req RunRequest, started time.Time, status string, exitCode int, runErr error) {
	if log == nil {
		log = logging.Component("adapter")
	}
	fields := []logging.Field{
		logging.F("adapter", adapter),
		logging.F("bee", req.Bee),
		logging.F("trace", req.TraceID),
		logging.F("agent", req.AgentID),
		logging.F("binary", binary),
		logging.F("status", status),
		logging.F("exit_code", itoa(exitCode)),
		logging.F("duration", time.Since(started).Round(time.Millisecond).String()),
	}
	if runErr != nil {
		fields = append(fields, logging.F("error", runErr.Error()))
	}
	if status == "failed" || status == "cancelled" {
		log.Warn("agent done", fields...)
		return
	}
	log.Info("agent done", fields...)
}

// RedactArgs returns CLI args safe for debug logging.
func RedactArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--api-key":
			out = append(out, arg, "<redacted>")
			i++
			continue
		}
		if strings.HasPrefix(arg, "--api-key=") {
			out = append(out, "--api-key=<redacted>")
			continue
		}
		// Cursor/Pi pass the rendered prompt as the final positional arg.
		if i == len(args)-1 && len(arg) > 120 {
			out = append(out, "<prompt>")
			continue
		}
		out = append(out, arg)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
