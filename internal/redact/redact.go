// Package redact masks obvious secrets in free-form content before it is
// rendered. The transcript path is the only caller today; the helper lives on
// its own so other render boundaries can adopt the same pattern set instead of
// growing a second one.
//
// The pattern set is deliberately small: authorization/bearer headers,
// well-known token prefixes, private-key blocks, and KEY=value assignments
// whose key looks secret. It is best effort, not a security boundary; the
// broader policy (colony-declared patterns, near-misses, redact-vs-drop) is
// tracked in the backlog.
package redact

import (
	"regexp"
	"strconv"
	"strings"
)

// Placeholder replaces a value the helper recognizes as secret. It is stable so
// redaction is idempotent.
const Placeholder = "<redacted>"

var (
	// A complete PEM private-key block wins over every other pattern so the
	// base64 body is never scanned for token prefixes.
	privateKeyBlock = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]*PRIVATE KEY[ A-Z]*-----.*?-----END [A-Z0-9 ]*PRIVATE KEY[ A-Z]*-----`)
	// A block whose END marker was already lost still redacts to end of text.
	privateKeyOpen = regexp.MustCompile(`(?is)-----BEGIN [A-Z0-9 ]*PRIVATE KEY[ A-Z]*-----.*$`)

	// authorizationAt finds the header prefix. The value is parsed by hand so
	// the scheme (Bearer, Basic, …) and the spaces around it stay unchanged.
	authorizationAt = regexp.MustCompile(`(?i)(?:proxy-)?authorization\s*[:=]\s*`)

	// A bare bearer value outside an Authorization header.
	bearerValue = regexp.MustCompile(`(?i)\b(bearer)(\s+)([A-Za-z0-9._~+/=-]{8,})`)

	// Well-known token prefixes. Each needs enough trailing characters that
	// ordinary prose ("sk-learn", "monkey") cannot match by accident.
	wellKnownToken = regexp.MustCompile(`\b(?:` +
		`gh[pousr]_[A-Za-z0-9]{20,}` +
		`|github_pat_[A-Za-z0-9_]{20,}` +
		`|glpat-[A-Za-z0-9_-]{20,}` +
		`|xox[abposr]-[A-Za-z0-9-]{10,}` +
		`|xapp-[A-Za-z0-9-]{10,}` +
		`|(?:sk|rk)_(?:live|test)_[A-Za-z0-9]{10,}` +
		`|sk-(?:proj-|ant-)?[A-Za-z0-9_-]{16,}` +
		`|(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA|ANVA|AIPA)[0-9A-Z]{16}` +
		`|AIza[0-9A-Za-z_-]{35}` +
		`|ya29\.[0-9A-Za-z_-]{20,}` +
		`|npm_[A-Za-z0-9]{30,}` +
		`|pypi-[A-Za-z0-9_-]{20,}` +
		`|SG\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}` +
		`|eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}` +
		`)\b`)

	// assignment finds "KEY=value" and "KEY: value" shapes. The key is
	// inspected afterwards, so an ordinary "path: main.go" is left alone.
	assignment = regexp.MustCompile(`(?i)(["']?)([A-Za-z_][A-Za-z0-9_.-]{0,64})(["']?)([ \t]*[=:][ \t]*)`)
)

// secretKeySuffixes are the key endings that make an assignment value secret.
// They match the end of the key, so "max_tokens" and "tokenizer" stay ordinary
// while "GITHUB_TOKEN", "db_password", and "OPENAI_API_KEY" do not.
var secretKeySuffixes = []string{
	"secret",
	"clientsecret", "client_secret", "client-secret",
	"token",
	"password", "passwd", "pwd", "passphrase",
	"credential",
	"apikey", "api_key", "api-key",
	"accesskey", "access_key", "access-key",
	"privatekey", "private_key", "private-key",
	"secretkey", "secret_key", "secret-key",
	"signingkey", "signing_key", "signing-key",
	"encryptionkey", "encryption_key", "encryption-key",
	"masterkey", "master_key", "master-key",
	"sessionkey", "session_key", "session-key",
	"access_key_id", "access-key-id",
	"basic_auth", "proxy_auth",
	"cookie",
}

// authSchemes are the header schemes whose prefix is preserved.
var authSchemes = []string{"bearer", "basic", "token", "digest", "bot"}

// nonSecretValues are assignment values that carry no secret of their own.
var nonSecretValues = map[string]bool{
	"null": true, "nil": true, "true": true, "false": true,
	"none": true, "anonymous": true, "undefined": true, "-": true,
}

// Text returns s with each recognized secret replaced by Placeholder. Text is
// idempotent: applying it to its own output changes nothing. Non-secret content
// around a secret is left byte for byte.
func Text(s string) string {
	if s == "" {
		return s
	}
	s = privateKeyBlock.ReplaceAllString(s, Placeholder)
	s = privateKeyOpen.ReplaceAllString(s, Placeholder)
	s = redactAuthHeaders(s)
	s = bearerValue.ReplaceAllString(s, "${1}${2}"+Placeholder)
	s = wellKnownToken.ReplaceAllString(s, Placeholder)
	return redactAssignments(s)
}

// redactAuthHeaders replaces only the credential in an Authorization header,
// keeping the header name, separator, and scheme. Parsing the value by hand
// keeps a second pass from mistaking the scheme for a credential.
func redactAuthHeaders(s string) string {
	matches := authorizationAt.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		if m[0] < last {
			continue
		}
		start := m[1]
		end := scanAuthWord(s, start)
		if end == start {
			continue
		}
		secretStart := start
		// A scheme only prefixes a credential when whitespace follows it;
		// otherwise the whole word is the value.
		if isAuthScheme(s[start:end]) {
			if next := skipSpaces(s, end); next > end {
				end = scanAuthWord(s, next)
				if end == next {
					continue
				}
				secretStart = next
			}
		}
		value := s[secretStart:end]
		if value == Placeholder || len(value) < 4 || nonSecretValues[strings.ToLower(value)] {
			continue
		}
		b.WriteString(s[last:secretStart])
		b.WriteString(Placeholder)
		last = end
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// scanAuthWord returns the end of the credential-looking word at i. Quotes and
// structural punctuation end it, and so does a backslash so a JSON-escaped
// quote source stays valid.
func scanAuthWord(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\r', '\n', ',', ';', '"', '\'', '}', ']', ')', '&', '#', '\\', '<', '>':
			return i
		}
		i++
	}
	return i
}

