package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/logging"
	"github.com/russ-p/paseka/internal/prompts"
	"github.com/russ-p/paseka/internal/runtime"
)

var (
	// ErrBeeNotFound reports a role with no bee YAML in the colony. A missing file
	// is a client mistake, not a broken colony, so it is a 404 rather than the
	// 500 the wrapped os error would otherwise earn.
	ErrBeeNotFound = errors.New("not found in this colony")
	// ErrRunBodyRequired reports a run asked for with neither a task nor a prompt,
	// and a bee whose adapter cannot invent one. The CLI says "provide --body or
	// --prompt" for the same refusal.
	ErrRunBodyRequired = errors.New("task body or inline prompt is required")
	// ErrNoPromptTemplate reports a task with nothing to render it through: the bee
	// declares no prompt template and the colony defaults none.
	ErrNoPromptTemplate = errors.New("no prompt template configured")
)

// beeRunLog is where a dispatch that outlived its HTTP response says how it went.
var beeRunLog = logging.Component("console")

// BeeRunFunc dispatches one headless bee run. It is Options.BeeRun, so a test can
// answer for the dispatch without an adapter behind it.
type BeeRunFunc func(ctx context.Context, req runtime.BeeRunRequest) (*runtime.BeeRunResult, error)

// RunBeeRequest is the JSON body for POST /api/bees/:role/run.
type RunBeeRequest struct {
	Body         string `json:"body"`
	TraceID      string `json:"traceId"`
	Intent       string `json:"intent"`
	InlinePrompt string `json:"inlinePrompt"`
}

// RunBeeResponse is returned once the dispatch is under way, not when the adapter
// exits: a run lasts as long as the agent takes, and a browser does not.
type RunBeeResponse struct {
	TraceID string `json:"traceId"`
	Bee     string `json:"bee"`
	Message string `json:"message,omitempty"`
}

// RunBee starts one headless run of a bee the way `paseka bee run` does — the
// console process dispatches the adapter itself rather than publishing a signal
// for a runtime that may not be running, so the row works with the hive stopped.
// It is deliberately the direct path and not the cue or task one: a cue is a
// committed standing instruction and a task is ledger work to be reviewed, while
// this is an operator asking one bee for one thing.
//
// Everything that can be refused is refused here, before the answer, because a
// dispatch that fails afterwards has no caller left to tell: the checks are the
// same ones `paseka bee run` makes — the bee exists, and a bee whose adapter
// reads a prompt gets one — plus whether anything is configured to render that
// task through. A failure deeper in — an adapter binary that is not on the path,
// a template file that was deleted — can only reach the log, because the answer
// has been sent by then; the operator guide says so rather than implying the page
// would have shown it.
//
// ctx is the request context and is used for nothing but the checks above; the
// run itself is detached from it, so closing the browser does not kill an agent
// halfway through its work.
func RunBee(ctx context.Context, colonyCtx colony.Context, role string, req RunBeeRequest, run BeeRunFunc) (RunBeeResponse, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		return RunBeeResponse{}, fmt.Errorf("bee role is required")
	}
	if run == nil {
		return RunBeeResponse{}, fmt.Errorf("console: no bee runner configured")
	}

	bee, overlay, err := colonyCtx.LoadBee(role)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return RunBeeResponse{}, fmt.Errorf("%w: %q", ErrBeeNotFound, role)
		}
		return RunBeeResponse{}, err
	}

	body := strings.TrimSpace(req.Body)
	prompt := strings.TrimSpace(req.InlinePrompt)
	if bee.RequiresPrompt() {
		if body == "" && prompt == "" {
			return RunBeeResponse{}, fmt.Errorf("bee %q: %w", role, ErrRunBodyRequired)
		}
		if prompt == "" {
			manifest, err := colony.LoadColony(colonyCtx.ColonyRoot)
			if err != nil {
				return RunBeeResponse{}, err
			}
			// The dispatcher's own resolution, so "nothing to render this task
			// through" is a refusal the operator reads rather than a log line.
			if _, _, err := prompts.Resolve(prompts.ResolveInput{
				BeeLocalTemplate: overlay.PromptTemplate,
				BeeTemplate:      bee.PromptTemplate,
				DefaultTemplate:  manifest.Defaults.PromptTemplate,
			}); err != nil {
				return RunBeeResponse{}, fmt.Errorf("bee %q: %w", role, ErrNoPromptTemplate)
			}
		}
	}

	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		id, err := colony.NewTraceID()
		if err != nil {
			return RunBeeResponse{}, err
		}
		traceID = id
	}

	dispatch := runtime.BeeRunRequest{
		StartDir:     colonyCtx.ColonyRoot,
		Bee:          role,
		TraceID:      traceID,
		Task:         body,
		Intent:       strings.TrimSpace(req.Intent),
		InlinePrompt: prompt,
	}
	go func() {
		res, err := run(context.WithoutCancel(ctx), dispatch)
		if err != nil {
			beeRunLog.Error("bee run failed",
				logging.F("bee", role),
				logging.F("trace", traceID),
				logging.F("error", err.Error()),
			)
			return
		}
		fields := []logging.Field{
			logging.F("bee", role),
			logging.F("trace", traceID),
		}
		if res != nil {
			fields = append(fields,
				logging.F("agent", res.AgentID),
				logging.F("run_dir", runtime.RelRunDir(colonyCtx.ColonyRoot, res.RunDir)),
			)
		}
		beeRunLog.Info("bee run finished", fields...)
	}()

	return RunBeeResponse{
		TraceID: traceID,
		Bee:     role,
		Message: "Run started. It appears under Runs while it works; honey is not spent on it, the same as paseka bee run.",
	}, nil
}

func (a *api) handleBeeByID(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/bees/"), "/")
	if path == "" {
		http.NotFound(w, r)
		return
	}

	parts := strings.Split(path, "/")
	role := parts[0]
	if role == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 && parts[1] == "run" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		a.runBee(w, r, role)
		return
	}
	http.NotFound(w, r)
}

func (a *api) runBee(w http.ResponseWriter, r *http.Request, role string) {
	var req RunBeeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	res, err := RunBee(r.Context(), a.ctx, role, req, a.beeRun)
	if err != nil {
		writeBeeRunError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, res)
}

// writeBeeRunError maps the two refusals a client can act on; anything else is
// the colony's own failure and stays a 500.
func writeBeeRunError(w http.ResponseWriter, err error) {
	msg := err.Error()
	switch {
	case errors.Is(err, ErrBeeNotFound):
		http.Error(w, msg, http.StatusNotFound)
	case errors.Is(err, ErrRunBodyRequired), errors.Is(err, ErrNoPromptTemplate):
		http.Error(w, msg, http.StatusBadRequest)
	default:
		writeError(w, err)
	}
}
