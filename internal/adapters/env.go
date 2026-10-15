package adapters

import "strings"

// EnvMap indexes an environment slice by variable name. Later entries win, so
// the returned map reflects the effective value of a duplicated key.
func EnvMap(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, entry := range env {
		if k, v, ok := strings.Cut(entry, "="); ok {
			out[k] = v
		}
	}
	return out
}

// RedactEnv renders an environment slice for logging: the named keys become
// NAME=<redacted>, every other entry is kept as-is. Order is preserved and keys
// absent from env are ignored, so the output mirrors what the child receives
// minus the secret values.
func RedactEnv(env []string, redact []string) []string {
	if len(env) == 0 {
		return nil
	}
	redacted := make(map[string]bool, len(redact))
	for _, key := range redact {
		redacted[key] = true
	}
	out := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && redacted[key] {
			out = append(out, key+"=<redacted>")
			continue
		}
		out = append(out, entry)
	}
	return out
}
