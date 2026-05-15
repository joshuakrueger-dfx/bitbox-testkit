// audit-skip-file: documents the patterns the audit-runner looks for.

// Package guards provides source-level regression checks. Consumers run
// these from their own test suite to prevent a known bug class from
// silently coming back through a refactor.
//
// Each guard reads files under a directory, applies a regex (or absence
// check), and reports a failure via the testing.TB so the failure is
// attributable to the call site.
package guards

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TB is the minimal subset of testing.TB the guards need. Tests substitute
// a custom implementation to verify a guard correctly fires.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

// Match is the location of a regex hit inside a file.
type Match struct {
	Path string
	Line int
	Text string
}

// MustNotMatch fails t if pattern matches any line in any file under root
// matching include (a doublestar-free glob, e.g. "*.go"). reason is shown
// alongside the failure to explain the rule.
func MustNotMatch(t TB, root, include string, pattern *regexp.Regexp, reason string) {
	t.Helper()
	matches, err := scan(root, include, pattern)
	if err != nil {
		t.Errorf("guards: scan %s: %v", root, err)
		return
	}
	if len(matches) == 0 {
		return
	}
	t.Errorf("guards: forbidden pattern %q matched %d location(s) (reason: %s):\n%s",
		pattern.String(), len(matches), reason, formatMatches(matches))
}

// MustMatchAtLeast fails t if pattern does not match at least min times.
// Useful for "every exported func must call recoverPanic" style checks
// when paired with a tight pattern.
func MustMatchAtLeast(t TB, root, include string, pattern *regexp.Regexp, min int, reason string) {
	t.Helper()
	matches, err := scan(root, include, pattern)
	if err != nil {
		t.Errorf("guards: scan %s: %v", root, err)
		return
	}
	if len(matches) < min {
		t.Errorf("guards: pattern %q matched %d location(s), expected >= %d (reason: %s)",
			pattern.String(), len(matches), min, reason)
	}
}

// MustOrderPaired fails t if any file contains `second` on a line that
// precedes the first occurrence of `first`. The classic motivating bug:
// `seenPackets.removeAll(...)` called before `seenPackets.contains(...)`
// in a BLE packet de-dup check.
func MustOrderPaired(t TB, root, include string, first, second *regexp.Regexp, reason string) {
	t.Helper()
	err := walkFiles(root, include, func(path string, content []byte) {
		firstIdx := first.FindIndex(content)
		secondIdx := second.FindIndex(content)
		if firstIdx == nil || secondIdx == nil {
			return
		}
		if secondIdx[0] < firstIdx[0] {
			line := 1 + strings.Count(string(content[:secondIdx[0]]), "\n")
			t.Errorf("guards: in %s line %d, %q appears before %q (reason: %s)",
				path, line, second.String(), first.String(), reason)
		}
	})
	if err != nil {
		t.Errorf("guards: walk %s: %v", root, err)
	}
}

func scan(root, include string, pattern *regexp.Regexp) ([]Match, error) {
	var out []Match
	err := walkFiles(root, include, func(path string, content []byte) {
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if pattern.MatchString(line) {
				out = append(out, Match{Path: path, Line: i + 1, Text: strings.TrimSpace(line)})
			}
		}
	})
	return out, err
}

func walkFiles(root, include string, fn func(path string, content []byte)) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Skip vendored and module-cache dirs by convention.
			name := d.Name()
			if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") && name != "." {
				return fs.SkipDir
			}
			return nil
		}
		ok, _ := filepath.Match(include, filepath.Base(path))
		if !ok {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fn(path, b)
		return nil
	})
}

func formatMatches(ms []Match) string {
	var sb strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&sb, "  %s:%d  %s\n", m.Path, m.Line, m.Text)
	}
	return sb.String()
}
