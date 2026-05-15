package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Report is the top-level audit output.
type Report struct {
	Repo       string         `json:"repo"`
	Firmware   string         `json:"firmware,omitempty"`
	FileCount  int            `json:"files_scanned"`
	QuirkCount int            `json:"quirks_evaluated"`
	Findings   []Finding      `json:"findings"`
	Summary    Summary        `json:"summary"`
	Coverage   CoverageReport `json:"coverage"`
}

type Summary struct {
	Critical int `json:"critical"`
	Warning  int `json:"warning"`
	Hint     int `json:"hint"`
	Total    int `json:"total"`
}

// CoverageReport surfaces the honesty gap: what fraction of quirks the audit
// runner was actually able to check statically. The remainder need dedicated
// runtime tests using the testkit's Scenario fakes.
type CoverageReport struct {
	// StaticIDs are the quirks the audit-runner has detection patterns for.
	StaticIDs []string `json:"static_ids"`
	// RuntimeOnlyIDs are the quirks with no static signature; absence of
	// findings tells you nothing about them — they need runtime tests.
	RuntimeOnlyIDs []string `json:"runtime_only_ids"`
}

func summarize(findings []Finding) Summary {
	s := Summary{}
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			s.Critical++
		case "warning":
			s.Warning++
		case "hint":
			s.Hint++
		}
	}
	s.Total = s.Critical + s.Warning + s.Hint
	return s
}

func (r Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

func (r Report) WriteMarkdown(w io.Writer) error {
	fmt.Fprintf(w, "# BitBox audit — %s\n\n", r.Repo)
	if r.Firmware != "" {
		fmt.Fprintf(w, "Firmware target: `%s`\n\n", r.Firmware)
	}
	fmt.Fprintf(w, "Files scanned: **%d** — Quirks evaluated: **%d**\n\n", r.FileCount, r.QuirkCount)

	// ── Coverage first, so readers can't mistake "0 findings" for "clean" ──
	fmt.Fprintf(w, "## Coverage\n\n")
	fmt.Fprintf(w, "Static detection ran against **%d / %d** quirks (%s). The remaining **%d** have no static signature and can only be caught by runtime tests that drive your code against the testkit's Scenario fakes.\n\n",
		len(r.Coverage.StaticIDs), r.QuirkCount, joinIDs(r.Coverage.StaticIDs), len(r.Coverage.RuntimeOnlyIDs))

	if len(r.Coverage.RuntimeOnlyIDs) > 0 {
		fmt.Fprintf(w, "<details>\n<summary>Quirks not statically checkable — verify via runtime tests</summary>\n\n")
		fmt.Fprintf(w, "%s\n\n</details>\n\n", joinIDs(r.Coverage.RuntimeOnlyIDs))
	}

	// ── Findings summary ──
	fmt.Fprintf(w, "## Findings summary\n\n")
	fmt.Fprintf(w, "| Severity | Count |\n|---|---|\n")
	fmt.Fprintf(w, "| critical | %d |\n| warning | %d |\n| hint | %d |\n| **total** | **%d** |\n\n",
		r.Summary.Critical, r.Summary.Warning, r.Summary.Hint, r.Summary.Total)

	if len(r.Findings) == 0 {
		fmt.Fprintln(w, "_No static findings._ This means the source-level patterns above did not match — it does **not** mean your BitBox integration is bug-free. Most BitBox quirks need runtime test coverage; see Coverage above.")
		return nil
	}

	// ── Detailed findings, grouped by severity ──
	byOrder := []string{"critical", "warning", "hint"}
	groups := map[string][]Finding{}
	for _, f := range r.Findings {
		groups[f.Severity] = append(groups[f.Severity], f)
	}

	for _, sev := range byOrder {
		fs := groups[sev]
		if len(fs) == 0 {
			continue
		}
		sort.Slice(fs, func(i, j int) bool {
			if fs[i].QuirkID != fs[j].QuirkID {
				return fs[i].QuirkID < fs[j].QuirkID
			}
			if fs[i].File != fs[j].File {
				return fs[i].File < fs[j].File
			}
			return fs[i].Line < fs[j].Line
		})
		fmt.Fprintf(w, "## %s findings\n\n", titleize(sev))
		for _, f := range fs {
			fmt.Fprintf(w, "### `%s` — %s\n\n", f.QuirkID, f.QuirkName)
			fmt.Fprintf(w, "- **File:** `%s:%d`\n", f.File, f.Line)
			fmt.Fprintf(w, "- **Snippet:** `%s`\n", f.Snippet)
			fmt.Fprintf(w, "- **Reason:** %s\n", f.Reason)
			if f.FixHint != "" {
				fmt.Fprintf(w, "- **Fix:** %s\n", f.FixHint)
			}
			fmt.Fprintf(w, "- **Source:** %s\n\n", f.Source)
		}
	}
	return nil
}

func joinIDs(ids []string) string {
	if len(ids) == 0 {
		return "_(none)_"
	}
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += "`" + id + "`"
	}
	return out
}

func titleize(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}
