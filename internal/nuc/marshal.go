package nuc

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseDocument reads and validates a nuc YAML document.
func ParseDocument(data []byte) (Document, error) {
	var doc Document
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Document{}, fmt.Errorf("nuc: parse: %w", err)
	}
	if err := doc.Validate(); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// MarshalDocument encodes a nuc document as YAML.
func MarshalDocument(doc Document) ([]byte, error) {
	if err := doc.Validate(); err != nil {
		return nil, err
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("nuc: marshal: %w", err)
	}
	return out, nil
}

// Validate checks required fields and path safety.
func (d Document) Validate() error {
	if d.APIVersion != APIVersion {
		return fmt.Errorf("nuc: unsupported apiVersion %q (want %q)", d.APIVersion, APIVersion)
	}
	if d.Kind != Kind {
		return fmt.Errorf("nuc: unsupported kind %q (want %q)", d.Kind, Kind)
	}
	if strings.TrimSpace(d.Metadata.Name) == "" {
		return fmt.Errorf("nuc: metadata.name is required")
	}
	if len(d.Spec.Bees) == 0 {
		return fmt.Errorf("nuc: spec.bees must not be empty")
	}
	for role, body := range d.Spec.Bees {
		if err := validateBeeRole(role); err != nil {
			return err
		}
		if strings.TrimSpace(body) == "" {
			return fmt.Errorf("nuc: bee %q body is empty", role)
		}
	}
	for path := range d.Spec.Prompts {
		if err := validatePromptPath(path); err != nil {
			return err
		}
	}
	return nil
}
