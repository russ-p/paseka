package redact

import (
	"strings"
	"testing"
)

func TestTextRedactsPatternClasses(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "authorization bearer header",
			in:   `curl -H 'Authorization: Bearer abcdefghijklmnopqrstuvwxyz' https://api.example.test/v1/items`,
			want: `curl -H 'Authorization: Bearer ` + Placeholder + `' https://api.example.test/v1/items`,
		},
		{
			name: "authorization basic header",
			in:   `Authorization: Basic dXNlcjpwYXNzd29yZA==`,
			want: `Authorization: Basic ` + Placeholder,
		},
		{
			name: "authorization header without scheme",
			in:   `proxy-authorization: abcdef123456`,
			want: `proxy-authorization: ` + Placeholder,
		},
		{
			name: "bare bearer value",
			in:   `use Bearer abcdefgh12345678 for the call`,
			want: `use Bearer ` + Placeholder + ` for the call`,
		},
		{
			name: "github token prefix",
			in:   `clone with ghp_abcdefghijklmnopqrstuvwxyz0123456789 today`,
			want: `clone with ` + Placeholder + ` today`,
		},
		{
			name: "slack token prefix",
			in:   `xoxb-123456789012-abcdefghijklmnopqrstuvwx`,
			want: Placeholder,
		},
		{
			name: "openai token prefix",
			in:   `sk-proj-abcdefghijklmnopqrstuvwxyz123456`,
			want: Placeholder,
		},
		{
			name: "aws access key prefix",
			in:   `AKIAIOSFODNN7EXAMPLE`,
			want: Placeholder,
		},
		{
			name: "google api key prefix",
			in:   `AIzaSyA1234567890abcdefghijklmnopqrstuv`,
			want: Placeholder,
		},
		{
			name: "jwt",
			in:   `eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U`,
			want: Placeholder,
		},
		{
			name: "private key block",
			in:   "before\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA7example\n-----END RSA PRIVATE KEY-----\nafter",
			want: "before\n" + Placeholder + "\nafter",
		},
		{
			name: "unterminated private key",
			in:   "before\n-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXk",
			want: "before\n" + Placeholder,
		},
		{
			name: "env assignment",
			in:   `DB_PASSWORD=hunter2 and the rest stays`,
			want: `DB_PASSWORD=` + Placeholder + ` and the rest stays`,
		},
		{
			name: "quoted yaml assignment",
			in:   `password: "correct horse battery staple"`,
			want: `password: "` + Placeholder + `"`,
		},
		{
			name: "json assignment keeps key and quotes",
			in:   `{"path":"main.go","api_key":"abc123def456"}`,
			want: `{"path":"main.go","api_key":"` + Placeholder + `"}`,
		},
		{
			name: "aws secret assignment",
			in:   `aws_secret_access_key: abc123def456ghi789`,
			want: `aws_secret_access_key: ` + Placeholder,
		},
		{
			name: "url query parameter",
			in:   `https://example.test/cb?token=abc123&state=xyz`,
			want: `https://example.test/cb?token=` + Placeholder + `&state=xyz`,
		},
		{
			name: "several secrets and ordinary prose",
			in:   `token=abc123 password=hunter2 then read /tmp/main.go`,
			want: `token=` + Placeholder + ` password=` + Placeholder + ` then read /tmp/main.go`,
		},
		{
			name: "json escaped authorization header",
			in:   `{"command":"curl -H \"Authorization: Bearer abcdefghijklmnop\" https://api.example.test"}`,
			want: `{"command":"curl -H \"Authorization: Bearer ` + Placeholder + `\" https://api.example.test"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Text(tc.in); got != tc.want {
				t.Fatalf("Text() = %q\nwant     %q", got, tc.want)
			}
			if got := Text(Text(tc.in)); got != tc.want {
				t.Fatalf("Text is not idempotent: %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestTextLeavesOrdinaryContentAlone(t *testing.T) {
	corpus := strings.Join([]string{
		"Path /home/runner/work/repo/main.go stays.",
		"Call id call-01a1a867357be42e stays.",
		"Contact dev@example.test and version v1.2.3 stay.",
		`func main() { fmt.Println("hello") }`,
		"key := \"value\"",
		"max_tokens=4096",
		"tokenizer=cl100k_base",
		"password_reset_url=https://example.test/reset",
		"monkey=banana",
		"oauth=enabled",
		"-----BEGIN PUBLIC KEY-----",
	}, "\n")

	if got := Text(corpus); got != corpus {
		t.Fatalf("ordinary content changed:\n got %q\nwant %q", got, corpus)
	}
	if strings.Contains(Text(corpus), Placeholder) {
		t.Fatal("ordinary content was redacted")
	}
}

func TestTextSkipsValuesThatCarryNoSecret(t *testing.T) {
	for _, in := range []string{
		"api_key=null",
		"api_key=",
		"password=false",
		"token_count=5",
		"token=1234",
		"api_key=<redacted>",
		`{"api_key":""}`,
		`{"api_key":"<redacted>"}`,
	} {
		if got := Text(in); strings.Contains(got, Placeholder) && !strings.Contains(in, Placeholder) {
			t.Fatalf("Text(%q) over-redacted to %q", in, got)
		}
	}
}

func TestTextDoesNotRedactAlreadyRedactedContent(t *testing.T) {
	for _, in := range []string{
		"Authorization: " + Placeholder,
		"Authorization: Bearer " + Placeholder,
		"DB_PASSWORD=" + Placeholder,
		`{"api_key":"` + Placeholder + `"}`,
	} {
		if got := Text(in); got != in {
			t.Fatalf("Text(%q) = %q, want unchanged", in, got)
		}
	}
}
