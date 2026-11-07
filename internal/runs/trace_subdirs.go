package runs

// proposalsDir is the reserved subdirectory holding proposal patches. It is duplicated as
// a string rather than imported from internal/patches because internal/patches depends on
// this package for path layout, and a run-listing rule must not be a dependency cycle.
const proposalsDir = "proposals"

// SkillProposalsDirName is the runtime-owned directory holding the decisions about a
// trail's skill proposals. It is duplicated as a string rather than imported from
// internal/skills because internal/skills reads trails through this package, and a
// run-listing rule must not be a dependency cycle. It shares its name with the
// artifact comb's bundle directory; the two live under different roots.
const SkillProposalsDirName = "skill-proposals"

// ReservedTraceSubdirs are non-agent directories under .paseka/runs/<traceId>/.
// They must not be treated as bee run directories.
var ReservedTraceSubdirs = map[string]struct{}{
	"tasks":     {},
	"artifacts": {},
	"console":   {},
	// Trail configuration snapshots and attempt ownership are runtime-owned.
	"config": {},
	// Proposal patches: captured code history the runtime owns, not a bee handoff.
	proposalsDir: {},
	// Fork requests are durable admission records, not agent runs.
	"fork-requests": {},
	// Cue invocations are the trail's retained ingress identities.
	CueInvocationsDirName: {},
	// Analysis admissions are durable reaction identities, not agent runs.
	AnalysisDirName: {},
	// Skill proposal decisions are the runtime's record of what a Beekeeper
	// decided, not a run output.
	SkillProposalsDirName: {},
	// Immutable work-completion checkpoints, result evidence, and metrics
	// snapshots are trail records, not bee runs.
	WorkCompletionDirName: {},
}

// skipTraceEventSubdirs have no per-agent events.ndjson to merge.
// "console" is reserved as a run dir but still holds human-producer audit events.
// "proposals" is reserved and skipped: its sidecars retain a synthesized event envelope,
// but that envelope is the archive's own context and is never merged into event feeds.
var skipTraceEventSubdirs = map[string]struct{}{
	"tasks":               {},
	"artifacts":           {},
	"config":              {},
	proposalsDir:          {},
	"fork-requests":       {},
	CueInvocationsDirName: {},
	AnalysisDirName:       {},
	SkillProposalsDirName: {},
	WorkCompletionDirName: {},
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
