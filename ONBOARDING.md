# Onboarding — bitbox-testkit in 5 minutes

You have an app that talks to a BitBox02. You want to know whether it handles every documented firmware quirk safely. This page takes you from zero to "audit comment on every PR" without reading the rest of the repo.

For schema details, see [`quirks/SCHEMA.md`](quirks/SCHEMA.md). For the full per-layer cookbook, see [`TESTING.md`](TESTING.md).

---

## What this kit gives you

| Capability | Where | What it catches |
| ---------- | ----- | --------------- |
| **`bitbox-audit` CLI** | repo-agnostic, runs on any clone | Static source patterns for known regressions |
| **Scriptable fakes (Go + TS)** | `/go/bitbox/fake`, `/ts/src/fake` | Plug into `firmware.NewDevice` or replace `bitbox-api` in Jest |
| **Scenario library** | `/go/bitbox/scenarios`, `/ts/src/scenarios` | Pre-built firmware responses for known bug classes |
| **Quirk registry (30 entries)** | `/go/bitbox/quirks/quirks.json` (canonical) | Severity, firmware version range, source citation, match regex, optional static-detection rule |
| **Source guards** | `/go/core/guards`, `/ts/src/guards` | Function-level static checks for test-time use |
| **Vendor simulator integration** (Go, Linux/amd64) | `/go/bitbox/simulator` | End-to-end against real firmware logic |

---

## 5-minute setup

### Pick your language

- **TypeScript / React Native / Web** consuming `bitbox-api` (Rust/WASM) → use `/ts/`.
- **Flutter plugin / Go consumer** of `bitbox02-api-go` → use `/go/`.

The two implementations share one knowledge base. A consumer in either language gets the same quirk coverage.

### 1 · Install the CLI on any developer machine

```bash
go install github.com/joshuakrueger-dfx/bitbox-testkit/go/cmd/bitbox-audit@main
go install github.com/joshuakrueger-dfx/bitbox-testkit/go/cmd/bitbox-audit-explain@main
```

### 2 · Audit your repo right now

```bash
bitbox-audit --repo /path/to/your/wallet --format markdown
```

You will see:

- **Static detection bucket** — quirks the audit checked for source patterns (currently 11 of 31: `E1`, `E7`, `B1`, `B2`, `C2`, `M1`, `P2`, `A1`, `A2`, `A3`, `A4`).
- **Not covered** — quirks with no static signature. These can only be caught by runtime tests you write using the testkit's Scenario fakes.

Zero static findings is **not** the same as "your integration is safe." Most quirks need runtime tests. Continue to step 3.

### 3 · Wire dynamic test coverage

In your test names, reference the quirk ID. The audit-runner scans `--test-results` for these references and reports per-quirk pass/fail.

#### TypeScript / Jest

Inside your existing test suite:

```ts
import { buildPairedBitBox } from '@joshuakrueger-dfx/bitbox-testkit/fake';
import { scenarioRegressionUmlautEIP712 } from '@joshuakrueger-dfx/bitbox-testkit/scenarios';

describe('signing — quirk E1 (non-ASCII EIP-712)', () => {
  it('rejects messages containing non-ASCII bytes', async () => {
    // mock bitbox-api with the scenario, drive your code, assert
  });
});
```

Run your tests with `--json --outputFile=jest-results.json`, then:

```bash
npx jest --json --outputFile=jest-results.json
bitbox-audit --repo . --test-results jest-results.json --format markdown
```

The Jest result names get scanned for quirk IDs (`quirk E1`, `QuirkE1`, etc.) and folded into the Coverage table.

#### Go / `go test`

```go
func TestSignMessage_QuirkE1_NonAsciiRejected(t *testing.T) {
    fake := scenarios.RegressionUmlautEIP712()
    // ... wire fake into firmware.NewDevice, drive your code, assert
}
```

```bash
go test -json ./... > go-test-results.json
bitbox-audit --repo . --test-results go-test-results.json --format markdown
```

### 4 · CI integration (one-line drop-in)

The testkit ships a composite GitHub Action. Every DFX repo's BitBox-audit workflow collapses to a single `uses:` step.

#### Option A · PR-on-open audit (recommended)

`.github/workflows/bitbox-audit.yml`:

```yaml
name: bitbox-audit
on:
  pull_request:
    paths:
      - 'src/**'
      - 'test/**'
      - 'package.json'
  workflow_dispatch:

permissions:
  contents: read
  pull-requests: write

jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: joshuakrueger-dfx/bitbox-testkit/.github/actions/bitbox-audit@v0.2.0
        # Optional inputs:
        # with:
        #   firmware: '9.23.0'           # narrow quirks to a fw range
        #   jest-extra-args: '--selectProjects unit'
        #   fail-on-findings: 'true'     # block PRs with criticals
        #   testkit-ref: main            # track bleeding edge instead of v0.2.0
```

That single `uses:` step does: Go + Node setup, dependency install, Jest run, audit CLI install + run, sticky PR comment, artifact upload. No copy-pasted YAML to drift.

