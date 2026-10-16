package adapters_test

import (
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

func TestResolveBinaryPrecedence(t *testing.T) {
	cases := []struct {
		name        string
		binary      string
		paramBinary string
		def         string
		want        string
	}{
		{"plan wins over params and default", "/plan/agent", "/params/agent", "agent", "/plan/agent"},
		{"params fallback when plan empty", "", "/params/agent", "agent", "/params/agent"},
		{"default fallback when both empty", "", "", "agent", "agent"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := adapters.ResolveBinary(tc.binary, tc.paramBinary, tc.def); got != tc.want {
				t.Fatalf("ResolveBinary(%q, %q, %q) = %q, want %q", tc.binary, tc.paramBinary, tc.def, got, tc.want)
			}
		})
	}
}
