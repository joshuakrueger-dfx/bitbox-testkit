# Changelog

All notable changes to bitbox-testkit. The project uses semantic versioning starting from v0.1.0.

## v0.3.1 — 2026-05-16

Patch: refresh SHA-256 pins for the three most-recent simulator binaries (v9.24.0, v9.25.0, v9.26.1) after Shift Crypto reproducibly rebuilt the upstream artefacts. Behaviour is unchanged — the rebuild only altered build-metadata (timestamps, paths). The five older versions (v9.19.0–v9.23.0) still match their original pins.

Caught by the first CI run of `bitbox-simulator-check` against `bitbox-api@0.12.0` on PR #153 — exactly the supply-chain-alarm mode the script was designed for. No user action required beyond bumping the testkit ref in consumer workflows from v0.3.0 → v0.3.1.

[v0.3.1]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.1

## v0.3.0 — 2026-05-16

End-to-end against real firmware: a new CLI launches the official BitBox02 simulator binary and runs the testkit's curated baseline scenarios — pair, deviceInfo, get-address (mainnet + Polygon multi-byte v), sign-message (small + 1024-byte boundary), sign-EIP1559 — against the actual firmware logic. Replaces mock-only test coverage with real-firmware validation for every consumer.

### Added

- **`bitbox-simulator-check` CLI** (`go/cmd/bitbox-simulator-check`): launches the simulator from the embedded SHA-pinned list (currently v9.19.0 through v9.26.1), exercises BaselineScenarios, emits JSON or Markdown report, exits non-zero on scenario failure. Linux/amd64 only — on macOS / Windows / arm Linux the CLI exits cleanly with a "skipped" status so cross-platform CI matrices stay simple.
- **`go/bitbox/simulator/scenarios.go`**: typed Scenario library. BaselineScenarios() returns the curated set every DFX consumer cares about — extendable by future quirks.
- **Composite action `.github/actions/bitbox-simulator/action.yml`**: one-line `uses:` step for any DFX repo. Inputs: `testkit-ref`, `comment-on-pr`, `fail-on-findings`, `fail-on-skip`.
- **Workflow templates** `bitbox-simulator.yml` (auto-trigger) and `bitbox-simulator-slash.yml` (`/bitbox-simulator` comment trigger with author_association auth gate).

### Changed

- ONBOARDING §4 adds a third recommended workflow alongside `bitbox-audit` and the slash command.