#### Option B · Slash-command "on-demand audit"

Drop `.github/workflows/bitbox-audit-slash.yml` (from the testkit's [`workflow-templates`](.github/workflow-templates/)) and any maintainer can comment `/bitbox-audit` on a PR to re-run the audit. Supports modifiers:

```
/bitbox-audit                    # defaults
/bitbox-audit firmware=9.23.0    # narrow to fw
/bitbox-audit fail               # block PR on findings
/bitbox-audit ref=main           # use bleeding-edge testkit
```

Authorization gates on `author_association ∈ {OWNER, MEMBER, COLLABORATOR}` to prevent drive-by triggers.

#### Both options share the same composite action

So a regression in audit logic ships once via a testkit tag bump; consumer workflows don't need editing.

A working example lives in [`dfx-wallet`'s PR #153](https://github.com/DFXswiss/dfx-wallet/pull/153).

### 5 · Read the audit comment on your next PR

The sticky comment shows:

- **Static findings**: the audit ran patterns against your source; any matches are flagged with file:line, severity, and a fix hint.
- **Coverage buckets**: which quirks are statically detected, which are covered by passing runtime tests, which have failing tests, which are untested.
- **Untested quirks**: explicit list. Each one is a gap until you add a test for it.

### 6 · End-to-end against the real firmware (`bitbox-simulator`)

The audit catches anti-patterns in your source. The simulator action catches what only the actual BitBox firmware can tell you: does your wire format round-trip, does pairing complete, do multi-page typed-data signs hold, does the firmware accept the bytes you're about to ship to a user's device.

`.github/workflows/bitbox-simulator.yml`:

```yaml
name: bitbox-simulator
on:
  pull_request:
    paths:
      - 'src/**'
      - 'test/**'
  workflow_dispatch:

permissions:
  contents: read
  pull-requests: write

jobs:
  simulator:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: joshuakrueger-dfx/bitbox-testkit/.github/actions/bitbox-simulator@v0.5.0
        with:
          testkit-ref: v0.5.0
          # Optional:
          #   firmware: bitbox02-multi-v9.21.0-simulator1.0.0-linux-amd64
          #   firmware: all      # matrix over every embedded firmware
```

What runs: the action downloads the SHA-pinned upstream BitBox02 simulator binary, brings it up via `Init → ChannelHash → ChannelHashVerify`, restores the deterministic fixture seed (root fingerprint `4c00739d`), then runs the **14 baseline scenarios** in order:

1. `pair_and_device_info` — Noise XX + DeviceInfo
2. `restore_simulator_mnemonic` — deterministic seed
3. `root_fingerprint_deterministic` — pins `4c00739d`
4. `eth_address_mainnet` — chainId=1 BIP-44
5. `eth_address_polygon_multibyte_v` — chainId=137 address
6. `eth_sign_message_ascii` — short personal sign
7. `eth_sign_message_boundary_1024` — firmware-doc max
8. `eth_sign_legacy_polygon_multibyte_v` — actual EIP-155 sign at chainId=137
9. `eth_sign_eip1559_mainnet` — type-2 tx, realistic payload
10. `eth_sign_typed_data_kyc_multipage` — 13-field EIP-712 multi-page (1/13 → 13/13)
11. `eth_sign_typed_data_non_ascii_rejected` — quirk E1 firmware-reject contract
12. `btc_xpub_zpub_mainnet` — BIP-84 ZPUB shape
13. `btc_address_p2wpkh_mainnet` — bech32 `bc1q…`
14. `btc_address_p2tr_taproot` — bech32m `bc1p…`
15. `btc_sign_message_mainnet` — 64-byte sig + 65-byte Electrum envelope

Total run on a GitHub-hosted Linux runner: ~400ms scenarios + ~3s setup. The simulator is **Linux/amd64 only** — on macOS / Windows / arm runners the action exits cleanly with a `skipped` status (use `fail-on-skip: true` to make non-Linux a hard fail).

#### Matrix mode

Set `firmware: all` to drive every embedded firmware version (v9.19.0 → v9.26.1) in parallel. Catches regressions that only surface on older firmwares still in the user wild — the BitBox02 only auto-updates when the user opens the BitBoxApp, so production has a long tail.

#### Slash trigger

Comment `/bitbox-simulator` on any PR. Modifiers: `firmware=v9.21.0`, `firmware=all`, `ref=Y`, `fail`. Auth gated on `author_association ∈ {OWNER, MEMBER, COLLABORATOR}`.

#### What it doesn't catch

The simulator validates the `bitbox-api` ↔ firmware protocol surface. It does **not** validate your consumer's USB-HID / BLE transport layer against real hardware — that still requires a physical BitBox02. The two checks are complementary: audit covers source patterns, simulator covers firmware contract, hardware-on-the-table covers transport.

---

## Test naming convention (important)

For the audit-runner to pick up your dynamic test coverage, every test that exercises a quirk **must mention the quirk ID** somewhere in its `describe` or `it` chain. The audit-runner pattern-matches against the test's full name. The matcher is permissive but specific:

### Patterns that work

```ts
describe('quirk E1 (non-ASCII EIP-712)', () => {
  it('rejects umlauts', ...);
});
// → covers E1

it('quirk A1 — bridge throws synchronously', ...);
// → covers A1

it('Quirk E10: unknown chain ID', ...);          // case-insensitive after "Quirk"
// → covers E10

it('TestQuirkE1Umlaut', ...);                    // camelCase
// → covers E1

it('handles E10 unknown chain', ...);            // standalone, with whitespace boundaries
// → covers E10
```

### Patterns that don't work

```ts
it('rejects umlaut bytes', ...);                 // no quirk ID anywhere
it('handles invalid input', ...);                // ambiguous
it('E1', ...);                                   // too short — needs a separator after
it('quirkE-1', ...);                             // dash inside the ID
```

### Verify locally before pushing

Run `bitbox-audit --test-results <jest-output>` against your test file. The `Runtime tests passing` bucket lists every quirk ID the audit linked. If a test you intended to cover quirk X3 doesn't show up there, the name doesn't match — rename it.

### Why this exists

The audit-runner has no other way to know that `it('handles bad input')` was meant to cover quirk E2 versus E3 versus B7. The quirk ID in the test name is the explicit link. We considered tagging via JSDoc or magic comments; in-test-name is the simplest convention that survives test runners, IDE refactors, and CI parsers.

---

## When the audit flags something

For each finding, the comment includes:

- **Quirk ID** + **severity** — look up in the registry table below
- **File:line** + snippet
- **Reason** — what's wrong
- **Fix hint** — one-line suggested remediation
- **Source** — citation (proto file, CHANGELOG version, observed-in-production note)

If you want a narrative summary:

```bash
bitbox-audit --repo . --format json | bitbox-audit-explain
```

With `ANTHROPIC_API_KEY` set, this calls Claude with a structured prompt. Without it, the prompt is printed for manual paste.

---

## Adding test coverage for an untested quirk

The untested-quirks list is the work queue. For each one:

1. Look up the quirk in [`/go/bitbox/quirks/quirks.json`](go/bitbox/quirks/quirks.json) — read `description`, `firmware`, `source`.
2. Pick a Scenario from `bitbox/scenarios` (or write a new one if the quirk needs something exotic). Most quirks share `scenarioErrInvalidInput` — that's a wire-level firmware reject, which is the right shape for "client sent something invalid."
3. Find the production call site in your code that could trigger the quirk.
4. Write a test that drives the call site with input the quirk would trip on, assert the right behaviour (transliteration, validation, error surfacing).
5. Name the test with the quirk ID so the audit-runner counts it: `it('quirk E1 — rejects non-ASCII', …)`.

---

## Adding a new quirk to the knowledge base

When you discover a new firmware constraint that should never bite anyone again:

1. Add an entry to `/go/bitbox/quirks/quirks.json`.
2. Run `./scripts/sync-quirks.sh` to refresh the TS copy.
3. If the bug class has a static signature (forbidden literal, wrong API call shape, ordered pair of operations), add a `detect` array. See [`quirks/SCHEMA.md`](quirks/SCHEMA.md) for the kinds.
4. If there's a wire-level firmware response, add a Scenario factory in `/go/bitbox/scenarios` and `/ts/src/scenarios`.
5. Open a PR. The shared quirks JSON means everyone using the testkit picks up the new detection on their next `go install`.

---

## Per-language deep dives

For full API documentation per layer (fake builders, scenario factories, guard helpers, vendor simulator), see [`TESTING.md`](TESTING.md). It's the long-form companion to this page.

---

## Reference: quirks by category

The 31 documented quirks. Run `bitbox-audit --format json | jq '.findings[].quirk_id'` against your repo for the subset relevant to you.

| Category | Count | Severity-mix |
| -------- | ----: | ------------ |
| ETH | 10 | 3 critical · 5 warning · 2 hint |
| BTC | 7 | 3 critical · 4 warning · 0 hint |
| Cardano | 4 | 3 critical · 0 warning · 1 hint |
| Mnemonic | 3 | 0 critical · 1 warning · 2 hint |
| Protocol | 3 | 2 critical · 0 warning · 1 hint |
| App | 4 | 3 critical · 1 warning · 0 hint |

Critical quirks are silent-corruption / silent-crash class. Warnings are user-visible but recoverable. Hints are documentation reminders.

---

## When you outgrow this onboarding

- The [`TESTING.md`](TESTING.md) cookbook covers the BLE transport fake (Go), vendor simulator integration, and the full scenario / guard API.
- [`quirks/SCHEMA.md`](quirks/SCHEMA.md) is the contract for adding new quirks.
- The [Audit CLI source](go/cmd/bitbox-audit) is small enough to read end-to-end if you want to extend it (new detection kinds, additional output formats).
