package skills_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/russ-p/paseka/internal/skills"
)

func mustReference(t *testing.T, text string) skills.Reference {
	t.Helper()
	ref, err := skills.ParseReference(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return ref
}

// TestParseReferenceAcceptsNamesAndPatterns pins the whole accepted syntax.
func TestParseReferenceAcceptsNamesAndPatterns(t *testing.T) {
	cases := []struct {
		text    string
		pattern bool
		wild    bool
	}{
		{text: "paseka-release"},
		{text: "release2"},
		{text: "paseka-*", pattern: true},
		{text: "*-release", pattern: true},
		{text: "paseka-*-release", pattern: true},
		{text: "*", pattern: true, wild: true},
		{text: "  paseka-release  "}, // surrounding space is not part of the name
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			ref := mustReference(t, tc.text)
			if ref.IsPattern() != tc.pattern {
				t.Fatalf("IsPattern = %v, want %v", ref.IsPattern(), tc.pattern)
			}
			if ref.IsWildcard() != tc.wild {
				t.Fatalf("IsWildcard = %v, want %v", ref.IsWildcard(), tc.wild)
			}
		})
	}
}

// TestParseReferenceRefusesTierPrefixPathsAndForeignWildcards pins that these
// are invalid references, never literal names: with one root there is nothing a
// prefix could select, a reference is not a path, and the only wildcard is `*`.
func TestParseReferenceRefusesTierPrefixPathsAndForeignWildcards(t *testing.T) {
	cases := []struct {
		text       string
		wantReason string
	}{
		{text: "", wantReason: "invalid reference"},
		{text: "   ", wantReason: "invalid reference"},
		{text: "shared:foo", wantReason: "tier prefixes are not supported"},
		{text: "local:foo", wantReason: "tier prefixes are not supported"},
		{text: "./foo", wantReason: "not a path"},
		{text: "../foo", wantReason: "not a path"},
		{text: ".paseka/skills/foo", wantReason: "not a path"},
		{text: `/abs/foo`, wantReason: "not a path"},
		{text: `foo\bar`, wantReason: "not a path"},
		{text: "foo?", wantReason: "the only wildcard is *"},
		{text: "foo[a]", wantReason: "the only wildcard is *"},
		{text: "foo{a,b}", wantReason: "the only wildcard is *"},
		{text: "Foo", wantReason: "lowercase letters"},
		{text: "foo bar", wantReason: "lowercase letters"},
	}
	for _, tc := range cases {
		t.Run(tc.text, func(t *testing.T) {
			_, err := skills.ParseReference(tc.text)
			if err == nil {
				t.Fatalf("%q was accepted", tc.text)
			}
			if !strings.Contains(err.Error(), tc.wantReason) {
				t.Fatalf("err = %v, want %q", err, tc.wantReason)
			}
		})
	}
}

// TestReferenceMatches pins the pattern dialect: `*` is the only wildcard and it
// matches any run of characters, while every other character is literal.
func TestReferenceMatches(t *testing.T) {
	cases := []struct {
		ref  string
		name string
		want bool
	}{
		{ref: "*", name: "paseka-release", want: true},
		{ref: "*", name: "anything", want: true},
		{ref: "paseka-*", name: "paseka-release", want: true},
		{ref: "paseka-*", name: "paseka-", want: true},
		{ref: "paseka-*", name: "release", want: false},
		{ref: "*-release", name: "paseka-release", want: true},
		{ref: "*-release", name: "paseka-release-notes", want: false},
		{ref: "paseka-*-release", name: "paseka-big-release", want: true},
		{ref: "paseka-release", name: "paseka-release", want: true},
		{ref: "paseka-release", name: "paseka-release-notes", want: false},
	}
	for _, tc := range cases {
		if got := mustReference(t, tc.ref).Matches(tc.name); got != tc.want {
			t.Fatalf("%q matches %q = %v, want %v", tc.ref, tc.name, got, tc.want)
		}
	}
}

// TestReportFindValidatesOneReference pins that a reference validates exactly
// the skills it names plus the store-wide findings, and that an unmatched
// pattern is a failure.
func TestReportFindValidatesOneReference(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paseka-release", validFrontmatter, "body\n")
	writeSkill(t, root, "paseka-ship", frontmatter("paseka-ship"), "body\n")
	writeSkill(t, root, "broken", "name: broken\ndescription: \"\"", "body\n")
	report := scanRoot(t, root)

	if issues := report.Find(mustReference(t, "paseka-release")); len(issues) != 0 {
		t.Fatalf("valid name reported: %v", issues)
	}
	if issues := report.Find(mustReference(t, "paseka-*")); len(issues) != 0 {
		t.Fatalf("valid pattern reported: %v", issues)
	}
	issues := report.Find(mustReference(t, "*"))
	if len(issues) != 1 || issues[0].Skill != "broken" {
		t.Fatalf("wildcard issues = %v, want the broken skill", issues)
	}
	issues = report.Find(mustReference(t, "paseka-nope"))
	if len(issues) != 1 || !strings.Contains(issues[0].Reason, "matches no skill") {
		t.Fatalf("unmatched name issues = %v", issues)
	}
	if !strings.Contains(issues[0].Error(), "paseka-nope") {
		t.Fatalf("unmatched issue does not name the reference: %q", issues[0].Error())
	}
	// A store-wide budget finding applies to every reference it is asked about.
	if issues := report.Find(mustReference(t, "paseka-release")); len(issues) != 0 {
		t.Fatalf("store-wide findings leaked into a clean store: %v", issues)
	}
}

// TestReportSelectFailsClosed pins the resolver seam: a non-wildcard reference
// that names nothing is an error, not an empty set, and `*` is allowed to match
// an empty store.
func TestReportSelectFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "paseka-release", validFrontmatter, "body\n")
	writeSkill(t, root, "paseka-ship", frontmatter("paseka-ship"), "body\n")
	report := scanRoot(t, root)

	selected, err := report.Select(mustReference(t, "paseka-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Name != "paseka-release" || selected[1].Name != "paseka-ship" {
		t.Fatalf("selected = %+v", selected)
	}

	if _, err := report.Select(mustReference(t, "paseka-nope")); err == nil {
		t.Fatal("an unmatched pattern must be an error")
	}

	empty := scanRoot(t, filepath.Join(t.TempDir(), "skills"))
	selected, err = empty.Select(mustReference(t, "*"))
	if err != nil || len(selected) != 0 {
		t.Fatalf("wildcard on an empty store = %v, %v", selected, err)
	}
	if _, err := empty.Select(mustReference(t, "paseka-*")); err == nil {
		t.Fatal("an unmatched pattern must be an error on an empty store too")
	}
}