[v0.3.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.3.0

## v0.2.0 — 2026-05-16

Reusable distribution: one `uses:` step gives any DFX repo a fully wired BitBox audit. Quirk count climbs from 30 → 31; static detection from 7 → 11.

### Added

- **Composite GitHub Action** `joshuakrueger-dfx/bitbox-testkit/.github/actions/bitbox-audit@v0.2.0`: encapsulates Go + Node setup, dependency install, Jest `--json`, audit CLI install + run, sticky PR comment, artifact upload. Inputs cover firmware narrowing, `jest-extra-args`, `fail-on-findings`, testkit ref pinning, WASM hash check toggle.
- **Slash-command template** `bitbox-audit-slash.yml`: a maintainer comments `/bitbox-audit [firmware=X] [ref=Y] [fail]` on any PR to trigger an on-demand audit. Authorization gates on `author_association ∈ {OWNER, MEMBER, COLLABORATOR}`.
- **Workflow templates** registered as `.github/workflow-templates/` so GitHub's "New workflow" UI lists them for any org-owned repo.
- **Quirk A4** — address-display-on-device-required (consumer must call `BTCAddress(displayOnDevice=true)` for receive flows, else man-in-the-middle on `getAddress` JSON-RPC).
- **Static detection** for quirks B2, C2, A3, A4. Coverage now 11 of 31 (E1, E7, B1, B2, C2, M1, P2, A1, A2, A3, A4).
- **CI: composite action self-test job** runs the action against the testkit itself to catch regressions in the action's own shell logic. YAML-lint job parses every workflow/action under `.github/`.

### Changed

- **ONBOARDING.md** §4 rewritten: composite-action `uses:` line is now the recommended path; long-form workflow demoted to fallback.
- **A1 detection** narrowed from "any `//export`" to "any `//export` without a paired `defer recoverPanic(` within 6 lines" via the new `missing_pair_within` rule kind — fixed false-positive on every cgo binding in `bitbox_flutter`.
- **A2 detection** narrowed via `context_regex` so `setTimeout` no longer matches; only BitBox / U2FHID / hardware-wallet / signing call sites are considered.

### Fixed

- `regex_in_context` no longer required RE2 lookahead (`(?!\s*\*)`), which Go's regexp engine doesn't support.

## v0.1.0 — 2026-05-15

First release. Establishes the architecture and ships an initial knowledge base of 30 documented BitBox02 firmware constraints.

### Capabilities

- **Quirks knowledge base** (`/go/bitbox/quirks/quirks.json`): 30 quirks across ETH (10), BTC (7), Cardano (4), Mnemonic (3), Protocol (3), App (3). Each carries severity, firmware version range, source citation, and — where statically detectable — at least one detection pattern.
- **Data-driven static detection** with four rule kinds: `regex`, `regex_in_context`, `ordered_pair`, `missing_pair_within`. Seven of the 30 quirks ship with detection patterns: E1, E7, B1, M1, P2, A1, A2.
- **`bitbox-audit` CLI** scans any repository (Go, TypeScript/JS, Dart) and emits JSON or Markdown reports. Coverage-aware: distinguishes statically-detected quirks from runtime-only ones, surfaces explicit "not covered" lists so absence of findings doesn't masquerade as completeness.
- **`bitbox-audit-explain` CLI** turns audit JSON into a plain-language narrative. Calls the Anthropic Messages API when `ANTHROPIC_API_KEY` is set; prints the structured prompt otherwise.
- **Test-results integration**: `--test-results <path>` accepts Jest `--json` or `go test -json` output. Quirks named in passing/failing test descriptions get folded into the Coverage table.
- **Per-file inline suppression** via `audit-skip-line` comments; whole-file via `audit-skip-file`.

### Go testkit (`/go`)

- `bitbox/fake` — scriptable in-memory implementation of `firmware.Communication` with handler chains, recorded calls, defensive snapshots.
- `bitbox/scenarios` — eight pre-built scenarios: Umlaut-EIP712, Disconnect, Panic, SlowResponse, ChannelHashEarly, ErrInvalidInput, UnknownNetwork, PairingRace.
- `bitbox/simulator` — official BitBox02 simulator binary lifecycle (Linux/amd64 only). Embedded list covers firmware v9.19.0 through v9.26.1 with SHA256 verification.
- `core/transport/ble` — `io.ReadWriteCloser` BLE peripheral fake. Race-tested with 5000-byte interleaved Inject/Read and 50-trial Close races.
- `core/guards` — test-time wrappers that lift static findings to `testing.TB.Errorf`. Data-driven from the same quirks.json the audit-runner uses, so the two surfaces cannot drift.
- `core/testutil` — deadlock detector, timeout-bounded execution, atomic counter, polling-based assertions.

### TypeScript testkit (`/ts`)

- `fake/` — `FakePairedBitBox` with Proxy-based dispatch. Symbol-safe (does not pretend to be thenable), records calls, supports `clearCalls` mid-flight.
- `scenarios/` — eight scenario factories matching the Go set.
- `guards/` — `expandGlobs` + per-quirk source-level checks for TypeScript / JavaScript codebases.
- `quirks/` — `Registry`, `subset`, `firmwareApplies`. Loads the canonical JSON shared with the Go side (kept byte-identical by `scripts/sync-quirks.sh`).

### CI integration

- `.github/workflows/test.yml`: three jobs — quirks-sync verification, Go vet + race tests, TypeScript unit tests.
- Worked example in `DFXswiss/dfx-wallet#153`: PR-triggered audit job that runs Jest in `--json` mode, feeds results to bitbox-audit, posts a sticky PR comment with static + dynamic coverage.

### Validated against

- `DFXswiss/dfx-wallet` (React Native + TypeScript): 223 files scanned, 0 false positives, 13 of 30 quirks covered (7 static + 10 runtime, 4 overlapping). The 16 uncovered are not reachable in dfx-wallet's current BitBox surface.
- `DFXswiss/bitbox_flutter` (Flutter plugin, Go-heavy): 20 files scanned, 0 false positives. The `missing_pair_within` A1 detector correctly recognises every `//export` declaration's paired `defer recoverPanic()`.
- `DFXswiss/realunit-app` (pure Dart): 408 files scanned, 0 false positives. Dart-shaped patterns (`Future.delayed(Duration(seconds: 10))`) added for A2.
- Testkit self-scan: 0 false positives after introducing `audit-skip-file` markers on the pattern-documenting guard files.

### Known limitations (intentional)

- No npm publish yet; TypeScript consumers either vendor `/ts/` or install via the git URL.
- 16 of the 30 quirks have no static signature; coverage relies on consumer-written runtime tests with the testkit's scenarios.
- macOS / Windows BitBox02 simulator builds do not exist upstream; the Linux/amd64 simulator integration is the only end-to-end runtime path.
- Vendor-firmware SHA256s for v9.24.0–v9.26.1 were transcribed from the BitBoxSwiss releases page; a CI download surfaces an explicit hash-mismatch error if upstream rebuilds an artifact, rather than silently substituting tampered content.

[v0.1.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.1.0
[v0.2.0]: https://github.com/joshuakrueger-dfx/bitbox-testkit/releases/tag/v0.2.0
