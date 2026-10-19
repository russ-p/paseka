package main

import (
	"strings"
	"testing"
)

func TestResolveExportTraceID(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		flag      string
		want      string
		wantError string
	}{
		{name: "positional", args: []string{"trace-1"}, want: "trace-1"},
		{name: "flag", flag: "trace-1", want: "trace-1"},
		{name: "both same", args: []string{"trace-1"}, flag: "trace-1", want: "trace-1"},
		{name: "positional wins over empty flag", args: []string{" trace-1 "}, want: "trace-1"},
		{name: "missing", wantError: "paseka export <traceId>"},
		{name: "conflict", args: []string{"trace-1"}, flag: "trace-2", wantError: "conflicting flight trail ids"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveExportTraceID(tt.args, tt.flag)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("trace id = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExportCmdAcceptsPositionalTraceID(t *testing.T) {
	cmd := newExportCmd()
	if cmd.Args == nil {
		t.Fatal("export command has no Args validator")
	}
	if cmd.Flags().Lookup("trace").Annotations != nil {
		t.Fatal("--trace should not be marked required; the positional form is accepted")
	}
	if err := cmd.Args(cmd, []string{"trace-1", "trace-2"}); err == nil {
		t.Fatal("expected error for more than one positional trace id")
	}
	if err := cmd.Args(cmd, []string{"trace-1"}); err != nil {
		t.Fatalf("positional trace id rejected: %v", err)
	}
}