func skipSpaces(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return i
}

func isAuthScheme(word string) bool {
	for _, scheme := range authSchemes {
		if strings.EqualFold(word, scheme) {
			return true
		}
	}
	return false
}

// redactAssignments walks KEY=value shapes, keeps the key and separator, and
// replaces only a value whose key looks secret.
func redactAssignments(s string) string {
	matches := assignment.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		if m[0] < last || !looksSecretKey(s[m[4]:m[5]]) {
			continue
		}
		valueStart, valueEnd := assignmentValue(s, m[1])
		if valueEnd <= valueStart {
			continue
		}
		value := strings.TrimSpace(s[valueStart:valueEnd])
		if !worthRedacting(value) {
			continue
		}
		b.WriteString(s[last:valueStart])
		b.WriteString(Placeholder)
		last = valueEnd
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// assignmentValue returns the value span that starts at i. A quoted value keeps
// its quotes; the returned span covers only the content between them. An
// unterminated quote extends to the end of the text rather than leaking.
func assignmentValue(s string, i int) (int, int) {
	if i >= len(s) {
		return i, i
	}
	if s[i] == '"' || s[i] == '\'' {
		quote := s[i]
		start := i + 1
		j := start
		for j < len(s) {
			if s[j] == '\\' && j+1 < len(s) {
				j += 2
				continue
			}
			if s[j] == quote {
				break
			}
			j++
		}
		return start, j
	}
	j := i
	for j < len(s) && !isValueTerminator(s[j]) {
		j++
	}
	return i, j
}

func isValueTerminator(c byte) bool {
	switch c {
	case ' ', '\t', '\r', '\n', ',', ';', '"', '\'', '}', ']', ')', '&', '#', '\\':
		return true
	}
	return false
}

// looksSecretKey reports whether an assignment key names a secret. Matching the
// end of the key keeps "tokenizer", "max_tokens", and "password_reset_url"
// ordinary.
func looksSecretKey(raw string) bool {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return false
	}
	for _, suffix := range secretKeySuffixes {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

// worthRedacting rejects values that cannot be a credential: the placeholder
// itself, booleans, nulls, and bare numbers.
func worthRedacting(value string) bool {
	if value == "" || value == Placeholder {
		return false
	}
	if nonSecretValues[strings.ToLower(value)] {
		return false
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return false
	}
	return true
}
