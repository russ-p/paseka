package export

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/paseka/paseka/internal/adapters"
	"github.com/paseka/paseka/internal/hiveview"
)

func TestResolveAgentLogTruncatesArgsAndCapsRows(t *testing.T) {
	long := strings.Repeat("a", maxAgentLogArgsRunes+20)
	calls := make([]adapters.ToolCall, maxAgentLogToolCalls+3)
	for i := range calls {
		calls[i] = adapters.ToolCall{Name: "Read", Args: long, CallID: "c"}
	}
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{log: adapters.SessionLog{ToolCalls: calls}}
	})
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if !got.Truncated {
		t.Fatal("expected Truncated")
	}
	if len(got.ToolCalls) != maxAgentLogToolCalls {
		t.Fatalf("len = %d, want %d", len(got.ToolCalls), maxAgentLogToolCalls)
	}
	if got.ToolCalls[0].Args != strings.Repeat("a", maxAgentLogArgsRunes)+"…" {
		t.Fatalf("args not truncated: %q", got.ToolCalls[0].Args)
	}
}

func TestResolveAgentLogEmptyToolCallsFound(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{log: adapters.SessionLog{ToolCalls: []adapters.ToolCall{}}}
	})
	if got.Omitted != "" {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
	if got.ToolCalls == nil {
		t.Fatal("expected empty slice, not nil omit")
	}
}

func TestResolveAgentLogEmptyToolCallsWithOmitted(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{log: adapters.SessionLog{Omitted: adapters.SessionLogOmittedNotImplemented}}
	})
	if got.Omitted != adapters.SessionLogOmittedNotImplemented {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestResolveAgentLogError(t *testing.T) {
	got := resolveAgentLog(hiveview.RunView{
		AgentID:           "agent-1",
		Adapter:           "cursor",
		ProviderSessionID: "sess",
	}, func(string) adapters.SessionLogResolver {
		return fakeSessionLogResolver{err: errors.New("boom")}
	})
	if got.Omitted != adapters.SessionLogOmittedResolveError {
		t.Fatalf("Omitted = %q", got.Omitted)
	}
}

func TestDefaultSessionLogLookup(t *testing.T) {
	if defaultSessionLogLookup("cursor") == nil || defaultSessionLogLookup("pi") == nil {
		t.Fatal("cursor and pi must resolve")
	}
	if defaultSessionLogLookup("claude") != nil || defaultSessionLogLookup("script") != nil {
		t.Fatal("claude and script must be unsupported")
	}
	got, err := defaultSessionLogLookup("cursor").ResolveSessionLog(context.Background(), adapters.SessionLogRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Omitted != adapters.SessionLogOmittedStoreNotFound {
		t.Fatalf("cursor missing id Omitted = %q", got.Omitted)
	}
	piLog, err := defaultSessionLogLookup("pi").ResolveSessionLog(context.Background(), adapters.SessionLogRequest{ProviderSessionID: "agent-1"})
	if err != nil {
		t.Fatal(err)
	}
	if piLog.Omitted != adapters.SessionLogOmittedNotImplemented {
		t.Fatalf("pi stub Omitted = %q", piLog.Omitted)
	}
}
