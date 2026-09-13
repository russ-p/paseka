package sessions

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/russ-p/paseka/internal/adapters"
)

func TestDeliverSessionPrompt(t *testing.T) {
	type captured struct {
		path string
		auth string
		body map[string]any
	}
	got := make(chan captured, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/session" {
			w.WriteHeader(http.StatusOK)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		user, pass, _ := r.BasicAuth()
		got <- captured{path: r.URL.Path, auth: user + ":" + pass, body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	deliverSessionPrompt(context.Background(), adapters.SessionPromptDelivery{
		BaseURL:       srv.URL,
		SessionID:     "ses_x",
		Username:      "opencode",
		Password:      "secret",
		Text:          "do the thing",
		Agent:         "plan",
		Variant:       "high",
		ModelProvider: "anthropic",
		ModelID:       "claude",
	})

	select {
	case c := <-got:
		if c.path != "/session/ses_x/prompt_async" {
			t.Fatalf("path = %q", c.path)
		}
		if c.auth != "opencode:secret" {
			t.Fatalf("auth = %q", c.auth)
		}
		if c.body["agent"] != "plan" || c.body["variant"] != "high" {
			t.Fatalf("agent/variant = %v/%v", c.body["agent"], c.body["variant"])
		}
		model, _ := c.body["model"].(map[string]any)
		if model["providerID"] != "anthropic" || model["modelID"] != "claude" {
			t.Fatalf("model = %v", model)
		}
		parts, _ := c.body["parts"].([]any)
		if len(parts) != 1 {
			t.Fatalf("parts = %v", parts)
		}
		part, _ := parts[0].(map[string]any)
		if part["type"] != "text" || part["text"] != "do the thing" {
			t.Fatalf("part = %v", part)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("prompt was not delivered")
	}
}

func TestDeliverSessionPromptSkipsEmpty(t *testing.T) {
	// No server needed: empty coordinates must return without network calls.
	deliverSessionPrompt(context.Background(), adapters.SessionPromptDelivery{})
}
