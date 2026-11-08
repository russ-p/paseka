package runs

import (
	"testing"
	"time"
)

func TestOpenFailuresOfResolvesWithinATask(t *testing.T) {
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	failed := func(task, agent string, minutes int) trailRunOutcome {
		return trailRunOutcome{TaskID: task, AgentID: agent, At: at(minutes), Failed: true}
	}
	succeeded := func(task, agent string, minutes int) trailRunOutcome {
		return trailRunOutcome{TaskID: task, AgentID: agent, At: at(minutes), Succeeded: true}
	}

	tests := []struct {
		name       string
		runs       []trailRunOutcome
		wantOpen   int
		wantNewest time.Time
	}{
		{
			name: "a trail with no terminal outcome is quiet",
		},
		{
			name:     "failure then completion of the same task resolves",
			runs:     []trailRunOutcome{failed("task-a", "agent-1", 1), succeeded("task-a", "agent-2", 2)},
			wantOpen: 0,
		},
		{
			name:       "completion then failure is open",
			runs:       []trailRunOutcome{succeeded("task-a", "agent-1", 1), failed("task-a", "agent-2", 2)},
			wantOpen:   1,
			wantNewest: at(2),
		},
		{
			name: "two failures on one task followed by one success resolve both",
			runs: []trailRunOutcome{
				failed("task-a", "agent-1", 1),
				failed("task-a", "agent-2", 2),
				succeeded("task-a", "agent-3", 3),
			},
			wantOpen: 0,
		},
		{
			name:       "retries within one task collapse into one open failure",
			runs:       []trailRunOutcome{failed("task-a", "agent-1", 1), failed("task-a", "agent-2", 3)},
			wantOpen:   1,
			wantNewest: at(3),
		},
		{
			name:       "a success on another task clears nothing",
			runs:       []trailRunOutcome{failed("task-a", "agent-1", 1), succeeded("task-b", "agent-2", 2)},
			wantOpen:   1,
			wantNewest: at(1),
		},
		{
			name:       "a failure on another task is counted separately",
			runs:       []trailRunOutcome{failed("task-a", "agent-1", 1), failed("task-b", "agent-2", 2)},
			wantOpen:   2,
			wantNewest: at(2),
		},
		{
			name:       "the empty task id forms one trail-level group",
			runs:       []trailRunOutcome{failed("", "agent-1", 1), succeeded("", "agent-2", 2)},
			wantOpen:   0,
			wantNewest: time.Time{},
		},
		{
			name:       "a closing task success does not clear a work failure",
			runs:       []trailRunOutcome{failed("task-a", "agent-1", 1), succeeded("closing-1", "agent-2", 2)},
			wantOpen:   1,
			wantNewest: at(1),
		},
		{
			name:       "a completion-instant tie is broken by agent id: the later id wins",
			runs:       []trailRunOutcome{succeeded("task-a", "agent-a", 1), failed("task-a", "agent-b", 1)},
			wantOpen:   1,
			wantNewest: at(1),
		},
		{
			name:       "a completion-instant tie is broken by agent id: the success can win",
			runs:       []trailRunOutcome{failed("task-a", "agent-a", 1), succeeded("task-a", "agent-b", 1)},
			wantOpen:   0,
			wantNewest: time.Time{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			open, newest := openFailuresOf(tc.runs)
			if open != tc.wantOpen {
				t.Fatalf("open = %d, want %d", open, tc.wantOpen)
			}
			if tc.wantNewest.IsZero() {
				if newest != nil {
					t.Fatalf("newest = %s, want absent", newest)
				}
				return
			}
			if newest == nil || !newest.Equal(tc.wantNewest) {
				t.Fatalf("newest = %v, want %s", newest, tc.wantNewest)
			}
		})
	}
}

// The projection must not depend on directory read order: the walk hands the
// outcomes over in whatever order the entries arrived, and the answer is still
// one total order.
func TestOpenFailuresOfIsIndependentOfInputOrder(t *testing.T) {
	base := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	at := func(minutes int) time.Time { return base.Add(time.Duration(minutes) * time.Minute) }
	runs := []trailRunOutcome{
		{TaskID: "task-a", AgentID: "agent-1", At: at(3), Failed: true},
		{TaskID: "task-b", AgentID: "agent-2", At: at(4), Succeeded: true},
		{TaskID: "task-b", AgentID: "agent-3", At: at(1), Failed: true},
		{TaskID: "task-a", AgentID: "agent-4", At: at(5), Succeeded: true},
		{TaskID: "task-c", AgentID: "agent-5", At: at(2), Failed: true},
	}
	wantOpen, wantNewest := openFailuresOf(runs)
	if wantOpen != 1 || wantNewest == nil || !wantNewest.Equal(at(2)) {
		t.Fatalf("baseline = %d/%v, want task-c open at %s", wantOpen, wantNewest, at(2))
	}

	for _, order := range permutations(len(runs)) {
		shuffled := make([]trailRunOutcome, len(runs))
		for i, index := range order {
			shuffled[i] = runs[index]
		}
		open, newest := openFailuresOf(shuffled)
		if open != wantOpen {
			t.Fatalf("order %v: open = %d, want %d", order, open, wantOpen)
		}
		if (newest == nil) != (wantNewest == nil) || (newest != nil && !newest.Equal(*wantNewest)) {
			t.Fatalf("order %v: newest = %v, want %s", order, newest, *wantNewest)
		}
	}
}

// permutations returns every ordering of n items, so a shuffle test covers all
// of them rather than a handful that could all miss the same ordering bug.
func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for _, tail := range permutations(n - 1) {
		for pos := 0; pos <= len(tail); pos++ {
			order := make([]int, 0, n)
			order = append(order, tail[:pos]...)
			order = append(order, n-1)
			order = append(order, tail[pos:]...)
			out = append(out, order)
		}
	}
	return out
}
