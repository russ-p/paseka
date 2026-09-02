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
