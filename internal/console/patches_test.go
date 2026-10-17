package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/patches"
	"github.com/russ-p/paseka/internal/protocol"
	"github.com/russ-p/paseka/internal/sessions"
)

func TestTraceProposalPatchesAPIHandler(t *testing.T) {
	repo := initConsoleRepo(t)
	traceID := "trace-proposals-api"
	store := patches.NewStore(repo, traceID)
	if _, err := store.Write(patches.Record{
		AgentID:     "agent-1",
		Bee:         "builder",
		Mode:        patches.ModeAFK,
		Kind:        string(protocol.MutationCodeProposalIsolated),
		Workspace:   "isolated",
		Summary:     "add the export flag",
		CapturedAt:  time.Date(2026, 1, 9, 10, 15, 0, 0, time.UTC),
		FileCount:   patches.FileCountFromDiff("diff --git a/x b/x\n"),
		Publication: patches.PublicationPublished,
	}, []byte("diff --git a/x b/x\n+added\n")); err != nil {
		t.Fatal(err)
	}
	// An incomplete record must be diagnosed, not hidden and not fatal.
	if err := os.WriteFile(store.MetaPath("agent-broken"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	listReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/proposal-patches", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listRec.Code, listRec.Body.String())
	}
	var list struct {
		Patches []struct {
			AgentID     string `json:"agentId"`
			Publication string `json:"publication"`
			Bytes       int64  `json:"bytes"`
		} `json:"patches"`
		Diagnostics []struct {
			AgentID string `json:"agentId"`
			Reason  string `json:"reason"`
		} `json:"diagnostics"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Patches) != 1 || list.Patches[0].AgentID != "agent-1" {
		t.Fatalf("patches = %+v", list.Patches)
	}
	if list.Patches[0].Publication != patches.PublicationPublished {
		t.Fatalf("publication = %q", list.Patches[0].Publication)
	}
	if list.Patches[0].Bytes == 0 {
		t.Fatal("the listing must carry the recorded size")
	}
	if len(list.Diagnostics) != 1 || list.Diagnostics[0].AgentID != "agent-broken" {
		t.Fatalf("diagnostics = %+v", list.Diagnostics)
	}
	// The listing must not have read the body: the response carries no content field.
	if strings.Contains(listRec.Body.String(), "+added") {
		t.Fatalf("the listing read a patch body:\n%s", listRec.Body.String())
	}

	q := url.Values{"agentId": {"agent-1"}}
	contentReq := httptest.NewRequest(http.MethodGet, "/api/traces/"+traceID+"/proposal-patches?"+q.Encode(), nil)
	contentRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(contentRec, contentReq)
	if contentRec.Code != http.StatusOK {
		t.Fatalf("content status = %d", contentRec.Code)
	}
	var content struct {
		Content string `json:"content"`
		Bytes   int64  `json:"bytes"`
	}
	if err := json.NewDecoder(contentRec.Body).Decode(&content); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content.Content, "+added") {
		t.Fatalf("content = %+v", content)
	}
}

// A trail with no proposal directory reads as an empty list, not an error, and the JSON
// carries `[]` rather than `null` so the page never has to guard the shape.
func TestTraceProposalPatchesAPIHandlerOnAnEmptyTrail(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traces/trace-nothing/proposal-patches", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"patches":[]`) || !strings.Contains(body, `"diagnostics":[]`) {
		t.Fatalf("empty trail must read as empty lists: %s", body)
	}
}

// A missing record is a 404-ish error rather than an empty body, so a mistyped agent id is
// distinguishable from a record with no content.
func TestTraceProposalPatchContentMissingRecord(t *testing.T) {
	repo := initConsoleRepo(t)
	srv := console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   colony.Context{ColonyRoot: repo, Slug: "console-test"},
		Sessions: sessions.NewManager(),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traces/trace-x/proposal-patches?agentId=nobody", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected an error status, got 200: %s", rec.Body.String())
	}
}
