// audit-skip-file: this file documents the patterns the audit-runner looks
// for. Without this marker it would self-flag.
package guards

import (
	"bytes"
	"regexp"
	"strings"
)

// Pre-built guards for known BitBox plugin regressions. Call these from a
// consumer's _test.go with the source directory to scan.

// MustHaveRecoverPanicOnExports asserts every gomobile-exported function in
// the given directory has a `defer recoverPanic(` or similar guard. The
// regex matches function declarations whose names start with an uppercase
// letter (Go convention for exported) and verifies the next ~5 lines
// reference recoverPanic.
//
// This is a heuristic: false positives are possible for non-export-bound
// helpers. The rule is "every exported function the plugin surfaces to
// Flutter must defer recoverPanic"; see the regex literal for the exact
// match expected.
var recoverPanicRequired = regexp.MustCompile(`(?m)^func\s+[A-Z][A-Za-z0-9_]*\s*\([^)]*\)[^{]*\{(?:\s*//[^\n]*)?\s*(?:[^d]|d[^e]|de[^f])`)

// BLE-dedup ordering: contains() must be checked before removeAll() runs.
// The bug class: removeAll fires first and the subsequent contains is
// never true, silently dropping legitimate retransmits.
var (
	seenPacketsContains  = regexp.MustCompile(`seenPackets\.contains\s*\(`)
	seenPacketsRemoveAll = regexp.MustCompile(`seenPackets\.removeAll\s*\(`)
)

// BitBoxDedupOrder fails if any source file under root contains
// `seenPackets.removeAll(` before `seenPackets.contains(`. Run from the
// plugin's u2fhid package directory.
func BitBoxDedupOrder(t TB, root, include string) {
	t.Helper()
	MustOrderPaired(t, root, include,
		seenPacketsContains, seenPacketsRemoveAll,
		"contains() must be evaluated before removeAll() in BLE packet de-dup; reversing the order silently drops legitimate retransmits")
}

// hardcoded10sTimeout matches the legacy `time.Sleep(10 * time.Second)` or
// `time.After(10 * time.Second)` pattern in transport code. The fix was to
// remove the hard-coded 10s and use context deadlines.
var hardcoded10sTimeout = regexp.MustCompile(`time\.(Sleep|After)\(\s*10\s*\*\s*time\.Second\s*\)`)

// NoHardcoded10sTransportTimeout fails if any file contains
// `time.Sleep(10 * time.Second)` or `time.After(10 * time.Second)`. Run
// from transport-layer source directories.
func NoHardcoded10sTransportTimeout(t TB, root, include string) {
	t.Helper()
	MustNotMatch(t, root, include, hardcoded10sTimeout,
		"hard-coded 10s timeouts in transport code blocked user-confirm flows; use context deadlines")
}

// nonAsciiInStringLiteral matches a quoted string that contains a non-ASCII
// byte (≥ 0x80). Used in tandem with a file-level EIP-712 context filter.
var nonAsciiInStringLiteral = regexp.MustCompile(`["'][^"']*[\x80-\xff][^"']*["']`)

// NoNonAsciiInEIP712Literals fails if any file under root that mentions
// EIP-712 / signTyped also contains a string literal with non-ASCII bytes.
// The bug class: BitBox firmware rejects non-ASCII in EIP-712 string
// values (ErrInvalidInput 101), so the plugin must transliterate to ASCII
// before sending. The two-pass scan keeps noise low: unrelated files with
// umlauts are ignored.
func NoNonAsciiInEIP712Literals(t TB, root, include string) {
	t.Helper()
	err := walkFiles(root, include, func(path string, content []byte) {
		lower := bytes.ToLower(content)
		if !bytes.Contains(lower, []byte("eip712")) && !bytes.Contains(lower, []byte("signtyped")) {
			return
		}
		loc := nonAsciiInStringLiteral.FindIndex(content)
		if loc == nil {
			return
		}
		line := 1 + strings.Count(string(content[:loc[0]]), "\n")
		t.Errorf("guards: %s:%d contains a non-ASCII string literal in an EIP-712/signTyped context; transliterate via toBitboxSafeAscii before signing",
			path, line)
	})
	if err != nil {
		t.Errorf("guards: walk %s: %v", root, err)
	}
}

var _ = recoverPanicRequired // reserved for a follow-up MustHaveRecoverPanicOnExports impl
