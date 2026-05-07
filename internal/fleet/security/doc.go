// Package security implements the v1 layer-1 security scanner that runs
// after `gh aw add` produces source markdown and compiled YAML in the
// work-dir clone, and before the deploy/sync/upgrade commit gate.
//
// The single entry point is Run, which orchestrates three detectors —
// embedded-credential scanning (via github.com/zricethezav/gitleaks/v8),
// fleet-structural rule evaluation against workflow frontmatter, and
// (when the binary is installed) compiled-YAML linting via actionlint.
// Findings are advisory: they never block the run; they surface on
// stderr (zerolog), in the JSON envelope's warnings[], and in the
// PR body's `## Security Findings` section.
package security
