package colony_test

import (
	"testing"

	"github.com/paseka/paseka/internal/colony"
)

func TestComputeSlugFromRemote(t *testing.T) {
	got := colony.ComputeSlug("/tmp/x", "https://github.com/acme/api.git")
	if got != "acme-api" {
		t.Fatalf("got %q, want acme-api", got)
	}
}

func TestComputeSlugFromDir(t *testing.T) {
	got := colony.ComputeSlug("/home/dev/paseka", "")
	if got != "paseka" {
		t.Fatalf("got %q", got)
	}
}
