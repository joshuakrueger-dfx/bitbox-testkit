package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joshuakrueger-dfx/bitbox-testkit/go/bitbox/quirks"
)

// quirkByID returns a quirk from the package registry. Fails if the ID is
// not present, which would mean the audit-runner is out of sync with the
// shared quirks knowledge base.
func quirkByID(t *testing.T, id string) quirks.Quirk {
	t.Helper()
	for _, q := range quirks.Registry {
		if q.ID == id {
			return q
		}
	}
	t.Fatalf("quirks.Registry missing %s", id)
	return quirks.Quirk{}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFlagsE1NonAsciiInEIP712Context(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sign.ts", `import { signTypedData } from 'bitbox-api';
const msg = "hëllo from eip712 land";
signTypedData(msg);
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "E1")})
	if len(got) != 1 {
		t.Fatalf("expected 1 E1 finding, got %d: %+v", len(got), got)
	}
	if got[0].QuirkID != "E1" {
		t.Fatalf("wrong id: %s", got[0].QuirkID)
	}
	if !strings.Contains(got[0].Snippet, "hëllo") {
		t.Fatalf("snippet missing umlaut: %q", got[0].Snippet)
	}
}

func TestScanE1IgnoresUnrelatedUmlauts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "i18n.ts", `export const greeting = "Grüße";`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "E1")})
	if len(got) != 0 {
		t.Fatalf("expected 0 findings (no EIP-712 context), got %d", len(got))
	}
}

func TestScanFlagsP2OrderedPairReversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "buggy.go", `package u2fhid
func process(id string) {
    seenPackets.removeAll(stale)
    if seenPackets.contains(id) { return }
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "P2")})
	if len(got) != 1 {
		t.Fatalf("expected 1 P2 finding, got %d: %+v", len(got), got)
	}
}

func TestScanP2PassesCorrectOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fixed.go", `package u2fhid
func process(id string) {
    if seenPackets.contains(id) { return }
    seenPackets.removeAll(stale)
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "P2")})
	if len(got) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(got))
	}
}

func TestScanFlagsA2HardcodedTimeoutsAcrossLanguages(t *testing.T) {
	dir := t.TempDir()
	// File path "transport" supplies the context filter ("transport"
	// matches the context_regex). The regex_in_context detection only
	// fires when both the context filter and the line regex hit, which
	// keeps the rule from flagging unrelated setTimeout calls in app
	// code (e.g. animation delays).
	writeFile(t, dir, "bitbox-transport.go", `package transport
// BitBox transport layer
import "time"
func wait() { time.Sleep(10 * time.Second) }
`)
	writeFile(t, dir, "bitbox-connect.ts", `// BitBox BLE transport
setTimeout(cb, 10000);
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A2")})
	if len(got) < 2 {
		t.Fatalf("expected ≥ 2 A2 findings (Go + TS in BitBox transport context), got %d: %+v", len(got), got)
	}
}

func TestScanFlagsA2DartFutureDelayed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bitbox_ble.dart", `// BitBox BLE transport for Flutter
import 'dart:async';
Future<void> waitConfirm() async {
  await Future.delayed(Duration(seconds: 10));
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A2")})
	if len(got) == 0 {
		t.Fatal("expected A2 finding for Dart Future.delayed(Duration(seconds: 10))")
	}
}

func TestScanA2IgnoresUnrelatedTimeouts(t *testing.T) {
	// 10s timeouts in non-transport contexts (animation, debounce) should
	// not trigger the audit. This is the false-positive guard.
	dir := t.TempDir()
	writeFile(t, dir, "animation.ts", `setTimeout(fade, 10000);  // fade-out delay`)
	writeFile(t, dir, "debounce.go", `package ui
func wait() { time.Sleep(10 * time.Second) }
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A2")})
	if len(got) != 0 {
		t.Fatalf("expected 0 A2 findings in unrelated context, got %d: %+v", len(got), got)
	}
}

func TestScanFlagsE7AddressCaseOutOfRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.ts", `const req = { addressCase: 3 };`)
	writeFile(t, dir, "ok.ts", `const req = { addressCase: 2 };`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "E7")})
	if len(got) != 1 {
		t.Fatalf("expected 1 E7 finding (only the addressCase: 3 line), got %d: %+v", len(got), got)
	}
}

func TestScanFlagsB1LocktimeOverflow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.ts", `const tx = { locktime: 1700000000 };`)         // 2023-11 timestamp
	writeFile(t, dir, "ok.ts", `const tx = { locktime: 800000 };`)               // block height
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "B1")})
	if len(got) != 1 {
		t.Fatalf("expected 1 B1 finding (only the timestamp-style line), got %d: %+v", len(got), got)
	}
}

func TestScanFlagsB2HardcodedVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad-sign.ts", `import { btcSignPsbt } from 'bitbox-api';
const tx = { version: 3, locktime: 0 };
btcSignPsbt(tx);
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "B2")})
	if len(got) == 0 {
		t.Fatal("expected B2 finding for version:3 in BTC sign context")
	}
}

func TestScanB2IgnoresVersionOutsideBtcContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "build.ts", `const pkg = { version: 3 };`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "B2")})
	if len(got) != 0 {
		t.Fatalf("expected 0 B2 findings outside BTC context, got %d", len(got))
	}
}

func TestScanFlagsC2BadCardanoNetwork(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "cardano-flow.ts", `import { cardanoAddress } from 'bitbox-api';
cardanoAddress({ network: 5 });
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "C2")})
	if len(got) == 0 {
		t.Fatal("expected C2 finding for network:5 in Cardano context")
	}
}

func TestScanA3FlagsEthSignWithoutAntiklepto(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "no-antiklepto.ts", `import { ethSignTransaction } from 'bitbox-api';
export async function send(rlp: Uint8Array) {
  return ethSignTransaction(1n, "m/44'/60'/0'/0/0", rlp);
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A3")})
	if len(got) == 0 {
		t.Fatal("expected A3 finding for ethSign* without antiklepto reference nearby")
	}
}

func TestScanA3SilentWhenAntikleptoPresent(t *testing.T) {
	// missing_pair_within scans forward (the recoverPanic-style semantic).
	// To suppress A3, the antiklepto reference must appear on a line
	// AFTER the ethSign* call within the window (default 50 lines).
	dir := t.TempDir()
	writeFile(t, dir, "with-antiklepto.ts", `import { ethSignTransaction } from 'bitbox-api';
export async function send(rlp: Uint8Array) {
  const sig = await ethSignTransaction(1n, "m/44'/60'/0'/0/0", rlp);
  // bitbox-api wraps host_nonce_commitment / antiklepto automatically.
  return sig;
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A3")})
	if len(got) != 0 {
		t.Fatalf("expected 0 A3 findings when antiklepto appears within window after ethSign*, got %d", len(got))
	}
}

func TestScanFlagsA1GomobileExportWithoutContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exports.go", `package main
// gomobile binding entry-point file.
//export DoThing
func DoThing() string {
    return "result"
}
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A1")})
	if len(got) != 1 {
		t.Fatalf("expected 1 A1 finding for the //export comment, got %d: %+v", len(got), got)
	}
}

func TestScanFlagsM118Words(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "recovery.ts", `const options = [{ wordCount: 12 }, { wordCount: 18 }, { wordCount: 24 }];`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "M1")})
	if len(got) == 0 {
		t.Fatal("expected at least one M1 finding for 18-word recovery option")
	}
}

func TestEnumerateSourcesSkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src/a.ts", "")
	writeFile(t, dir, "node_modules/bad.ts", "")
	files, _ := enumerateSources(dir)
	if len(files) != 1 {
		t.Fatalf("expected 1 file (node_modules skipped), got %d: %v", len(files), files)
	}
}

func TestClassifyCoverageSplitsByPatterns(t *testing.T) {
	in := []quirks.Quirk{
		{ID: "X", Patterns: []quirks.DetectRule{{Kind: "regex", Regex: "x"}}},
		{ID: "Y"},
		{ID: "Z", Patterns: []quirks.DetectRule{{Kind: "regex", Regex: "z"}}},
	}
	c := classify(in)
	if len(c.Static) != 2 || len(c.RuntimeOnly) != 1 {
		t.Fatalf("classify wrong: %+v", c)
	}
	if c.RuntimeOnly[0].ID != "Y" {
		t.Fatalf("Y should be runtime-only, got %s", c.RuntimeOnly[0].ID)
	}
}

func TestReportSummary(t *testing.T) {
	r := Report{
		Findings: []Finding{
			{Severity: "critical"},
			{Severity: "critical"},
			{Severity: "warning"},
		},
	}
	s := summarize(r.Findings)
	if s.Critical != 2 || s.Warning != 1 || s.Total != 3 {
		t.Fatalf("summary off: %+v", s)
	}
}

func TestScanRespectsAuditSkipLineMarkerSameLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "demo.ts", `import { ethAddress } from 'bitbox-api';
// docs: callers used to pass displayOnDevice: false; audit-skip-line
const placeholder = true;
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A4")})
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on same-line skip, got %d: %+v", len(got), got)
	}
}

func TestScanRespectsAuditSkipLineMarkerLineAbove(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "demo.ts", `import { ethAddress } from 'bitbox-api';
// audit-skip-line
const placeholder = "displayOnDevice: false is no longer valid";
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A4")})
	if len(got) != 0 {
		t.Fatalf("expected 0 findings on line-above skip, got %d: %+v", len(got), got)
	}
}

func TestScanStillFlagsWithoutSkipMarker(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "demo.ts", `import { ethAddress } from 'bitbox-api';
ethAddress({ displayOnDevice: false });
`)
	files, _ := enumerateSources(dir)
	got := scan(dir, files, []quirks.Quirk{quirkByID(t, "A4")})
	if len(got) != 1 {
		t.Fatalf("expected 1 A4 finding without skip marker, got %d: %+v", len(got), got)
	}
}
