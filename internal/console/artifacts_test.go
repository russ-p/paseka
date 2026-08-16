package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/paseka/paseka/internal/colony"
	"github.com/paseka/paseka/internal/console"
	"github.com/paseka/paseka/internal/hiveview"
	"github.com/paseka/paseka/internal/sessions"
)

func TestTraceArtifactsAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-artifacts-api"
	comb := filepath.Join(repo, ".paseka", "runs", traceID, "artifacts")
	if err := os.MkdirAll(comb, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(comb, "research.md"), []byte("# Research\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var items []hiveview.ArtifactView
	if err := json.NewDecoder(listRec.Body).Decode(&items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ArtifactKind != "research" {
		t.Fatalf("items = %+v", items)
	}
	if !items[0].Staged || items[0].Announced {
		t.Fatalf("expected staged only: %+v", items[0])
	}

	q := url.Values{"ref": {items[0].Ref}}
	contentReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/artifacts?"+q.Encode(), nil)
	contentRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(contentRec, contentReq)
	if contentRec.Code != http.StatusOK {
		t.Fatalf("content status = %d", contentRec.Code)
	}
	var view hiveview.ArtifactContentView
	if err := json.NewDecoder(contentRec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	if view.Content == "" || view.ContentHTML == "" {
		t.Fatalf("preview = %+v", view)
	}
}
