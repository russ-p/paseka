package console_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
	"github.com/russ-p/paseka/internal/console"
	"github.com/russ-p/paseka/internal/sessions"
)

func newConfigServer(t *testing.T, ctxColony colony.Context) *console.Server {
	t.Helper()
	return console.NewServer(console.Options{
		Addr:     "127.0.0.1:0",
		Colony:   ctxColony,
		Sessions: sessions.NewManager(),
	})
}

func getConfigView(t *testing.T, srv *console.Server) console.ConfigView {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/config status = %d body=%s", rec.Code, rec.Body.String())
	}
	var view console.ConfigView
	if err := json.NewDecoder(rec.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	return view
}

func configHomeFile(t *testing.T, slug, name string) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "paseka", slug, name)
}

func adapterByName(t *testing.T, view console.ConfigView, name string) console.ConfigAdapterView {
	t.Helper()
	for _, adapter := range view.Adapters {
		if adapter.Name == name {
			return adapter
		}
	}
	t.Fatalf("adapter %q not in %+v", name, view.Adapters)
	return console.ConfigAdapterView{}
}

func TestConfigAPINamesTheSourceOfEveryValue(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHomeWithNATS(t, repo, "nats://127.0.0.1:4333")

	view := getConfigView(t, newConfigServer(t, ctxColony))

	if view.NATS.URL.Value != "nats://127.0.0.1:4333" {
		t.Fatalf("nats url = %q, want the home config's own value", view.NATS.URL.Value)
	}
	if view.NATS.URL.Source != "config.yaml" {
		t.Fatalf("nats url source = %q, want config.yaml", view.NATS.URL.Source)
	}
	// No subject_prefix is declared, so the prefix is the bus's own default and
	// must say so rather than naming a file that never mentioned it.
	if view.NATS.SubjectPrefix.Value != "paseka."+ctxColony.Slug {
		t.Fatalf("subject prefix = %q, want the bus default for the slug", view.NATS.SubjectPrefix.Value)
	}
	if view.NATS.SubjectPrefix.Source != "default" {
		t.Fatalf("subject prefix source = %q, want default", view.NATS.SubjectPrefix.Source)
	}
}

func TestConfigAPIEnvNATSURLOutranksTheHomeConfig(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHomeWithNATS(t, repo, "nats://127.0.0.1:4333")
	t.Setenv("PASEKA_NATS_URL", "nats://elsewhere:4222")

	view := getConfigView(t, newConfigServer(t, ctxColony))

	// The console connects to the environment's URL, so reporting the file's would
	// be a settings page telling an operator about a value nothing uses.
	if view.NATS.URL.Value != "nats://elsewhere:4222" {
		t.Fatalf("nats url = %q, want the environment's URL", view.NATS.URL.Value)
	}
	if view.NATS.URL.Source != "env:PASEKA_NATS_URL" {
		t.Fatalf("nats url source = %q, want env:PASEKA_NATS_URL", view.NATS.URL.Source)
	}
}

func TestConfigAPIReportsAnUnsetNATSURLRatherThanEmpty(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	view := getConfigView(t, newConfigServer(t, ctxColony))

	if view.NATS.URL.Value != "" || view.NATS.URL.Source != "unset" {
		t.Fatalf("nats url = %+v, want an unset value", view.NATS.URL)
	}
}

