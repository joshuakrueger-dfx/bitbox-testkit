package main

import (
	"bytes"
	"os"
	"strings"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/quirks"
)

// Finding is the audit-runner-local alias for quirks.Finding so report.go
// keeps a stable shape independent of internal type moves.
type Finding = quirks.Finding

// auditSkipLineMarker is the inline-comment string consumers add to a
// source line (or the line directly above the offending tokens) to
// silence a per-line false positive. Use sparingly — prefer
// audit-skip-file for whole-file SDK boundaries.
const auditSkipLineMarker = "audit-skip-line"

// scan walks every file once, asks quirks.ScanFile to apply each
// applicable rule, and aggregates findings. Findings whose source
// line (or the line immediately above) contains the audit-skip-line
// marker are dropped so doc-comments demonstrating an anti-pattern
// don't get flagged as real code.
func scan(root string, files []string, applicable []quirks.Quirk) []Finding {
	var out []Finding
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		rel := relative(root, path)
		raw := quirks.ScanFile(rel, content, applicable)
		if len(raw) == 0 {
			continue
		}
		lines := bytes.Split(content, []byte("\n"))
		for _, f := range raw {
			if isLineSuppressed(lines, f.Line) {
				continue
			}
			out = append(out, f)
		}
	}
	return out
}

// isLineSuppressed reports whether the audit-skip-line marker appears
// on the finding's own line OR on the immediately-preceding line. The
// 1-line lookback supports the natural "comment on the line above"
// pattern many editors and linters use.
func isLineSuppressed(lines [][]byte, line int) bool {
	if line <= 0 || line > len(lines) {
		return false
	}
	// 1-based to 0-based.
	idx := line - 1
	if strings.Contains(string(lines[idx]), auditSkipLineMarker) {
		return true
	}
	if idx > 0 && strings.Contains(string(lines[idx-1]), auditSkipLineMarker) {
		return true
	}
	return false
}

func relative(root, path string) string {
	if len(path) > len(root)+1 && path[:len(root)+1] == root+"/" {
		return path[len(root)+1:]
	}
	return path
}

// Coverage classifies each quirk's static-detectability and how the audit
// runner can report on it.
type Coverage struct {
	Static      []quirks.Quirk // has at least one Pattern → audit-runner checks it statically
	RuntimeOnly []quirks.Quirk // no Patterns → only catchable via runtime tests
}

func classify(applicable []quirks.Quirk) Coverage {
	c := Coverage{}
	for _, q := range applicable {
		if len(q.Patterns) > 0 {
			c.Static = append(c.Static, q)
		} else {
			c.RuntimeOnly = append(c.RuntimeOnly, q)
		}
	}
	return c
}
