package skills

import (
	"fmt"
	"strings"
)

// Reference names one skill or a family of them. It is the one syntax a bee's
// configuration and the shell share, so a command cannot disagree with the
// runtime about what a name refers to.
//
// A reference is a bare name (`paseka-release`), a `*` glob over that name
// (`paseka-*`), or `*` alone for every skill in the store. There is one root and
// therefore one namespace: a `shared:` or `local:` prefix, a path-shaped
// reference, and `?`, `[]`, or `{}` name nothing that can exist and are refused
// as invalid references rather than read as literal names.
type Reference struct {
	text string
}

// ParseReference validates a reference's syntax.
func ParseReference(text string) (Reference, error) {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return Reference{}, fmt.Errorf("invalid reference %q: a reference is a skill name, a * pattern, or *", text)
	}
	for _, r := range raw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '*':
			continue
		case r == ':':
			return Reference{}, fmt.Errorf(
				"invalid reference %q: tier prefixes are not supported; a reference is a bare skill name, a * pattern, or *", text,
			)
		case r == '/' || r == '\\' || r == '.':
			return Reference{}, fmt.Errorf("invalid reference %q: a reference is a skill name, not a path", text)
		case r == '?' || r == '[' || r == ']' || r == '{' || r == '}':
			return Reference{}, fmt.Errorf("invalid reference %q: the only wildcard is *", text)
		default:
			return Reference{}, fmt.Errorf(
				"invalid reference %q: a skill name uses lowercase letters, digits, and hyphens, with * as the only wildcard", text,
			)
		}
	}
	return Reference{text: raw}, nil
}

// String returns the reference as written.
func (r Reference) String() string {
	return r.text
}

// IsPattern reports whether the reference contains a wildcard.
func (r Reference) IsPattern() bool {
	return strings.Contains(r.text, "*")
}

// IsWildcard reports whether the reference is `*` alone, which is allowed to
// match an empty store.
func (r Reference) IsWildcard() bool {
	return r.text == "*"
}

// Matches reports whether the reference names this skill name.
func (r Reference) Matches(name string) bool {
	return matchGlob(r.text, name)
}

// matchGlob implements the pattern dialect: `*` matches any run of characters
// and every other character matches itself. The dialect is deliberately this
// small — no `?`, no character classes, no alternation, no regexp.
func matchGlob(pattern, name string) bool {
	if pattern == "" {
		return name == ""
	}
	if pattern[0] == '*' {
		for i := 0; i <= len(name); i++ {
			if matchGlob(pattern[1:], name[i:]) {
				return true
			}
		}
		return false
	}
	if name == "" || pattern[0] != name[0] {
		return false
	}
	return matchGlob(pattern[1:], name[1:])
}
