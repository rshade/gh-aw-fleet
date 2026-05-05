// Package frontmatter provides leaf-package helpers for splitting and
// parsing the YAML frontmatter from gh-aw markdown workflow files.
//
// It is intentionally a leaf (no dependencies on internal/fleet) so that
// internal/fleet AND its sub-packages (e.g. internal/fleet/security) can
// share the same parser without import cycles. internal/fleet re-exports
// Split / Parse / ErrEmpty under the legacy SplitFrontmatter /
// ParseFrontmatter / ErrEmptyFrontmatter names.
package frontmatter

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// utf8BOM is the UTF-8 byte-order mark sometimes prepended to text files
// by Windows editors. Stripped before fence detection. Constructed from
// raw bytes so the source file itself does not contain the literal,
// avoiding "illegal byte order mark" on toolchains that scan source.
//
//nolint:gochecknoglobals // immutable single-use marker
var utf8BOM = string([]byte{0xEF, 0xBB, 0xBF})

// ErrEmpty is returned by Parse when the input has no YAML content.
// Callers typically treat this as a non-fatal skip.
var ErrEmpty = errors.New("empty frontmatter")

// Split separates a markdown file's YAML frontmatter from its body.
// Returns (frontmatterYAML, body) — frontmatterYAML is empty if no
// "---" fence.
func Split(src string) (string, string) {
	src = strings.TrimPrefix(src, utf8BOM)
	if !strings.HasPrefix(src, "---") {
		return "", src
	}
	after := strings.TrimPrefix(src, "---")
	after = strings.TrimPrefix(after, "\r")
	after = strings.TrimPrefix(after, "\n")
	end := strings.Index(after, "\n---")
	if end < 0 {
		return "", src
	}
	fm := after[:end]
	body := after[end+len("\n---"):]
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	return fm, body
}

// Parse yaml-decodes a frontmatter chunk into a generic map.
func Parse(fm string) (map[string]any, error) {
	if strings.TrimSpace(fm) == "" {
		return nil, ErrEmpty
	}
	out := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &out); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	return out, nil
}
