// Command bitbox-audit scans a repository for known BitBox02 firmware-quirk
// regressions and emits a structured report.
//
// The audit runs source-level pattern checks against every supported file
// type (.go, .ts, .tsx, .js). For each finding, the report names the quirk
// from the shared knowledge base — Severity, FirmwareRange, Source citation
// and Description are all derived from quirks.json.
//
// Usage:
//
//	bitbox-audit                        # scan current directory
//	bitbox-audit --repo /path/to/repo
//	bitbox-audit --firmware 9.23.0      # restrict to quirks applying to v9.23.0
//	bitbox-audit --format markdown      # human-readable output
//	bitbox-audit --output report.json   # write to file
//
// Exit codes:
//   0 — no findings
//   1 — usage/IO error
//   2 — findings present (any severity)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/quirks"
)

func quirkIDs(qs []quirks.Quirk) []string {
	out := make([]string, len(qs))
	for i, q := range qs {
		out[i] = q.ID
	}
	return out
}

func main() {
	var (
		repo     = flag.String("repo", ".", "path to repository to scan")
		firmware = flag.String("firmware", "", "firmware version (e.g. 9.23.0) — restricts quirks to those applying to this version")
		format   = flag.String("format", "json", "output format: json | markdown")
		output   = flag.String("output", "", "write report to file (default: stdout)")
	)
	flag.Parse()

	if err := run(*repo, *firmware, *format, *output); err != nil {
		fmt.Fprintf(os.Stderr, "bitbox-audit: %v\n", err)
		os.Exit(1)
	}
}

func run(repo, firmware, format, output string) error {
	abs, err := absPath(repo)
	if err != nil {
		return fmt.Errorf("resolve %q: %w", repo, err)
	}
	files, err := enumerateSources(abs)
	if err != nil {
		return fmt.Errorf("enumerate sources: %w", err)
	}

	relevant := quirks.Subset(quirks.Filter{Firmware: firmware})
	findings := scan(abs, files, relevant)
	coverage := classify(relevant)

	report := Report{
		Repo:       abs,
		Firmware:   firmware,
		FileCount:  len(files),
		QuirkCount: len(relevant),
		Findings:   findings,
		Summary:    summarize(findings),
		Coverage: CoverageReport{
			StaticIDs:      quirkIDs(coverage.Static),
			RuntimeOnlyIDs: quirkIDs(coverage.RuntimeOnly),
		},
	}

	w := os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("open output: %w", err)
		}
		defer f.Close()
		w = f
	}

	switch format {
	case "json":
		if err := report.WriteJSON(w); err != nil {
			return err
		}
	case "markdown":
		if err := report.WriteMarkdown(w); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown format %q (json|markdown)", format)
	}

	if len(findings) > 0 {
		os.Exit(2)
	}
	return nil
}
