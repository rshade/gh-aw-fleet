package security

import (
	"fmt"
	"strings"
)

// RenderForStderr returns a multi-line plain-text rendering of findings,
// one line per finding, suitable for stderr or test golden assertions.
// Empty string when len(findings) == 0.
//
// Format: "[SEVERITY] rule_id  file:line  message" with two-space
// separators. When Line == 0, the file slot omits the ":0" suffix.
// When both File == "" and Line == 0, the file slot is rendered as "-".
func RenderForStderr(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	for i, f := range findings {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "[%s] %s  %s  %s",
			f.Severity.String(), f.RuleID, fileSlot(f), f.Message,
		)
	}
	return b.String()
}

// RenderForPRBody returns the markdown body of the `## Security Findings`
// section: a `**Summary**:` tally line followed by per-finding bullets in
// sorted order. Empty string when len(findings) == 0 — caller suppresses
// the heading entirely (FR-005).
//
// Per-finding bullets use em-dash (U+2014) separators. The leading
// `## Security Findings` heading is added by RenderPRSection, not by this
// function — RenderForPRBody is also exported for tests that assert on
// the body shape independent of the heading.
func RenderForPRBody(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("**Summary**: ")
	b.WriteString(severityTally(findings))
	b.WriteString("\n\n")
	for i, f := range findings {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b,
			"- **%s** `%s` — `%s` — %s — %s",
			f.Severity.String(), f.RuleID, fileSlot(f), f.Message, f.Remedy,
		)
	}
	return b.String()
}

// SeveritySummary returns the per-severity tally used by both the PR-body
// summary and the interactive findings prompt — e.g. "2 HIGH, 1 MEDIUM" —
// in HIGH→MEDIUM→LOW→INFO order, omitting zero counts. Returns the empty
// string when len(findings) == 0. It is a thin exported wrapper over the
// unexported severityTally so the prompt's one-line summary stays identical
// to the PR-body summary (one severity-counting code path).
func SeveritySummary(findings []Finding) string {
	return severityTally(findings)
}

// numSeverityBuckets is the count of distinct severity values whose
// counts are tallied in the PR-body summary line (HIGH, MEDIUM, LOW,
// INFO). The Renovate config scanner emits LOW (FR-015).
const numSeverityBuckets = 4

// severityTally returns "2 HIGH, 1 MEDIUM, 1 INFO" — severities present in
// findings, in HIGH→MEDIUM→LOW→INFO order, omitting zero counts.
func severityTally(findings []Finding) string {
	var counts [numSeverityBuckets]int
	for _, f := range findings {
		switch f.Severity {
		case SeverityHigh:
			counts[0]++
		case SeverityMedium:
			counts[1]++
		case SeverityLow:
			counts[2]++
		case SeverityInfo:
			counts[3]++
		}
	}
	labels := [numSeverityBuckets]string{
		severityHighLabel,
		severityMediumLabel,
		severityLowLabel,
		severityInfoLabel,
	}
	parts := make([]string, 0, numSeverityBuckets)
	for i, c := range counts {
		if c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, labels[i]))
		}
	}
	return strings.Join(parts, ", ")
}

// RenderPRSection returns the markdown for the `## Security Findings`
// section of a deploy/upgrade PR body — heading, summary tally, per-finding
// bullets, and a trailing newline. Returns the empty string when there are
// no findings so callers can suppress the heading and any inter-section
// blank line they would otherwise emit (FR-005).
func RenderPRSection(findings []Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Security Findings\n\n")
	b.WriteString(RenderForPRBody(findings))
	b.WriteString("\n")
	return b.String()
}

// fileSlot renders the file:line slot per the contract:
//   - both populated → "file:line"
//   - File != "" && Line == 0 → "file"
//   - File == "" && Line == 0 → "-"
func fileSlot(f Finding) string {
	switch {
	case f.File == "" && f.Line == 0:
		return "-"
	case f.Line == 0:
		return f.File
	default:
		return fmt.Sprintf("%s:%d", f.File, f.Line)
	}
}
