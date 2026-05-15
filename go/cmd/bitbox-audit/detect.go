package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/quirks"
)

// Finding is one detected occurrence of a quirk in source.
type Finding struct {
	QuirkID   string `json:"quirk_id"`
	QuirkName string `json:"quirk_name"`
	Category  string `json:"category"`
	Severity  string `json:"severity"`
	File      string `json:"file"`
	Line      int    `json:"line"`
	Snippet   string `json:"snippet"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
	FixHint   string `json:"fix_hint,omitempty"`
}

// regexCache prevents recompiling the same pattern across files.
type regexCache struct {
	mu sync.Mutex
	m  map[string]*regexp.Regexp
}

func (c *regexCache) get(p string) (*regexp.Regexp, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.m[p]; ok {
		return r, nil
	}
	r, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	if c.m == nil {
		c.m = map[string]*regexp.Regexp{}
	}
	c.m[p] = r
	return r, nil
}

// scan applies every applicable Quirk's data-driven Patterns to the given
// source files and returns aggregated findings.
func scan(root string, files []string, applicable []quirks.Quirk) []Finding {
	cache := &regexCache{}
	var out []Finding
	for _, q := range applicable {
		for _, rule := range q.Patterns {
			out = append(out, applyRule(cache, q, rule, files, root)...)
		}
	}
	return out
}

func applyRule(cache *regexCache, q quirks.Quirk, rule quirks.DetectRule, files []string, root string) []Finding {
	switch rule.Kind {
	case "regex":
		return applyRegex(cache, q, rule, files, root)
	case "regex_in_context":
		return applyRegexInContext(cache, q, rule, files, root)
	case "ordered_pair":
		return applyOrderedPair(cache, q, rule, files, root)
	case "missing_pair_within":
		return applyMissingPairWithin(cache, q, rule, files, root)
	default:
		// Unknown kind — silently skip rather than crash the audit. A
		// future schema bump can introduce new kinds while older audit
		// binaries keep running.
		return nil
	}
}

// applyMissingPairWithin flags every Regex match that does NOT have a
// PairRegex match within the following WithinLines lines. Used for
// "every X must be paired with Y nearby" rules (e.g. //export with
// defer recoverPanic).
func applyMissingPairWithin(cache *regexCache, q quirks.Quirk, rule quirks.DetectRule, files []string, root string) []Finding {
	primary, err := cache.get(rule.Regex)
	if err != nil {
		return nil
	}
	pair, err := cache.get(rule.PairRegex)
	if err != nil {
		return nil
	}
	window := rule.WithinLines
	if window <= 0 {
		window = 5
	}
	var out []Finding
	for _, path := range files {
		if !matchesGlobs(path, rule.FileGlobs) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			if !primary.MatchString(line) {
				continue
			}
			end := i + 1 + window
			if end > len(lines) {
				end = len(lines)
			}
			paired := false
			for _, follower := range lines[i+1 : end] {
				if pair.MatchString(follower) {
					paired = true
					break
				}
			}
			if !paired {
				out = append(out, makeFinding(q, rule, path, root, i+1, line))
			}
		}
	}
	return out
}

func applyRegex(cache *regexCache, q quirks.Quirk, rule quirks.DetectRule, files []string, root string) []Finding {
	re, err := cache.get(rule.Regex)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, path := range files {
		if !matchesGlobs(path, rule.FileGlobs) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for i, line := range strings.Split(string(content), "\n") {
			if re.MatchString(line) {
				out = append(out, makeFinding(q, rule, path, root, i+1, line))
			}
		}
	}
	return out
}

func applyRegexInContext(cache *regexCache, q quirks.Quirk, rule quirks.DetectRule, files []string, root string) []Finding {
	re, err := cache.get(rule.Regex)
	if err != nil {
		return nil
	}
	ctxRe, err := cache.get(rule.ContextRegex)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, path := range files {
		if !matchesGlobs(path, rule.FileGlobs) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !ctxRe.Match(content) {
			continue
		}
		for i, line := range strings.Split(string(content), "\n") {
			if re.MatchString(line) {
				out = append(out, makeFinding(q, rule, path, root, i+1, line))
			}
		}
	}
	return out
}

func applyOrderedPair(cache *regexCache, q quirks.Quirk, rule quirks.DetectRule, files []string, root string) []Finding {
	beforeRe, err := cache.get(rule.BeforeRegex)
	if err != nil {
		return nil
	}
	afterRe, err := cache.get(rule.AfterRegex)
	if err != nil {
		return nil
	}
	var out []Finding
	for _, path := range files {
		if !matchesGlobs(path, rule.FileGlobs) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		beforeLoc := beforeRe.FindIndex(content)
		afterLoc := afterRe.FindIndex(content)
		if beforeLoc == nil || afterLoc == nil {
			continue
		}
		if afterLoc[0] < beforeLoc[0] {
			line := 1 + bytes.Count(content[:afterLoc[0]], []byte{'\n'})
			out = append(out, makeFinding(q, rule, path, root, line, extractLine(content, afterLoc[0])))
		}
	}
	return out
}

func makeFinding(q quirks.Quirk, rule quirks.DetectRule, path, root string, line int, snippet string) Finding {
	return Finding{
		QuirkID:   q.ID,
		QuirkName: q.Name,
		Category:  string(q.Category),
		Severity:  q.Severity.String(),
		File:      relative(root, path),
		Line:      line,
		Snippet:   strings.TrimSpace(snippet),
		Reason:    rule.Reason,
		Source:    q.Source,
		FixHint:   rule.FixHint,
	}
}

func matchesGlobs(path string, globs []string) bool {
	if len(globs) == 0 {
		return true
	}
	base := filepath.Base(path)
	for _, g := range globs {
		if ok, _ := filepath.Match(g, base); ok {
			return true
		}
	}
	return false
}

func extractLine(content []byte, offset int) string {
	start := offset
	for start > 0 && content[start-1] != '\n' {
		start--
	}
	end := offset
	for end < len(content) && content[end] != '\n' {
		end++
	}
	return strings.TrimSpace(string(content[start:end]))
}

func relative(root, path string) string {
	if strings.HasPrefix(path, root+"/") {
		return path[len(root)+1:]
	}
	return path
}

// Coverage classifies each quirk's static-detectability and how the audit
// runner can report on it.
type Coverage struct {
	Static     []quirks.Quirk // has at least one Pattern → audit-runner checks it statically
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

// unused so far — placeholder to silence the import linter while the
// Coverage type is wired into reports in a later chunk.
var _ = fmt.Sprintf
