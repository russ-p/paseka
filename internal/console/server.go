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

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/runtime"
	"github.com/paseka/paseka/internal/sessions"
	"golang.org/x/term"
)

// Options configures the Queen Console HTTP server.
type Options struct {
	Addr     string
	Colony   colony.Context
	Sessions *sessions.Manager
	Runtime  *runtime.Supervisor
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
	apiHandler := &api{ctx: opts.Colony, sessions: mgr, runtime: runtimeSup}
	mux.HandleFunc("/api/runtime", apiHandler.handleRuntime)
	mux.HandleFunc("/api/runtime/start", apiHandler.handleRuntimeStart)
	mux.HandleFunc("/api/runtime/stop", apiHandler.handleRuntimeStop)
	mux.HandleFunc("/api/bees", apiHandler.handleBees)
	mux.HandleFunc("/api/sessions", apiHandler.handleSessions)
	mux.HandleFunc("/api/sessions/", apiHandler.handleSessionByID)
	mux.HandleFunc("/api/runs", apiHandler.handleRuns)
	mux.HandleFunc("/api/runs/", apiHandler.handleRunByID)

	staticFS, _ := fs.Sub(staticFiles, "static")
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
	fmt.Printf("%s listening at http://%s\n", boldYellow("Queen Console 🐝"), host)

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

func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/api/") {
			if _, err := fs.Stat(staticFS, strings.TrimPrefix(r.URL.Path, "/")); err != nil {
				r.URL.Path = "/"
			}
		}
		fileServer.ServeHTTP(w, r)
	})
}
