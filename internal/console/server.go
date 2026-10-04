package console

import (
	"context"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/runtime"
	"github.com/russ-p/paseka/internal/sessions"
	"golang.org/x/term"
)

// Options configures the Queen Console HTTP server.
type Options struct {
	Addr     string
	Colony   colony.Context
	Sessions *sessions.Manager
	Runtime  *runtime.Supervisor
	// BeeRun dispatches one headless bee run for POST /api/bees/:role/run. Nil
	// builds a fresh dispatcher per run, which is what `paseka bee run` uses, so
	// the console launches the adapter itself rather than asking a runtime for it.
	BeeRun BeeRunFunc
}

// Server serves the Queen Console sessions UI and JSON API.
type Server struct {
	addr     string
	ctx      colony.Context
	sessions *sessions.Manager
	runtime  *runtime.Supervisor
	http     *http.Server
}

// NewServer builds a console server for the given colony context.
func NewServer(opts Options) *Server {
	addr := opts.Addr
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	mgr := opts.Sessions
	if mgr == nil {
		mgr = sessions.NewManager()
	}
	runtimeSup := opts.Runtime
	if runtimeSup == nil {
		runtimeSup = runtime.DefaultSupervisor()
	}
	s := &Server{
		addr:     addr,
		ctx:      opts.Colony,
		sessions: mgr,
		runtime:  runtimeSup,
	}
	mux := http.NewServeMux()
	apiHandler := &api{ctx: opts.Colony, sessions: mgr, runtime: runtimeSup, sampler: newCPUSampler()}
	apiHandler.chrome = newChromeHub(apiHandler)
	apiHandler.beeRun = opts.BeeRun
	if apiHandler.beeRun == nil {
		apiHandler.beeRun = func(ctx context.Context, req runtime.BeeRunRequest) (*runtime.BeeRunResult, error) {
			return runtime.NewDispatcher().BeeRun(ctx, req)
		}
	}
	mux.HandleFunc("/api/runtime", apiHandler.handleRuntime)
	mux.HandleFunc("/api/runtime/start", apiHandler.handleRuntimeStart)
	mux.HandleFunc("/api/runtime/stop", apiHandler.handleRuntimeStop)
	mux.HandleFunc("/api/agents", apiHandler.handleAgents)
	mux.HandleFunc("/api/system", apiHandler.handleSystem)
	mux.HandleFunc("/api/version", apiHandler.handleVersion)
	mux.HandleFunc("/api/git", apiHandler.handleGit)
	mux.HandleFunc("/api/git/fetch", apiHandler.handleGitFetch)
	mux.HandleFunc("/api/git/push", apiHandler.handleGitPush)
	mux.HandleFunc("/api/git/pull", apiHandler.handleGitPull)
	mux.HandleFunc("/api/git/branches/delete", apiHandler.handleGitBranchesDelete)
	mux.HandleFunc("/api/git/worktrees/prune", apiHandler.handleGitWorktreesPrune)
	mux.HandleFunc("/api/chrome/stream", apiHandler.handleChromeStream)
	mux.HandleFunc("/api/dashboard", apiHandler.handleDashboard)
	mux.HandleFunc("/api/cues", apiHandler.handleCues)
	mux.HandleFunc("/api/cues/", apiHandler.handleCueByID)
	mux.HandleFunc("/api/tasks", apiHandler.handleTasks)
	mux.HandleFunc("/api/review-queue", apiHandler.handleReviewQueue)
	mux.HandleFunc("/api/traces", apiHandler.handleTraces)
	mux.HandleFunc("/api/traces/", apiHandler.handleTraceByID)
	mux.HandleFunc("/api/events", apiHandler.handleEvents)
	mux.HandleFunc("/api/bees", apiHandler.handleBees)
	mux.HandleFunc("/api/bees/", apiHandler.handleBeeByID)
	mux.HandleFunc("/api/config", apiHandler.handleConfig)
	mux.HandleFunc("/api/colony/topology", apiHandler.handleColonyTopology)
	mux.HandleFunc("/api/sessions", apiHandler.handleSessions)
	mux.HandleFunc("/api/sessions/", apiHandler.handleSessionByID)
	mux.HandleFunc("/api/invites", apiHandler.handleInvites)
	mux.HandleFunc("/api/invites/", apiHandler.handleInviteByID)
	mux.HandleFunc("/api/runs", apiHandler.handleRuns)
	mux.HandleFunc("/api/runs/", apiHandler.handleRunByID)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/next", http.RedirectHandler("/next/", http.StatusPermanentRedirect))
	mux.Handle("/next/", nextSPAHandler(nextFiles))
	mux.Handle("/", spaHandler(staticFS))

	s.http = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Handler exposes the HTTP handler (for tests).
func (s *Server) Handler() http.Handler {
	return s.http.Handler
}

// Run starts the HTTP server and blocks until ctx is cancelled or the server exits.
func (s *Server) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	host := s.addr
	if strings.HasPrefix(host, ":") {
		host = "127.0.0.1" + host
	}
	for _, line := range startupBanner(host) {
		fmt.Println(line)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.http.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.sessions.StopAll()
		_ = s.http.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func boldYellow(s string) string {
	if os.Getenv("NO_COLOR") != "" {
		return s
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return s
	}
	return "\033[1;33m" + s + "\033[0m"
}

// startupBanner is what a `paseka console` says when it takes the port: where to
// open it, which build is answering, and that the redesigned console lives under
// a second URL. The build sits in the banner rather than only in the UI because
// the process about to hold a port is the one an operator will want to name in a
// bug report an hour later, and a scrollback scroll is the last place they look.
func startupBanner(host string) []string {
	return []string{
		fmt.Sprintf("%s listening at http://%s", boldYellow("Queen Console 🐝"), host),
		fmt.Sprintf("  %s", buildLine()),
		fmt.Sprintf("  Redesign preview: http://%s/next/", host),
	}
}

const nextConsoleBasePath = "/next/"

func nextSPAHandler(consoleFiles fs.FS) http.Handler {
	distFS, err := fs.Sub(consoleFiles, "next/dist")
	if err != nil {
		panic(fmt.Sprintf("prepare next console assets: %v", err))
	}
	fallbackHTML, err := fs.ReadFile(consoleFiles, "next/fallback.html")
	if err != nil {
		panic(fmt.Sprintf("read next console fallback: %v", err))
	}
	fileServer := http.FileServer(http.FS(distFS))
	_, buildErr := fs.Stat(distFS, "200.html")
	hasBuild := buildErr == nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := strings.TrimPrefix(r.URL.Path, nextConsoleBasePath)
		rel = strings.TrimPrefix(rel, "/")
		if hasBuild {
			if rel == "" {
				rel = "200.html"
			}
			if _, err := fs.Stat(distFS, rel); err != nil {
				if looksLikeStaticAsset(rel) {
					http.NotFound(w, r)
					return
				}
				rel = "200.html"
			}
			request := r.Clone(r.Context())
			url := *r.URL
			url.Path = "/" + rel
			request.URL = &url
			fileServer.ServeHTTP(w, request)
			return
		}
		if rel != "" && looksLikeStaticAsset(rel) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(fallbackHTML)
	})
}

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/") {
			rel := strings.TrimPrefix(r.URL.Path, "/")
			if _, err := fs.Stat(staticFS, rel); err != nil {
				if looksLikeStaticAsset(rel) {
					http.NotFound(w, r)
					return
				}
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}

// looksLikeStaticAsset reports paths whose last segment has a file extension.
// Missing assets must 404 instead of falling back to index.html (otherwise
// go install binaries that omit files look like a working SPA serving HTML as JS).
func looksLikeStaticAsset(path string) bool {
	base := path
	if i := strings.LastIndex(path, "/"); i >= 0 {
		base = path[i+1:]
	}
	dot := strings.LastIndex(base, ".")
	return dot > 0 && dot < len(base)-1
}
