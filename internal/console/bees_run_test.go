package console_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/runtime"
)

// beeRunRecorder is a stand-in for the adapter dispatch: it answers for the run
// and hands back what it was asked, so a test can assert on the request the
// console built without a cursor binary behind it.
type beeRunRecorder struct {
	called chan runtime.BeeRunRequest
	result chan *runtime.BeeRunResult
	err    chan error
	// release gates the answer, so a test can end the HTTP request first and learn
	// whether the run outlives it.
	release chan struct{}
	// ctxErr is the dispatch context's error at the moment the answer was given.
	ctxErr chan error
}

func newBeeRunRecorder() *beeRunRecorder {
	return &beeRunRecorder{
		called:  make(chan runtime.BeeRunRequest, 1),
		result:  make(chan *runtime.BeeRunResult, 1),
		err:     make(chan error, 1),
		release: make(chan struct{}),
		ctxErr:  make(chan error, 1),
	}
}

func (r *beeRunRecorder) run(ctx context.Context, req runtime.BeeRunRequest) (*runtime.BeeRunResult, error) {
	r.called <- req
	<-r.release
	r.ctxErr <- ctx.Err()
	select {
	case err := <-r.err:
		return nil, err
	default:
		return &runtime.BeeRunResult{
			TraceID: req.TraceID,
			AgentID: "agent-console",
			RunDir:  filepath.Join(".paseka", "runs", req.TraceID, "agent-console"),
		}, nil
	}
}

func beeRunServer(t *testing.T, repo string, rec *beeRunRecorder) *console.Server {
	t.Helper()
	return console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   setupConsoleHome(t, repo),
		Sessions: nil,
		BeeRun:   rec.run,
	})
}

