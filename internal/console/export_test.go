package console_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/runs"
	"github.com/russ-p/paseka/internal/sessions"
)

// writeExportRun seeds one completed run so hiveview.GetTrace finds the trail
// and internal/export has usage, a status, and an event to render.
func writeExportRun(t *testing.T, repo, traceID string) {
	t.Helper()
	const agentID = "agent-export"
	d := runs.Dir{ColonyRoot: repo, TraceID: traceID, AgentID: agentID}
	if err := d.Prepare(); err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC().Add(-time.Minute)
	if err := d.WriteRequest(protocol.Request{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Bee:             "scout",
		Adapter:         "cursor",
		Workspace:       repo,
		ColonyRoot:      repo,
		CreatedAt:       started,
	}); err != nil {
		t.Fatal(err)
	}
	finished := started.Add(30 * time.Second)
	if err := d.WriteStatusSnapshot(protocol.StatusSnapshot{
		ProtocolVersion: protocol.Version,
		State:           protocol.StatusCompleted,
		StartedAt:       started,
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteResult(protocol.Result{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Status:          protocol.StatusCompleted,
		Summary:         "done",
		Usage:           &protocol.Usage{InputTokens: 120, OutputTokens: 30},
		FinishedAt:      finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.AppendEvent(protocol.Event{
		ProtocolVersion: protocol.Version,
		TraceID:         traceID,
		AgentID:         agentID,
		Type:            protocol.EventInsight,
		CreatedAt:       started.Add(2 * time.Second),
		Payload:         []byte(`{"kind":"narrative","text":"hello export"}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTraceExportAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	ctx := setupConsoleHome(t, repo)
	traceID := "trace-export-api"
	writeExportRun(t, repo, traceID)

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctx,
		Sessions: sessions.NewManager(),
	})

	// The default is HTML, served as an attachment whose name internal/export owns.
	htmlReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/export", nil)
	htmlRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(htmlRec, htmlReq)
	if htmlRec.Code != http.StatusOK {
		t.Fatalf("html status = %d body=%s", htmlRec.Code, htmlRec.Body.String())
	}
	if got := htmlRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("html content type = %q", got)
	}
	disposition := htmlRec.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") || !strings.Contains(disposition, ".html") {
		t.Fatalf("html disposition = %q", disposition)
	}
	if !strings.Contains(htmlRec.Body.String(), traceID) {
		t.Fatalf("html body missing trace id:\n%s", htmlRec.Body.String())
	}

	// Markdown is its own content type and extension.
	mdReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/export?format=md", nil)
	mdRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(mdRec, mdReq)
	if mdRec.Code != http.StatusOK {
		t.Fatalf("md status = %d body=%s", mdRec.Code, mdRec.Body.String())
	}
	if got := mdRec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/markdown") {
		t.Fatalf("md content type = %q", got)
	}
	if !strings.Contains(mdRec.Header().Get("Content-Disposition"), ".md") {
		t.Fatalf("md disposition = %q", mdRec.Header().Get("Content-Disposition"))
	}

	// The include list reaches the renderer: the colony YAML is embedded only when asked.
	includeReq := httptest.NewRequest(
		http.MethodGet,
		"/api/traces/"+traceID+"/export?format=md&include=colony&include=usage",
		nil,
	)
	includeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(includeRec, includeReq)
	if includeRec.Code != http.StatusOK {
		t.Fatalf("include status = %d body=%s", includeRec.Code, includeRec.Body.String())
	}
	body := includeRec.Body.String()
	for _, want := range []string{"## Colony", "**Usage:** in 120 / out 30"} {
		if !strings.Contains(body, want) {
			t.Fatalf("include body missing %q:\n%s", want, body)
		}
	}
}

func TestTraceExportAPIErrors(t *testing.T) {
	repo := initConsoleRepo(t)
	ctx := setupConsoleHome(t, repo)
	traceID := "trace-export-errors"
	writeExportRun(t, repo, traceID)

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctx,
		Sessions: sessions.NewManager(),
	})

	cases := []struct {
		name string
		url  string
		want int
	}{
		{"unsupported format", "/api/traces/" + traceID + "/export?format=pdf", http.StatusBadRequest},
		{"unsupported include", "/api/traces/" + traceID + "/export?include=nope", http.StatusBadRequest},
		{"unknown trail", "/api/traces/trace-does-not-exist/export", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			// The body is the server's reason, which is exactly what the console toasts.
			if strings.TrimSpace(rec.Body.String()) == "" {
				t.Fatal("expected an error message")
			}
		})
	}
}
