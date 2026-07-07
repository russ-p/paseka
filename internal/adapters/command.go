package adapters

import "strings"

// ResolveExec picks a custom command argv or falls back to the adapter builder.
func ResolveExec(argv []string, build func() (binary string, args []string)) (binary string, args []string) {
	if len(argv) > 0 {
		if len(argv) == 1 {
			return argv[0], nil
		}
		return argv[0], argv[1:]
	}
	return build()
}

// FlagValue scans argv for a flag value (supports --flag value and --flag=value).
func FlagValue(argv []string, flag string) string {
	prefix := flag + "="
	for i, arg := range argv {
		if arg == flag && i+1 < len(argv) {
			return argv[i+1]
		}
		if strings.HasPrefix(arg, prefix) {
			return arg[len(prefix):]
		}
	}
	return ""
}
