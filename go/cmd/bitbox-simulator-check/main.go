//go:build simulator

// bitbox-simulator-check launches the official BitBox02 simulator and runs
// the testkit's curated baseline scenarios against the REAL firmware logic.
//
// Linux/amd64 only — that's the only platform the simulator binary ships
// for. On macOS / Windows / Linux-arm we exit cleanly with a "skipped"
// status so CI matrices stay simple.
//
// Usage:
//
//	bitbox-simulator-check                          # run baseline, markdown to stdout
//	bitbox-simulator-check --format json            # emit machine-readable
//	bitbox-simulator-check --output report.md       # write to file
//	bitbox-simulator-check --cache ~/.bitbox-cache  # reuse downloaded binaries
//	bitbox-simulator-check --fail-on-skip           # treat skip as failure
//
// Exit codes:
//
//	0  all scenarios passed (or skipped on non-Linux without --fail-on-skip)
//	1  at least one scenario failed
//	2  the simulator could not be launched (download / TCP / binary fault)
//	3  invalid CLI flags
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/simulator"
)

func main() {
	format := flag.String("format", "markdown", "Output format: markdown or json.")
	output := flag.String("output", "", "Write report to file instead of stdout.")
	cacheDir := flag.String("cache", "", "Simulator-binary cache dir (default: $TMPDIR/bitbox-testkit-simcache).")
	failOnSkip := flag.Bool("fail-on-skip", false, "Exit nonzero if scenarios were skipped (non-Linux host).")
	version := flag.Bool("version", false, "Print version and exit.")
	flag.Parse()

	if *version {
		fmt.Println("bitbox-simulator-check dev")
		return
	}

	if *format != "markdown" && *format != "json" {
		fmt.Fprintln(os.Stderr, "--format must be markdown or json")
		os.Exit(3)
	}

	report := buildReport(*cacheDir, *failOnSkip)

	var rendered []byte
	switch *format {
	case "json":
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "marshal:", err)
			os.Exit(2)
		}
		rendered = append(b, '\n')
	default:
		rendered = []byte(renderMarkdown(report))
	}

	if *output != "" {
		if err := os.WriteFile(*output, rendered, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(2)
		}
	} else {
		_, _ = os.Stdout.Write(rendered)
	}

	os.Exit(report.ExitCode)
}

// Report is the JSON-serialisable summary of a simulator run.
type Report struct {
	Host       string              `json:"host"`
	Skipped    bool                `json:"skipped"`
	SkipReason string              `json:"skip_reason,omitempty"`
	Firmware   string              `json:"firmware,omitempty"`
	Started    time.Time           `json:"started"`
	Finished   time.Time           `json:"finished"`
	Results    []simulator.Result  `json:"results"`
	Summary    Summary             `json:"summary"`
	ExitCode   int                 `json:"exit_code"`
}

// Summary is a rollup over the per-scenario results.
type Summary struct {
	Total  int `json:"total"`
	Passed int `json:"passed"`
	Failed int `json:"failed"`
}

func buildReport(cacheDirFlag string, failOnSkip bool) Report {
	started := time.Now()
	host := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)

	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		exitCode := 0
		if failOnSkip {
			exitCode = 2
		}
		return Report{
			Host:       host,
			Skipped:    true,
			SkipReason: "the BitBox02 simulator binary is published for linux/amd64 only",
			Started:    started,
			Finished:   time.Now(),
			ExitCode:   exitCode,
		}
	}

	cacheDir := cacheDirFlag
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "bitbox-testkit-simcache")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return failed(started, host, fmt.Errorf("mkdir cache: %w", err))
	}

	inst, err := simulator.Launch(cacheDir)
	if err != nil {
		return failed(started, host, fmt.Errorf("simulator.Launch: %w", err))
	}
	defer inst.Stop()

	dev, err := simulator.Connect(inst, simulator.ConnectOptions{})
	if err != nil {
		return failed(started, host, err)
	}

	report := Report{
		Host:     host,
		Started:  started,
		Firmware: simulator.Simulators()[0].Name,
	}
	for _, scenario := range simulator.BaselineScenarios() {
		res := scenario(dev)
		report.Results = append(report.Results, res)
		report.Summary.Total++
		if res.Passed {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
	}
	report.Finished = time.Now()
	if report.Summary.Failed > 0 {
		report.ExitCode = 1
	}
	return report
}

func failed(started time.Time, host string, err error) Report {
	return Report{
		Host:    host,
		Started: started,
		Finished: time.Now(),
		Results: []simulator.Result{
			{
				Name:   "launch",
				Passed: false,
				Detail: err.Error(),
			},
		},
		Summary: Summary{Total: 1, Failed: 1},
		ExitCode: 2,
	}
}

func renderMarkdown(r Report) string {
	out := "# BitBox02 simulator check\n\n"
	out += fmt.Sprintf("Host: `%s` — Started: %s — Duration: %s\n\n",
		r.Host, r.Started.Format(time.RFC3339), r.Finished.Sub(r.Started).Round(time.Millisecond))
	if r.Skipped {
		out += fmt.Sprintf("**Skipped:** %s\n", r.SkipReason)
		return out
	}
	out += fmt.Sprintf("Firmware: `%s`\n\n", r.Firmware)
	out += "| Scenario | Result | Duration | Detail |\n"
	out += "|---|---|---:|---|\n"
	for _, res := range r.Results {
		mark := "✅"
		if !res.Passed {
			mark = "❌"
		}
		detail := res.Detail
		if detail == "" {
			detail = "—"
		}
		out += fmt.Sprintf("| `%s` | %s | %dms | %s |\n",
			res.Name, mark, res.DurationMs, detail)
	}
	out += fmt.Sprintf("\n**Summary:** %d total · %d passed · %d failed\n",
		r.Summary.Total, r.Summary.Passed, r.Summary.Failed)
	return out
}

