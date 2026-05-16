//go:build simulator

package simulator_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/simulator"
)

// simCacheDir returns the directory simulator binaries are cached in,
// honouring WALLET_TESTKIT_SIMCACHE so CI can pin a single download
// path across all simulator-tagged tests.
func simCacheDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("WALLET_TESTKIT_SIMCACHE")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "bitbox-testkit-simcache")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestSimulatorRoundtrip launches the newest known BitBox02 simulator and
// verifies basic connectivity. Cheaper than the full baseline so we
// keep it as a fast smoke test that fails loudly if the binary cache or
// TCP plumbing is broken.
func TestSimulatorRoundtrip(t *testing.T) {
	inst, err := simulator.Launch(simCacheDir(t))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	t.Cleanup(inst.Stop)

	if inst.Conn == nil {
		t.Fatal("Launch returned nil Conn")
	}
	if inst.Comm == nil {
		t.Fatal("Launch returned nil Comm")
	}
	// Give the simulator a moment to be fully ready, then close cleanly.
	time.Sleep(100 * time.Millisecond)
}

// TestSimulatorBaselineScenarios drives every scenario in BaselineScenarios
// against the simulator firmware and fails the test for any non-pass.
// This is the canonical "does the consumer-facing scenario set still
// match the real firmware contract?" check — CI runs this on every
// push so a firmware drift OR a scenario-logic regression in the
// testkit itself surfaces as a single red signal.
//
// We do NOT use a sub-test per scenario: if any scenario fails, every
// subsequent scenario likely fails too (the device may be in a bad
// state after a failed RestoreFromMnemonic, for example). Reporting
// the first failure plus the post-failure context is more useful than
// burying it under N parallel red dots.
func TestSimulatorBaselineScenarios(t *testing.T) {
	inst, err := simulator.Launch(simCacheDir(t))
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	t.Cleanup(inst.Stop)

	dev, err := simulator.Connect(inst, simulator.ConnectOptions{})
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	scenarios := simulator.BaselineScenarios()
	if len(scenarios) == 0 {
		t.Fatal("BaselineScenarios returned empty slice")
	}

	var firstFailure string
	for _, sc := range scenarios {
		res := sc(dev)
		if res.Passed {
			t.Logf("PASS %-44s (%dms)", res.Name, res.DurationMs)
			continue
		}
		t.Errorf("FAIL %-44s (%dms) — %s", res.Name, res.DurationMs, res.Detail)
		if firstFailure == "" {
			firstFailure = res.Name
		}
	}
	if firstFailure != "" {
		t.Fatalf("first failing scenario: %s — see above for details", firstFailure)
	}
}
