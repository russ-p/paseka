package export

import (
	"strings"
	"testing"
)

func TestParseInclude(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    []IncludeKind
		wantErr bool
	}{
		{
			name: "empty",
			in:   nil,
			want: nil,
		},
		{
			name: "agent-logs",
			in:   []string{"agent-logs"},
			want: []IncludeKind{IncludeAgentLogs},
		},
		{
			name: "transcript",
			in:   []string{"transcript"},
			want: []IncludeKind{IncludeTranscript},
		},
		{
			name: "reasoning implies transcript",
			in:   []string{"reasoning"},
			want: []IncludeKind{IncludeTranscript, IncludeReasoning},
		},
		{
			name: "transcript and reasoning are equivalent to reasoning alone",
			in:   []string{"transcript,reasoning"},
			want: []IncludeKind{IncludeTranscript, IncludeReasoning},
		},
		{
			name: "reasoning deduped with transcript",
			in:   []string{"reasoning", "reasoning,transcript"},
			want: []IncludeKind{IncludeTranscript, IncludeReasoning},
		},
		{
			name: "csv",
			in:   []string{"usage,durations"},
			want: []IncludeKind{IncludeUsage, IncludeDurations},
		},
		{
			name: "repeatable",
			in:   []string{"bees", "cues", "artifacts"},
			want: []IncludeKind{IncludeBees, IncludeCues, IncludeArtifacts},
		},
		{
			name: "patches",
			in:   []string{"patches"},
			want: []IncludeKind{IncludePatches},
		},
		{
			name: "dedupe",
			in:   []string{"usage", "usage,colony"},
			want: []IncludeKind{IncludeUsage, IncludeColony},
		},
		{
			name:    "invalid",
			in:      []string{"configs"},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseInclude(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), "invalid --include") {
					t.Fatalf("error = %v", err)
				}
				if !strings.Contains(err.Error(), "transcript") {
					t.Fatalf("error does not list transcript: %v", err)
				}
				if !strings.Contains(err.Error(), "reasoning") {
					t.Fatalf("error does not list reasoning: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ParseInclude() = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("ParseInclude()[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestIncludeSetHas(t *testing.T) {
	set, err := ParseInclude([]string{"usage", "bees"})
	if err != nil {
		t.Fatal(err)
	}
	if !set.Has(IncludeUsage) || !set.Has(IncludeBees) {
		t.Fatalf("unexpected set: %+v", set)
	}
	if set.Has(IncludeColony) {
		t.Fatal("colony should not be included")
	}
}

func TestIncludeSetConversationHelpers(t *testing.T) {
	reasoning, err := ParseInclude([]string{"reasoning"})
	if err != nil {
		t.Fatal(err)
	}
	if !reasoning.HasConversation() || !reasoning.HasReasoning() {
		t.Fatalf("reasoning set = %+v, want a conversation with thinking", reasoning)
	}
	transcript, err := ParseInclude([]string{"transcript"})
	if err != nil {
		t.Fatal(err)
	}
	if !transcript.HasConversation() || transcript.HasReasoning() {
		t.Fatalf("transcript set = %+v, want a conversation without thinking", transcript)
	}
}
