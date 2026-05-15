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

- **Static detection bucket** — quirks the audit checked for source patterns (currently 4 of 30: `E1`, `M1`, `P2`, `A2`).
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

### 4 · CI integration (GitHub Actions)

Drop this workflow into your repo. Adapt `paths:` to match where BitBox code lives.

```yaml
name: bitbox-audit
on:
  pull_request:
    paths: ['src/**', 'test/**']

permissions:
  contents: read
  pull-requests: write

jobs:
  audit:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - uses: actions/setup-node@v4
        with: { node-version: '20', cache: 'npm' }
      - run: npm ci --no-audit
      - name: install audit CLI
        run: |
          go install github.com/joshuakrueger-dfx/bitbox-testkit/go/cmd/bitbox-audit@main
      - name: run jest with json output
        run: |
          npx jest --json --outputFile=jest.json \
            --testPathPattern='bitbox.*\\.test\\.ts' || true
      - name: run audit
        run: |
          bitbox-audit --repo . --test-results jest.json \
            --format markdown --output report.md
      - uses: marocchino/sticky-pull-request-comment@v2
        with:
          header: bitbox-audit
          path: report.md
```

A working example lives in [`dfx-wallet`'s PR #153](https://github.com/DFXswiss/dfx-wallet/pull/153).

### 5 · Read the audit comment on your next PR

The sticky comment shows:

- **Static findings**: the audit ran patterns against your source; any matches are flagged with file:line, severity, and a fix hint.
- **Coverage buckets**: which quirks are statically detected, which are covered by passing runtime tests, which have failing tests, which are untested.
- **Untested quirks**: explicit list. Each one is a gap until you add a test for it.

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

The 30 documented quirks. Run `bitbox-audit --format json | jq '.findings[].quirk_id'` against your repo for the subset relevant to you.

| Category | Count | Severity-mix |
| -------- | ----: | ------------ |
| ETH | 10 | 3 critical · 5 warning · 2 hint |
| BTC | 7 | 3 critical · 3 warning · 1 hint |
| Cardano | 4 | 3 critical · 0 warning · 1 hint |
| Mnemonic | 3 | 0 critical · 1 warning · 2 hint |
| Protocol | 3 | 2 critical · 0 warning · 1 hint |
| App | 3 | 2 critical · 1 warning · 0 hint |

Critical quirks are silent-corruption / silent-crash class. Warnings are user-visible but recoverable. Hints are documentation reminders.

---

## When you outgrow this onboarding

- The [`TESTING.md`](TESTING.md) cookbook covers the BLE transport fake (Go), vendor simulator integration, and the full scenario / guard API.
- [`quirks/SCHEMA.md`](quirks/SCHEMA.md) is the contract for adding new quirks.
- The [Audit CLI source](go/cmd/bitbox-audit) is small enough to read end-to-end if you want to extend it (new detection kinds, additional output formats).
