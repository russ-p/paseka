package protocol

import (
	"testing"
)

func TestNormalizeCodeProposalKind(t *testing.T) {
	tests := []struct {
		in   MutationKind
		want MutationKind
	}{
		{MutationCodeProposal, MutationCodeProposalIsolated},
		{MutationCodeProposalIsolated, MutationCodeProposalIsolated},
		{MutationCodeProposalRoot, MutationCodeProposalRoot},
	}
	for _, tc := range tests {
		if got := NormalizeCodeProposalKind(tc.in); got != tc.want {
			t.Fatalf("NormalizeCodeProposalKind(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCodeProposalKindsMatchMatrix(t *testing.T) {
	type pair struct {
		sub, ev string
		want    bool
	}
	tests := []pair{
		{"code.proposal", "code.proposal", true},
		{"code.proposal", "code.proposal.isolated", true},
		{"code.proposal.isolated", "code.proposal", true},
		{"code.proposal.isolated", "code.proposal.isolated", true},
		{"code.proposal.root", "code.proposal.root", true},
		{"code.proposal.root", "code.proposal", false},
		{"code.proposal.root", "code.proposal.isolated", false},
		{"code.proposal", "code.proposal.root", false},
		{"code.proposal.isolated", "code.proposal.root", false},
	}
	for _, tc := range tests {
		got := CodeProposalKindsMatch(tc.sub, tc.ev)
		if got != tc.want {
			t.Fatalf("CodeProposalKindsMatch(%q, %q) = %v, want %v", tc.sub, tc.ev, got, tc.want)
		}
	}
}

func TestValidateCodeProposalKinds(t *testing.T) {
	kinds := []string{
		string(MutationCodeProposal),
		string(MutationCodeProposalIsolated),
		string(MutationCodeProposalRoot),
	}
	for _, kind := range kinds {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"` + kind + `","diff":"diff --git"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatalf("kind %q: %v", kind, err)
		}
		if details := in.Validate(); len(details) != 0 {
			t.Fatalf("kind %q: details = %#v", kind, details)
		}
	}
}

func TestValidateCodeProposalProvenance(t *testing.T) {
	t.Run("isolated with provenance", func(t *testing.T) {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"code.proposal.isolated","diff":"d","workspace":"isolated","baseSha":"abc123def456","worktreePath":".paseka/worktrees/trace-1","sector":"backend"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		if details := in.Validate(); len(details) != 0 {
			t.Fatalf("details = %#v", details)
		}
	})

	t.Run("root with provenance", func(t *testing.T) {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"code.proposal.root","summary":"cfg","workspace":"root","baseSha":"abc123def456"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		if details := in.Validate(); len(details) != 0 {
			t.Fatalf("details = %#v", details)
		}
	})

	t.Run("workspace mismatch isolated", func(t *testing.T) {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"code.proposal.isolated","diff":"d","workspace":"root"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		details := in.Validate()
		if len(details) != 1 || details[0].Path != "payload.workspace" {
			t.Fatalf("details = %#v", details)
		}
	})

	t.Run("worktreePath on root rejected", func(t *testing.T) {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"code.proposal.root","diff":"d","worktreePath":".paseka/worktrees/trace-1"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		details := in.Validate()
		if len(details) != 1 || details[0].Path != "payload.worktreePath" {
			t.Fatalf("details = %#v", details)
		}
	})

	t.Run("invalid workspace value", func(t *testing.T) {
		raw := []byte(`{"traceId":"trace-1","type":"MUTATION","payload":{"kind":"code.proposal.isolated","diff":"d","workspace":"main"}}`)
		in, err := ParseEventInput(raw)
		if err != nil {
			t.Fatal(err)
		}
		details := in.Validate()
		if len(details) != 1 || details[0].Path != "payload.workspace" {
			t.Fatalf("details = %#v", details)
		}
	})
}
