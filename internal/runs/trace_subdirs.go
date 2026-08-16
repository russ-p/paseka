package runs

// ReservedTraceSubdirs are non-agent directories under .paseka/runs/<traceId>/.
// They must not be treated as bee run directories.
var ReservedTraceSubdirs = map[string]struct{}{
	"tasks":     {},
	"artifacts": {},
	"console":   {},
}

// skipTraceEventSubdirs have no per-agent events.ndjson to merge.
// "console" is reserved as a run dir but still holds human-producer audit events.
var skipTraceEventSubdirs = map[string]struct{}{
	"tasks":     {},
	"artifacts": {},
}

// IsReservedTraceSubdir reports whether name is a reserved trace subdirectory.
func IsReservedTraceSubdir(name string) bool {
	_, ok := ReservedTraceSubdirs[name]
	return ok
}

func skipTraceEventDir(name string) bool {
	_, ok := skipTraceEventSubdirs[name]
	return ok
}