func TestConfigAPISeparatesADeclaredAdapterValueFromALoaderDefault(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	// setupConsoleHome writes cursor.yaml holding only `binary`, so the loaded
	// api_key_env is the loader's default and saying otherwise would point an
	// operator at a line of their file that does not exist.
	piPath := configHomeFile(t, ctxColony.Slug, filepath.Join("adapters", "pi.yaml"))
	if err := os.WriteFile(piPath, []byte("binary: pi-alt\napi_key_env: GEMINI_API_KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_API_KEY", "declared-secret")
	t.Setenv("CURSOR_API_KEY", "cursor-secret")

	// The context was resolved before pi.yaml existed, so re-resolve rather than
	// hand-build one: the point of the assertions below is that the response
	// attributes each value to the file that really declares it.
	resolved, err := colony.ResolveContext(repo)
	if err != nil {
		t.Fatal(err)
	}
	view := getConfigView(t, newConfigServer(t, resolved))

	cursor := adapterByName(t, view, "cursor")
	if !cursor.Configured {
		t.Fatal("cursor configured = false, want the file setupConsoleHome wrote")
	}
	if cursor.Binary.Source != "config.yaml" {
		t.Fatalf("cursor binary source = %q, want config.yaml", cursor.Binary.Source)
	}
	if cursor.APIKeyEnv.Source != "default" {
		t.Fatalf("cursor api_key_env source = %q, want default for a key the file omits", cursor.APIKeyEnv.Source)
	}
	if !cursor.APIKeySet {
		t.Fatal("cursor apiKeySet = false, want true for a variable that resolves")
	}

	pi := adapterByName(t, view, "pi")
	if pi.APIKeyEnv.Source != "config.yaml" {
		t.Fatalf("pi api_key_env source = %q, want config.yaml", pi.APIKeyEnv.Source)
	}
	if !pi.APIKeySet {
		t.Fatal("pi apiKeySet = false, want true for a declared and set variable")
	}

	opencode := adapterByName(t, view, "opencode")
	if opencode.APIKeyEnv.Value != "" || opencode.APIKeyEnv.Source != "unset" {
		t.Fatalf("opencode api_key_env = %+v, want unset", opencode.APIKeyEnv)
	}
	if opencode.APIKeySet {
		t.Fatal("opencode apiKeySet = true, want false with no variable to read")
	}
}

func TestConfigAPINeverSendsACredential(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	slug := ctxColony.Slug

	telegram := "enabled: true\nbot_token: 12345:file-token\nallow_from: [7]\nchat_ids: [7]\n"
	if err := os.WriteFile(configHomeFile(t, slug, "telegram.yaml"), []byte(telegram), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CURSOR_API_KEY", "cursor-secret-value")

	rec := httptest.NewRecorder()
	newConfigServer(t, ctxColony).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	body := rec.Body.String()

	for _, secret := range []string{"12345:file-token", "cursor-secret-value"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response carried the credential %q: %s", secret, body)
		}
	}
	// The variable name is not a secret, and the operator cannot act on anything else.
	if !strings.Contains(body, "CURSOR_API_KEY") {
		t.Fatalf("response dropped the variable name: %s", body)
	}
	if !strings.Contains(body, "PASEKA_TELEGRAM_BOT_TOKEN") {
		t.Fatalf("response dropped the token's variable name: %s", body)
	}
}

func TestConfigAPIReportsTheTelegramGateWithoutValidatingIt(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	slug := ctxColony.Slug

	// A gate that is present but disabled is a configuration, not a failure:
	// telegram.Load answers with an error for exactly this file.
	disabled := "enabled: false\nbot_token: 12345:t\nnotify:\n  invites: silent\n  waiting_review: off\n"
	if err := os.WriteFile(configHomeFile(t, slug, "telegram.yaml"), []byte(disabled), 0o600); err != nil {
		t.Fatal(err)
	}

	view := getConfigView(t, newConfigServer(t, ctxColony))

	if !view.Telegram.Present {
		t.Fatal("telegram present = false, want the file that was written")
	}
	if view.Telegram.Enabled {
		t.Fatal("telegram enabled = true, want false")
	}
	if !view.Telegram.BotTokenSet {
		t.Fatal("botTokenSet = false, want true for a token in the file")
	}

	byCategory := map[string]string{}
	for _, entry := range view.Telegram.Notify {
		byCategory[entry.Category] = entry.Mode
	}
	if byCategory["invites"] != "silent" {
		t.Fatalf("invites = %q, want silent", byCategory["invites"])
	}
	// waiting_review is legacy and maps onto both review categories, so it is
	// applied here and never reported as a category of its own.
	if _, present := byCategory["waiting_review"]; present {
		t.Fatalf("categories = %v, want no legacy waiting_review", byCategory)
	}
	if byCategory["review_required"] != "off" || byCategory["review_final"] != "off" {
		t.Fatalf("review categories = %+v, want the legacy off applied to both", byCategory)
	}
	// A category the file never mentions still reports the gate's own default,
	// because that is the mode a push would actually use.
	if byCategory["commit_gate"] != "off" {
		t.Fatalf("commit_gate = %q, want the default off", byCategory["commit_gate"])
	}
	if len(byCategory) != 7 {
		t.Fatalf("categories = %v, want the seven canonical ones", byCategory)
	}
}

func TestConfigAPIReportsAMissingTelegramGateAsAbsent(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	view := getConfigView(t, newConfigServer(t, ctxColony))

	if view.Telegram.Present {
		t.Fatal("telegram present = true, want false for a colony with no telegram.yaml")
	}
	if view.Telegram.Enabled {
		t.Fatal("telegram enabled = true, want false")
	}
	// An absent file must not borrow the loader's defaults and report a mode
	// nobody chose.
	if view.Telegram.Mode.Value != "" {
		t.Fatalf("telegram mode = %+v, want empty for a file that is not there", view.Telegram.Mode)
	}
	if len(view.Telegram.Notify) != 0 {
		t.Fatalf("notify = %+v, want none for an absent gate", view.Telegram.Notify)
	}
}

func TestConfigAPIRejectsAWrite(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	rec := httptest.NewRecorder()
	newConfigServer(t, ctxColony).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader("{}")))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdapterConfigDeclaredSeparatesPresenceFromDeclaredKeys(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)
	slug := ctxColony.Slug

	// setupConsoleHome writes cursor.yaml holding only `binary`.
	exists, keys := colony.AdapterConfigDeclared(slug, "cursor")
	if !exists {
		t.Fatal("cursor exists = false, want the file setupConsoleHome wrote")
	}
	if !keys["binary"] {
		t.Fatalf("cursor keys = %v, want binary declared", keys)
	}
	if keys["api_key_env"] {
		t.Fatalf("cursor keys = %v, want no api_key_env", keys)
	}

	if exists, _ := colony.AdapterConfigDeclared(slug, "pi"); exists {
		t.Fatal("pi exists = true, want false before the file is written")
	}

	piPath := configHomeFile(t, slug, filepath.Join("adapters", "pi.yaml"))
	if err := os.WriteFile(piPath, []byte("binary: pi\napi_key_env: GEMINI_API_KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	exists, keys = colony.AdapterConfigDeclared(slug, "pi")
	if !exists || !keys["api_key_env"] {
		t.Fatalf("pi exists=%v keys=%v, want the declared key found", exists, keys)
	}

	// A file that does not parse is reported as absent rather than as a second,
	// quieter error: the loader already fails on it in the request path.
	badPath := configHomeFile(t, slug, filepath.Join("adapters", "claude.yaml"))
	if err := os.WriteFile(badPath, []byte("binary: [oops\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if exists, _ := colony.AdapterConfigDeclared(slug, "claude"); exists {
		t.Fatal("claude exists = true, want false for an unparseable file")
	}
}

// A fixture returning `[]` and `{}` hides a null list and a wrongly-cased field
// key, and both have bitten this API before — `runs: null` and `taskCounts: null`
// were found by running against a live console, not by a test. This asserts the
// raw body rather than the decoded struct for that reason.
func TestConfigAPISendsNoNullListsAndOnlyCamelCaseKeys(t *testing.T) {
	repo := initConsoleRepo(t)
	ctxColony := setupConsoleHome(t, repo)

	rec := httptest.NewRecorder()
	newConfigServer(t, ctxColony).Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	body := rec.Body.String()

	if strings.Contains(body, `":null`) {
		t.Fatalf("response carried a null list, which a client has to special-case: %s", body)
	}
	// `colony.ProfileLayers` has no JSON tags, so embedding it would have
	// marshalled this object as `Colony`/`Home` in a camelCase API.
	for _, key := range []string{`"colony"`, `"home"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("response is missing the %s key: %s", key, body)
		}
	}
	for _, wrong := range []string{`"Colony"`, `"Home"`, `"Notify"`, `"APIKeySet"`} {
		if strings.Contains(body, wrong) {
			t.Fatalf("response carries the Go field name %s: %s", wrong, body)
		}
	}
}