func postBeeRun(t *testing.T, srv *console.Server, role, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/bees/"+role+"/run", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// A run is launched and answered with the trail it will appear under, not with
// the adapter's output: the agent has not finished, and may not have started.
func TestRunBeeAPIStartsARunAndAnswersWithItsTrail(t *testing.T) {
	repo := initConsoleRepo(t)
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	resp := postBeeRun(t, srv, "scout", `{"body":"check the failing test","intent":"general"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	var view console.RunBeeResponse
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Bee != "scout" {
		t.Fatalf("bee = %q, want scout", view.Bee)
	}
	if view.TraceID == "" {
		t.Fatal("traceId is empty; the console has nothing to link the run to")
	}

	var dispatch runtime.BeeRunRequest
	select {
	case dispatch = <-rec.called:
	case <-time.After(5 * time.Second):
		t.Fatal("the console answered without dispatching anything")
	}
	if dispatch.Bee != "scout" || dispatch.Task != "check the failing test" || dispatch.Intent != "general" {
		t.Fatalf("dispatch = %+v", dispatch)
	}
	if dispatch.TraceID != view.TraceID {
		t.Fatalf("dispatch trace = %q, response trace = %q", dispatch.TraceID, view.TraceID)
	}
	if dispatch.StartDir != repo {
		t.Fatalf("dispatch start dir = %q, want %q", dispatch.StartDir, repo)
	}
	close(rec.release)
}

// Closing the browser must not kill an agent halfway through its work, so the
// dispatch runs on a context detached from the request.
func TestRunBeeAPIDispatchOutlivesTheRequest(t *testing.T) {
	repo := initConsoleRepo(t)
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/bees/scout/run", bytes.NewBufferString(`{"body":"go"}`)).WithContext(ctx)
	resp := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}

	select {
	case <-rec.called:
	case <-time.After(5 * time.Second):
		t.Fatal("the console answered without dispatching anything")
	}
	cancel()
	close(rec.release)
	select {
	case err := <-rec.ctxErr:
		if err != nil {
			t.Fatalf("dispatch context = %v after the request was cancelled, want it still live", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the dispatch never finished")
	}
}

// An operator naming a bee that is not in the colony gets to hear so, rather
// than a 500 carrying a wrapped os error.
func TestRunBeeAPIUnknownBeeIs404(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := beeRunServer(t, repo, newBeeRunRecorder())

	resp := postBeeRun(t, srv, "ghost", `{"body":"go"}`)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("not found")) {
		t.Fatalf("body = %q, want it to say the bee is not found", resp.Body.String())
	}
}

// A prompt-driven bee needs a task; the CLI refuses the same run with
// "provide --body or --prompt".
func TestRunBeeAPIRequiresATaskForAPromptBee(t *testing.T) {
	repo := initConsoleRepo(t)
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	resp := postBeeRun(t, srv, "scout", `{}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	select {
	case <-rec.called:
		t.Fatal("a refused run was dispatched anyway")
	case <-time.After(100 * time.Millisecond):
	}
}

// The prompt template is what a task is rendered through, so a bee without one
// is refused up front — inside the dispatch there would be nobody left to tell.
func TestRunBeeAPIRefusesABeeWithNoPromptTemplate(t *testing.T) {
	repo := initConsoleRepo(t)
	writeBeeYAML(t, repo, "teller", "role: teller\nadapter: cursor\n")
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	resp := postBeeRun(t, srv, "teller", `{"body":"count the bees"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !bytes.Contains(resp.Body.Bytes(), []byte("prompt template")) {
		t.Fatalf("body = %q, want it to name the missing prompt template", resp.Body.String())
	}

	// A raw prompt replaces the template, so the same bee runs when one is given.
	ok := postBeeRun(t, srv, "teller", `{"inlinePrompt":"count the bees"}`)
	if ok.Code != http.StatusCreated {
		t.Fatalf("raw prompt status = %d body=%s", ok.Code, ok.Body.String())
	}
	select {
	case dispatch := <-rec.called:
		if dispatch.InlinePrompt != "count the bees" || dispatch.Task != "" {
			t.Fatalf("dispatch = %+v", dispatch)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the console answered without dispatching anything")
	}
}

// A script bee runs its own command, so it is the one case where an empty task
// is a run rather than a refusal.
func TestRunBeeAPIRunsAScriptBeeWithNoTask(t *testing.T) {
	repo := initConsoleRepo(t)
	writeBeeYAML(t, repo, "sweeper", "role: sweeper\nadapter: script\ncommand: ./sweep.sh\n")
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	resp := postBeeRun(t, srv, "sweeper", `{}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	select {
	case <-rec.called:
	case <-time.After(5 * time.Second):
		t.Fatal("the console answered without dispatching anything")
	}
}

// A trail id the operator supplied is the trail the run joins, so a second run
// continues one trail instead of opening another.
func TestRunBeeAPIKeepsAnExplicitTrail(t *testing.T) {
	repo := initConsoleRepo(t)
	rec := newBeeRunRecorder()
	srv := beeRunServer(t, repo, rec)

	resp := postBeeRun(t, srv, "scout", `{"body":"go","traceId":"trail-daily"}`)
	if resp.Code != http.StatusCreated {
		t.Fatalf("run status = %d body=%s", resp.Code, resp.Body.String())
	}
	var view console.RunBeeResponse
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.TraceID != "trail-daily" {
		t.Fatalf("traceId = %q, want trail-daily", view.TraceID)
	}
}

func TestBeeRunRouteRejectsAnythingButAPost(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := beeRunServer(t, repo, newBeeRunRecorder())

	for _, path := range []string{"/api/bees/scout/run", "/api/bees/scout", "/api/bees/scout/chat"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		srv.Handler().ServeHTTP(resp, req)
		want := http.StatusMethodNotAllowed
		if path != "/api/bees/scout/run" {
			want = http.StatusNotFound
		}
		if resp.Code != want {
			t.Fatalf("GET %s status = %d, want %d", path, resp.Code, want)
		}
	}
}
