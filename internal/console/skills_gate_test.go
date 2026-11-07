package console

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/colony"
)

// TestSkillDecisionGateRefusesASecondDecisionWhileOneIsInFlight pins the busy
// guard at the handler: the second click is answered now, not queued behind the
// first decision that may change the state it was computed from.
func TestSkillDecisionGateRefusesASecondDecisionWhileOneIsInFlight(t *testing.T) {
	a := &api{ctx: colony.Context{ColonyRoot: t.TempDir()}}
	path := "/api/skills/proposals/trace-a/paseka-release/approve"
	if !a.skillMutations.tryBegin() {
		t.Fatal("the first decision must take the gate")
	}

	rec := httptest.NewRecorder()
	a.handleSkillProposal(rec, httptest.NewRequest(http.MethodPost, path, nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("second decision status = %d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "in flight") {
		t.Fatalf("busy body = %q", rec.Body.String())
	}

	a.skillMutations.end()

	// Once the first decision has finished the gate is free again; the next
	// request reaches the domain rather than being told the console is busy.
	rec = httptest.NewRecorder()
	a.handleSkillProposal(rec, httptest.NewRequest(http.MethodPost, path, nil))
	if rec.Code == http.StatusConflict {
		t.Fatal("a finished decision must not leave the console busy")
	}
}
