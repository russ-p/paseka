package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/sessions"
)

func TestSystemAPIHandlers(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/system", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.SystemView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.OS == "" || view.Arch == "" {
		t.Fatalf("os/arch missing: %+v", view)
	}
	if view.ConsolePID == 0 {
		t.Fatal("consolePid missing")
	}
	if view.CPUs < 1 {
		t.Fatalf("cpus = %d", view.CPUs)
	}
	if view.GoVersion == "" {
		t.Fatal("goVersion missing")
	}
	if len(view.Processes) > 25 {
		t.Fatalf("process cap exceeded: %d", len(view.Processes))
	}
	if runtime.GOOS == "linux" {
		if view.MemTotalBytes == nil || *view.MemTotalBytes == 0 {
			t.Fatalf("linux snapshot missing memory: %+v", view)
		}
		if view.Load1 == nil {
			t.Fatalf("linux snapshot missing load: %+v", view)
		}
	}

	post := httptest.NewRequest(http.MethodPost, "/api/system", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, post)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d", postRec.Code)
	}
}
