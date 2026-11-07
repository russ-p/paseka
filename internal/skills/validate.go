package skills

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// document is the frontmatter of one `SKILL.md`. The fields are the standard's
// required pair plus its optional string-to-string metadata map; any other
// frontmatter key is ignored so that a skill authored for another client stays
// readable here unchanged.
type document struct {
	Name        string               `yaml:"name"`
	Description string               `yaml:"description"`
	Metadata    map[string]yaml.Node `yaml:"metadata"`
}

// plainDocument is a document whose metadata has been checked and flattened to
// the string-to-string map the standard requires.
type plainDocument struct {
	name        string
	description string
	metadata    map[string]string
}

// parseDocument splits a `SKILL.md` into frontmatter and body and decodes the
// frontmatter. A non-empty reason is the validation finding.
func parseDocument(data []byte) (plainDocument, string, string) {
	front, body, ok := splitFrontmatter(string(data))
	if !ok {
		return plainDocument{}, body, "SKILL.md must begin with a --- frontmatter block closed by ---"
	}
	var doc document
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return plainDocument{}, body, fmt.Sprintf("invalid frontmatter: %v", err)
	}
	plain := plainDocument{name: doc.Name, description: doc.Description}
	if len(doc.Metadata) > 0 {
		// The standard's metadata is string to string. yaml would happily turn
		// `revision: 3` into "3", so the type is checked rather than coerced: a
		// provenance record must not be a number one day and a string the next.
		plain.metadata = make(map[string]string, len(doc.Metadata))
		for key, node := range doc.Metadata {
			if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
				return plainDocument{}, body, fmt.Sprintf(
					"invalid frontmatter: metadata.%s must be a string", key,
				)
			}
			plain.metadata[key] = node.Value
		}
	}
	return plain, body, ""
}

// splitFrontmatter returns the YAML block and the Markdown body. The opening
// fence must be the first line and the closing fence must be a line of its own,
// which is what every client that reads this format implements.
func splitFrontmatter(text string) (front, body string, ok bool) {
	text = strings.TrimPrefix(text, "\ufeff") // an editor's byte-order mark must not disable a skill
	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return "", "", false
	}
	rest := text[strings.IndexByte(text, '\n')+1:]
	offset := 0
	for {
		end := strings.IndexByte(rest[offset:], '\n')
		line := rest[offset:]
		if end >= 0 {
			line = rest[offset : offset+end]
		}
		if isFence(line) {
			bodyStart := len(rest)
			if end >= 0 {
				bodyStart = offset + end + 1
			}
			return rest[:offset], rest[bodyStart:], true
		}
		if end < 0 {
			return "", "", false
		}
		offset += end + 1
	}
}

func isFence(line string) bool {
	return strings.TrimSpace(strings.TrimSuffix(line, "\r")) == "---"
}

// validateDocument is the one rule set: a skill is valid when its name is
// present, has the standard's shape, equals the directory that holds it, and its
// description is present and within the standard's bound.
func validateDocument(dirName string, doc plainDocument) []Issue {
	var issues []Issue
	name := strings.TrimSpace(doc.name)
	if name == "" {
		return []Issue{{Skill: dirName, Reason: "name is required"}}
	}
	if n := utf8.RuneCountInString(name); n > MaxNameLength {
		issues = append(issues, Issue{Skill: dirName, Reason: fmt.Sprintf(
			"name %q is %d characters; the limit is %d", name, n, MaxNameLength,
		)})
	}
	if reason := nameShapeReason(name); reason != "" {
		issues = append(issues, Issue{Skill: dirName, Reason: reason})
	}
	if name != dirName {
		issues = append(issues, Issue{Skill: dirName, Reason: fmt.Sprintf(
			"name %q does not match its directory %q", name, dirName,
		)})
	}
	issues = append(issues, descriptionIssues(dirName, doc.description)...)
	return issues
}

// nameShapeReason reports the standard's shape violations: lowercase letters,
// digits, and hyphens only, with no leading, trailing, or doubled hyphen.
func nameShapeReason(name string) string {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return fmt.Sprintf("name %q must use lowercase letters, digits, and hyphens only", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Sprintf("name %q must not begin or end with a hyphen", name)
	}
	if strings.Contains(name, "--") {
		return fmt.Sprintf("name %q must not contain a doubled hyphen", name)
	}
	return ""
}

func descriptionIssues(dirName, description string) []Issue {
	if description == "" {
		return []Issue{{Skill: dirName, Reason: "description is required"}}
	}
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return []Issue{{Skill: dirName, Reason: "description is empty"}}
	}
	if n := utf8.RuneCountInString(trimmed); n > MaxDescriptionLength {
		return []Issue{{Skill: dirName, Reason: fmt.Sprintf(
			"description is %d characters; the limit is %d", n, MaxDescriptionLength,
		)}}
	}
	return nil
}
