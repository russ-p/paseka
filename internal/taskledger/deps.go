package taskledger

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/russ-p/paseka/internal/protocol"
)

// ValidateDependencies reports dependsOn entries that can never be satisfied: an
// id no task in the trace carries, a task depending on itself, an empty entry, or
// a cycle. specs are tasks about to be planned onto trace, so one plan may depend
// on a sibling registered by the same event. The reducer stays tolerant; this is
// the producer-side check that keeps a typo from parking a trail for good.
func ValidateDependencies(trace TraceSnapshot, specs ...protocol.TaskSpec) error {
	deps := make(map[string][]string, len(trace.Tasks)+len(specs))
	for id, task := range trace.Tasks {
		deps[id] = task.DependsOn
	}
	for _, spec := range specs {
		if spec.TaskID == "" {
			continue
		}
		// The reducer ignores a plan for an id that already has a status, so the
		// stored dependencies are the ones that count, not the spec's.
		if _, exists := trace.Tasks[spec.TaskID]; exists {
			continue
		}
		deps[spec.TaskID] = spec.DependsOn
	}

	var problems []string
	for id, list := range deps {
		for _, dep := range list {
			switch dep {
			case "":
				problems = append(problems, fmt.Sprintf("task %q has an empty dependency", id))
			case id:
				problems = append(problems, fmt.Sprintf("task %q depends on itself", id))
			default:
				if _, ok := deps[dep]; !ok {
					problems = append(problems, fmt.Sprintf("task %q depends on unknown task %q", id, dep))
				}
			}
		}
	}
	if cycle := dependencyCycle(deps); len(cycle) > 0 {
		quoted := make([]string, len(cycle))
		for i, id := range cycle {
			quoted[i] = fmt.Sprintf("%q", id)
		}
		problems = append(problems, fmt.Sprintf("tasks %s form a dependency cycle", strings.Join(quoted, ", ")))
	}
	if len(problems) == 0 {
		return nil
	}

	sort.Strings(problems)
	errs := make([]error, len(problems))
	for i, problem := range problems {
		errs[i] = errors.New(problem)
	}
	return errors.Join(errs...)
}

// dependencyCycle returns the ids on one dependency cycle, or nil when the graph
// is acyclic. Self-dependencies are skipped: they are reported on their own and
// would otherwise hide every other cycle behind them.
func dependencyCycle(deps map[string][]string) []string {
	const (
		white = 0
		gray  = 1
		black = 2
	)

	color := make(map[string]int, len(deps))
	var stack []string
	var cycle []string

	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = gray
		stack = append(stack, id)
		// Sort neighbours so a graph with several cycles reports the same one on
		// every run; a doctor warning that moves is a warning nobody trusts.
		neighbors := append([]string(nil), deps[id]...)
		sort.Strings(neighbors)
		for _, dep := range neighbors {
			if dep == id {
				continue
			}
			if _, known := deps[dep]; !known {
				continue
			}
			switch color[dep] {
			case white:
				if visit(dep) {
					return true
				}
			case gray:
				for i, id := range stack {
					if id == dep {
						cycle = append([]string(nil), stack[i:]...)
						return true
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}

	ids := make([]string, 0, len(deps))
	for id := range deps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if color[id] == white && visit(id) {
			break
		}
	}
	return cycle
}
