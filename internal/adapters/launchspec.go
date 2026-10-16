package adapters

// LaunchSpec describes the launch facts an adapter owns, so a resolver can fill
// them into a launch plan without the adapter reading machine-local config or the
// ambient environment itself.
//
// DefaultBinary is the adapter's built-in executable, the last fallback after a
// bee or machine file override. CredentialEnv is the child environment variable
// that carries the resolved credential; it is empty when the adapter passes the
// key another way (for example on argv) or needs no credential at all.
type LaunchSpec struct {
	DefaultBinary string
	CredentialEnv string
}

// ResolveBinary picks the executable for a launch. The resolver-provided
// Binary is authoritative when set; a params override and the adapter's
// built-in default remain as fallbacks so direct adapter callers that predate
// the launch plan keep working.
func ResolveBinary(binary, paramBinary, defaultBinary string) string {
	if binary != "" {
		return binary
	}
	if paramBinary != "" {
		return paramBinary
	}
	return defaultBinary
}

// ResolveDir picks the working directory for a child process. A non-empty dir is
// authoritative; an empty dir falls back to the workspace. Callers that must
// inherit the parent's cwd pass an empty dir and an empty workspace.
func ResolveDir(dir, workspace string) string {
	if dir == "" {
		return workspace
	}
	return dir
}
