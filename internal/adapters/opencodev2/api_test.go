package opencodev2

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testAPIClient(server *httptest.Server) *apiClient {
	return &apiClient{baseURL: server.URL, username: apiUsername, password: "secret", http: server.Client()}
}

func TestAPIClientCreateSession(t *testing.T) {
	var gotMethod, gotPath, gotUser, gotPass string
	var gotBody createSessionBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotUser, gotPass, _ = r.BasicAuth()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":{"id":"ses_abc","location":{"directory":"/workspace"},"title":"agent-1","outcome":"succeeded"}}`)
	}))
	defer server.Close()

	rec, err := testAPIClient(server).CreateSession(context.Background(), createSessionBody{
		ID:       "ses_abc",
		Location: &locationRef{Directory: "/workspace"},
		Title:    "agent-1",
		Agent:    "plan",
		Model:    &sessionModel{ProviderID: "anthropic", ID: "claude", Variant: "high"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/session" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotUser != apiUsername || gotPass != "secret" {
		t.Fatalf("basic auth = %q/%q", gotUser, gotPass)
	}
	if gotBody.ID != "ses_abc" || gotBody.Location == nil || gotBody.Location.Directory != "/workspace" || gotBody.Title != "agent-1" || gotBody.Agent != "plan" {
		t.Fatalf("body = %+v", gotBody)
	}
	if gotBody.Model == nil || gotBody.Model.ProviderID != "anthropic" || gotBody.Model.ID != "claude" || gotBody.Model.Variant != "high" {
		t.Fatalf("model = %+v", gotBody.Model)
	}
	if rec.ID != "ses_abc" || rec.Location.Directory != "/workspace" {
		t.Fatalf("record = %+v", rec)
	}
}

// TestCreateSessionBodyMatchesV2Schema pins the wire shape the server validates:
// model uses `id` (not `modelID`) and location is a `{"directory": ...}` object.
func TestCreateSessionBodyMatchesV2Schema(t *testing.T) {
	body := createSessionBody{
		ID:       "ses_abc",
		Location: &locationRef{Directory: "/workspace"},
		Model:    sessionModelFor("anthropic/claude", "high"),
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Location map[string]string `json:"location"`
		Model    map[string]string `json:"model"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Location["directory"] != "/workspace" {
		t.Fatalf("location = %v in %s", raw.Location, data)
	}
	if raw.Model["id"] != "claude" || raw.Model["providerID"] != "anthropic" || raw.Model["variant"] != "high" {
		t.Fatalf("model = %v in %s", raw.Model, data)
	}
	if _, ok := raw.Model["modelID"]; ok {
		t.Fatalf("model must not carry modelID: %s", data)
	}
}

// TestSessionRecordAcceptsLegacyStringLocation keeps reads working against a 2.x
// server that answered with a bare path instead of the location object.
func TestSessionRecordAcceptsLegacyStringLocation(t *testing.T) {
	var rec sessionRecord
	if err := decodeEnvelope([]byte(`{"data":{"id":"ses_abc","location":"/workspace"}}`), &rec); err != nil {
		t.Fatal(err)
	}
	if rec.Location.Directory != "/workspace" {
		t.Fatalf("location = %+v", rec.Location)
	}
}

func TestAPIClientCreateSessionFallsBackWhenIDRejected(t *testing.T) {
	var bodies []createSessionBody
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body createSessionBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		bodies = append(bodies, body)
		if body.ID != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad id"}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"id":"ses_assigned"}}`)
	}))
	defer server.Close()

	rec, err := testAPIClient(server).CreateSession(context.Background(), createSessionBody{ID: "ses_rejected"})
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "ses_assigned" {
		t.Fatalf("record = %+v", rec)
	}
	if len(bodies) != 2 || bodies[0].ID != "ses_rejected" || bodies[1].ID != "" {
		t.Fatalf("bodies = %+v", bodies)
	}
}

func TestAPIClientSessionAndPrompt(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/session/"):
			_, _ = io.WriteString(w, `{"data":{"id":"ses_abc","tokens":{"input":4,"output":1,"reasoning":2},"cost":0.1,"outcome":"completed"}}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/prompt"):
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			if body["text"] != "hello" {
				t.Errorf("prompt body = %+v", body)
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := testAPIClient(server)
	rec, err := client.Session(context.Background(), "ses_abc")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID != "ses_abc" || rec.Cost != 0.1 || rec.Outcome != "completed" {
		t.Fatalf("record = %+v", rec)
	}
	if err := client.Prompt(context.Background(), "ses_abc", "hello"); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /api/session/ses_abc", "POST /api/session/ses_abc/prompt"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestDecodeEnvelopeAcceptsUnwrappedBody(t *testing.T) {
	var out struct {
		ID string `json:"id"`
	}
	if err := decodeEnvelope([]byte(`{"id":"ses_plain"}`), &out); err != nil {
		t.Fatal(err)
	}
	if out.ID != "ses_plain" {
		t.Fatalf("id = %q", out.ID)
	}
}
