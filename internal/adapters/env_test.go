package adapters_test

import (
	"reflect"
	"testing"

	"github.com/russ-p/paseka/internal/adapters"
)

func TestEnvMapLastWins(t *testing.T) {
	got := adapters.EnvMap([]string{"A=1", "B=2", "A=3", "MALFORMED"})
	want := map[string]string{"A": "3", "B": "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("EnvMap = %v, want %v", got, want)
	}
}

func TestRedactEnvMasksNamedKeys(t *testing.T) {
	env := []string{"PATH=/bin", "CURSOR_API_KEY=secret", "GITHUB_TOKEN=ghp_x"}
	got := adapters.RedactEnv(env, []string{"CURSOR_API_KEY", "GITHUB_TOKEN"})
	want := []string{"PATH=/bin", "CURSOR_API_KEY=<redacted>", "GITHUB_TOKEN=<redacted>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RedactEnv = %v, want %v", got, want)
	}
}

func TestRedactEnvKeepsUnlistedValues(t *testing.T) {
	env := []string{"PATH=/bin", "UNLISTED=visible"}
	got := adapters.RedactEnv(env, []string{"CURSOR_API_KEY"})
	want := []string{"PATH=/bin", "UNLISTED=visible"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RedactEnv = %v, want %v", got, want)
	}
}
