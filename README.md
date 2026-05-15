# bitbox-testkit

Test infrastructure for BitBox02 integrations. Catches known firmware-quirk regressions in any wallet that talks to a BitBox — across stacks, languages, and CI setups.

**Start here:** [`ONBOARDING.md`](ONBOARDING.md) — zero to PR-comment audit coverage in 5 minutes.

For schema details: [`quirks/SCHEMA.md`](quirks/SCHEMA.md). For the per-layer cookbook: [`TESTING.md`](TESTING.md).

---

## Stacks

| You're using | Pick |
| ------------ | ---- |
| TypeScript / React Native / Web on `bitbox-api` (Rust → WASM) | `/ts/` |
| Flutter plugin / native Go consumer of `bitbox02-api-go` | `/go/` |

Both implementations share one knowledge base (`/go/bitbox/quirks/quirks.json`); the TS copy is kept byte-identical by `scripts/sync-quirks.sh` and enforced in CI.

## What it gives you

- **30 documented BitBox firmware quirks** with severity, source citation, firmware version range, and (where possible) a static-detection rule.
- **`bitbox-audit` CLI** that scans any repo and reports findings + coverage buckets. Consumes Jest or `go test` JSON output to surface dynamic test coverage alongside static.
- **Scriptable fakes** for `firmware.Communication` (Go) and `PairedBitBox` (TS) — drop into your existing test suite.
- **Pre-built scenarios** for known bug classes: non-ASCII EIP-712, BLE-dedup retransmit, gomobile/WebView panic, hard-coded transport timeouts, channel-hash race, device disconnect, slow user-confirm.
- **Source guards** (Go and TS) for test-time static checks.
- **Vendor simulator integration** (Go, Linux/amd64): downloads and runs the official BitBox02 simulator binary for end-to-end runs.

## Repo layout

```
/quirks/SCHEMA.md           canonical schema of the knowledge base
/go/bitbox/quirks/          Go module + embedded quirks.json
/go/cmd/bitbox-audit/       Audit CLI (Go)
/go/cmd/bitbox-audit-explain/  LLM-narrative wrapper
/ts/src/                    TypeScript source
/scripts/sync-quirks.sh     keep ts/src/quirks/quirks.json byte-identical to Go-side
.github/workflows/test.yml  CI: Go vet/race + TS unit + sync check
ONBOARDING.md               5-minute consumer onboarding
TESTING.md                  full per-layer cookbook
```

## Quick install

```bash
# Audit CLI (runs against any BitBox-integrating repo)
go install github.com/joshuakrueger-dfx/bitbox-testkit/go/cmd/bitbox-audit@main
go install github.com/joshuakrueger-dfx/bitbox-testkit/go/cmd/bitbox-audit-explain@main

# TypeScript testkit (Jest / React Native consumers) — npm package coming;
# for now, vendor /ts/ or install via git URL.
```

## Audit any BitBox-integrating repo

```bash
bitbox-audit --repo /path/to/your/wallet --format markdown
```

For test-coverage integration:

```bash
npx jest --json --outputFile=jest.json
bitbox-audit --repo . --test-results jest.json --format markdown
```

The report names every untested quirk explicitly — no more misleading "0 findings = clean".
