// Package quirks is the knowledge base of every BitBox02 firmware constraint
// the testkit knows about. Each Quirk bundles:
//
//   - a unique ID and human-readable name
//   - the firmware version range it applies to
//   - a Severity hint (Critical / Warning / Hint)
//   - a Source reference (proto file, CHANGELOG entry, observed bug)
//   - optional Detect: a source-level static check ([[guards]] style)
//   - optional Scenario: a *fake.Fake configured to simulate the firmware
//     reaction for runtime tests
//   - optional Match: maps a Go test failure message back to this quirk so
//     audit reports can name the bug class
//
// Quirks live in category files (eth.go, btc.go, ...) and register themselves
// via init(). The audit runner iterates Registry to produce reports.
package quirks

import (
	"strconv"
	"strings"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/fake"
	"github.com/joshuakrueger-dfx/bitbox-testkit/go/core/guards"
)

// Category groups quirks by area of impact.
type Category string

const (
	CategoryETH      Category = "eth"
	CategoryBTC      Category = "btc"
	CategoryCardano  Category = "cardano"
	CategoryMnemonic Category = "mnemonic"
	CategoryProtocol Category = "protocol"
	CategoryApp      Category = "app"
)

// Severity escalates the operational impact of a missed quirk.
type Severity int

const (
	SeverityHint Severity = iota // useful to know
	SeverityWarning              // wrong but recoverable
	SeverityCritical             // silent data loss or crash
)

func (s Severity) String() string {
	switch s {
	case SeverityHint:
		return "hint"
	case SeverityWarning:
		return "warning"
	case SeverityCritical:
		return "critical"
	}
	return "unknown"
}

// FirmwareRange describes the firmware versions a quirk applies to. Min is
// inclusive, Max is exclusive. Empty Min means "from the beginning"; empty
// Max means "forever".
type FirmwareRange struct {
	Min string // e.g. "9.24.0"
	Max string // e.g. "9.99.0"
}

// Applies reports whether the range covers version v (e.g. "9.23.0"). Returns
// false on malformed input rather than panicking.
func (r FirmwareRange) Applies(v string) bool {
	if v == "" {
		return true
	}
	if r.Min != "" && compareVersion(v, r.Min) < 0 {
		return false
	}
	if r.Max != "" && compareVersion(v, r.Max) >= 0 {
		return false
	}
	return true
}

func (r FirmwareRange) String() string {
	switch {
	case r.Min == "" && r.Max == "":
		return "all"
	case r.Min != "" && r.Max == "":
		return ">=" + r.Min
	case r.Min == "" && r.Max != "":
		return "<" + r.Max
	}
	return r.Min + " <= v < " + r.Max
}

// DetectRule is a data-driven static-detection pattern, loaded from
// quirks.json. The audit-runner iterates each Quirk's Patterns and applies
// them to source files.
//
// Kind decodes how the other fields are used:
//
//	"regex"               — line-level match against Regex. FileGlobs limits files.
//	"regex_in_context"    — two-pass: file must contain ContextRegex first,
//	                        then per-line Regex flags occurrences.
//	"ordered_pair"        — file must contain both BeforeRegex and AfterRegex;
//	                        violation = AfterRegex appears before BeforeRegex.
//	"missing_pair_within" — for each Regex match, look at the next
//	                        WithinLines lines for a PairRegex match. If
//	                        none, flag the original. Used for "every //export
//	                        must be followed by defer recoverPanic within
//	                        a few lines" rules.
type DetectRule struct {
	Kind         string   `json:"kind"`
	Regex        string   `json:"regex,omitempty"`
	ContextRegex string   `json:"context_regex,omitempty"`
	BeforeRegex  string   `json:"before_regex,omitempty"`
	AfterRegex   string   `json:"after_regex,omitempty"`
	PairRegex    string   `json:"pair_regex,omitempty"`
	WithinLines  int      `json:"within_lines,omitempty"`
	FileGlobs    []string `json:"file_globs,omitempty"`
	Reason       string   `json:"reason"`
	FixHint      string   `json:"fix_hint,omitempty"`
}

// Quirk is one documented BitBox firmware constraint.
type Quirk struct {
	// ID is a short stable identifier ("E1", "B7", "P2").
	ID string

	// Name is a kebab-case slug suitable for log lines and reports.
	Name string

	// Category buckets the quirk for filtering and reporting.
	Category Category

	// Severity tells consumers how alarmed to be.
	Severity Severity

	// Description is a one-paragraph human explanation.
	Description string

	// Source is a citation (proto path, CHANGELOG version, issue link).
	Source string

	// Firmware bounds the firmware versions the quirk applies to.
	Firmware FirmwareRange

	// Detect runs a static source check at test-time (using a Go testing.TB).
	// Independent of Patterns: Detect is the rich/code-driven path used by
	// dedicated test suites; Patterns drive the data-driven audit-runner.
	Detect func(t guards.TB, srcDir, include string)

	// Scenario returns a configured *fake.Fake the consumer plugs into
	// firmware.NewDevice.
	Scenario func() *fake.Fake

	// Match returns true if a Go test failure output line could plausibly
	// be caused by this quirk. Used by audit runners to classify failures.
	Match func(testOutputLine string) bool

	// Patterns are data-driven static-detection rules loaded from quirks.json.
	// Empty means the quirk has no statically-detectable signature; the
	// audit-runner surfaces it as "needs runtime test coverage".
	Patterns []DetectRule
}

// Registry holds every known quirk. Populated by init() in category files.
var Registry []Quirk

// Register appends a quirk to the global registry. Called from category
// files. Panics on duplicate IDs since that's a programming error.
func Register(q Quirk) {
	for _, existing := range Registry {
		if existing.ID == q.ID {
			panic("quirks: duplicate ID " + q.ID)
		}
	}
	Registry = append(Registry, q)
}

// Filter narrows the registry to quirks matching all set fields.
type Filter struct {
	Category    Category // empty means any
	MinSeverity Severity // SeverityHint accepts everything
	Firmware    string   // version string, empty means any
}

// Subset returns quirks matching the filter.
func Subset(f Filter) []Quirk {
	out := make([]Quirk, 0, len(Registry))
	for _, q := range Registry {
		if f.Category != "" && q.Category != f.Category {
			continue
		}
		if q.Severity < f.MinSeverity {
			continue
		}
		if f.Firmware != "" && !q.Firmware.Applies(f.Firmware) {
			continue
		}
		out = append(out, q)
	}
	return out
}

// compareVersion does a lexical compare of dotted-int versions. Returns
// -1 if a < b, 0 if equal, +1 if a > b. Non-numeric segments are treated as
// 0 (so "9.24.0-rc1" and "9.24.0" compare as equal at the third segment).
func compareVersion(a, b string) int {
	pa := splitVersion(a)
	pb := splitVersion(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var ai, bi int
		if i < len(pa) {
			ai = pa[i]
		}
		if i < len(pb) {
			bi = pb[i]
		}
		if ai < bi {
			return -1
		}
		if ai > bi {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		// strip suffix like "-rc1"
		if i := strings.IndexAny(p, "-+"); i >= 0 {
			p = p[:i]
		}
		n, _ := strconv.Atoi(p)
		out = append(out, n)
	}
	return out
}
